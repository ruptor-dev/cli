package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/cloud"
	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/evaluator"
	"github.com/ruptor-dev/cli/internal/evaluator/llmjudge"
	"github.com/ruptor-dev/cli/internal/evaluator/rules"
	"github.com/ruptor-dev/cli/internal/llmclient"
	"github.com/ruptor-dev/cli/internal/proxy"
	"github.com/ruptor-dev/cli/internal/proxy/faults"
	"github.com/ruptor-dev/cli/internal/report"
	"github.com/ruptor-dev/cli/internal/runner"
	"github.com/ruptor-dev/cli/internal/simulate"
	"github.com/ruptor-dev/cli/internal/ui"
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/spf13/cobra"
)

// version, commit, and buildDate are overridden at release time by
// goreleaser via -ldflags "-X main.version=… -X main.commit=…
// -X main.buildDate=…". Dev builds keep the placeholder values so
// `ruptor --version` is never silently empty.
var (
	version   = "0.0.0-dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// rootLogger is the process-wide zerolog logger. Replaces the previous
// slog default. Initialised in PersistentPreRun so --verbose / --quiet
// flags (added in a later PR) can tune the level.
var rootLogger = ui.SilentLogger()

func newRootCmd() *cobra.Command {
	var logLevel string

	cmd := &cobra.Command{
		Use:     "ruptor",
		Short:   "Ruptor - chaos testing and simulation for AI agents",
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			rootLogger = ui.NewLogger(ui.LogLevel(logLevel))
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetVersionTemplate("ruptor version {{.Version}}\n")

	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")

	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newSimulateCmd())
	cmd.AddCommand(newValidateCmd())
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newDoctorCmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newSyncCmd())

	return cmd
}

// ---------------------------------------------------------------------------
// run command
// ---------------------------------------------------------------------------

func newRunCmd() *cobra.Command {
	var outputPath string
	var testFilter string
	var cloudFlag bool
	var verbose bool

	cmd := &cobra.Command{
		Use:   "run <config-file>",
		Short: "Run chaos tests against an AI agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChaos(cmd.Context(), args[0], outputPath, testFilter, cloudFlag, verbose)
		},
	}

	cmd.Flags().StringVar(&outputPath, "output", "", "output file path for the report")
	cmd.Flags().StringVar(&testFilter, "test", "", "run only the test with this ID")
	cmd.Flags().BoolVar(&cloudFlag, "cloud", false, "spool report to ~/.ruptor/pending/ for upload by `ruptor sync`")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "tee runner/proxy logs to stderr alongside <runDir>/ruptor.log (may interleave with the TUI)")

	return cmd
}

func runChaos(ctx context.Context, cfgPath, outputPath, testFilter string, cloudFlag, verbose bool) error {
	cfg, err := config.LoadChaos(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	tests, err := filterTests(cfg.Tests, testFilter)
	if err != nil {
		return err
	}

	// Open the run log directory and redirect the logger into
	// ruptor.log BEFORE constructing any component that captures a
	// logger reference. While the TUI owns the terminal, zerolog
	// must not touch stdout or stderr — both share the tty and
	// interleave with Bubbletea's render escape sequences.
	runDir, err := newRunLogDir()
	if err != nil {
		rootLogger.Warn().Err(err).Msg("could not create run log dir; continuing with default logger")
	}

	logger := rootLogger
	logFilePath := ""
	if runDir != "" {
		logFilePath = filepath.Join(runDir, "ruptor.log")
		if f, err := os.Create(logFilePath); err == nil {
			// Verbose opts into log-vs-TUI interleave: events land in
			// both ruptor.log and the operator's terminal. The live
			// TUI is unchanged; the operator explicitly asked to see
			// the stream.
			if verbose {
				logger = ui.NewLoggerTo(io.MultiWriter(f, os.Stderr), ui.LogInfo)
			} else {
				logger = ui.NewLoggerTo(f, ui.LogInfo)
			}
			defer f.Close()
		} else {
			logFilePath = ""
		}
	} else if verbose {
		// No run dir — fall back to rootLogger (stderr). Matches the
		// verbose contract even when we could not open the log file.
		logger = ui.NewLogger(ui.LogInfo)
	}

	registry := faults.NewFaultRegistry()

	judge, err := buildJudge(cfg.Evaluation.LLMJudge, logger)
	if err != nil {
		return err
	}

	eval := evaluator.NewChaosEvaluator(judge, cfg.Evaluation.MaxIterations, logger)

	p := proxy.NewProxy(
		&cfg.Proxy,
		tests,
		registry,
		proxy.WithLogger(logger),
		proxy.WithTimeout(time.Duration(cfg.Proxy.RequestTimeoutS)*time.Second),
	)

	renderer := rendererFor(cfg.Output, outputPath)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	proxyErrCh := make(chan error, 1)
	go func() {
		proxyErrCh <- p.Start(ctx)
	}()

	logger.Info().
		Str("agent", cfg.Agent.Name).
		Int("tests", len(tests)).
		Int("port", cfg.Proxy.Port).
		Msg("ruptor chaos proxy starting")

	durations := newDurationTracker()
	runStart := time.Now()

	// Build the TUI program up-front (if interactive) so the
	// orchestrator can send it ForceSnapshotMsg + a 50ms render
	// grace right after the last durations.set, guaranteeing the
	// "4/4 → final %" frame lands before ctx cancels.
	var prog *ui.Program
	if ui.IsInteractive() {
		boundPort := waitAndReadBoundPort(ctx, p, 10*time.Second)
		if boundPort == 0 {
			boundPort = cfg.Proxy.Port
		}
		prog = ui.NewRunProgress(ui.RunContext{
			ConfigFile: cfg.Agent.Name,
			AgentName:  cfg.Agent.Name,
			Port:       boundPort,
			Entrypoint: cfg.Agent.Entrypoint,
			Snapshot:   snapshotFn(tests, p, durations, cfg.Evaluation.MaxIterations, ctx),
		})
	}
	flush := func() {
		if prog != nil {
			prog.Send(ui.ForceSnapshotMsg{})
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Warn once per run when MCP mode could plausibly be exercised.
	// Faults still fire on the wire, but MCP observations are not yet
	// wired to the evaluator — see docs/specs/backlog/mcp-observations-evaluator.md.
	warnIfMCPModeUnscored(cfg)

	orchErrCh := make(chan error, 1)
	go func() {
		orchErrCh <- orchestrateExperiments(ctx, stop, cfg, tests, p, runDir, durations, flush, logger)
	}()

	if err := runChaosTUI(ctx, stop, prog); err != nil {
		return err
	}

	if err := <-orchErrCh; err != nil && !errors.Is(err, context.Canceled) {
		logger.Warn().Err(err).Msg("experiment orchestrator returned error")
	}

	if err := waitForProxy(ctx, proxyErrCh, logger); err != nil {
		return err
	}

	rpt, err := buildChaosReport(context.Background(), eval, cfg, tests, p.Observations(), durations, logger)
	if err != nil {
		return err
	}
	rpt.DurationMs = time.Since(runStart).Milliseconds()

	if err := renderer.RenderChaos(rpt); err != nil {
		return fmt.Errorf("rendering chaos report: %w", err)
	}

	if cloudFlag {
		if err := spoolReportForCloud(rpt, logger); err != nil {
			// Spooling is best-effort: a failure must not turn an
			// otherwise-successful chaos run into a failed exit. Log
			// the error and continue to the completion screen.
			logger.Warn().Err(err).Msg("ruptor: could not spool report to pending/")
		}
	}

	ui.PrintCompletion(ui.CompletionSummary{
		// Completion bar speaks in whole-percent integers; Score is
		// the canonical 0.0–1.0 form persisted to the report.
		ScorePercent: int(rpt.Score * 100),
		Passed:       rpt.Passed,
		Failed:       rpt.Failed,
		ReportPaths:  reportPathsFor(renderer, cfg.Output, outputPath),
		AgentLogDir:  runDir,
		LogPath:      logFilePath,
	})

	logger.Info().
		Int("total", rpt.TotalTests).
		Int("passed", rpt.Passed).
		Int("failed", rpt.Failed).
		Float64("score", rpt.Score).
		Msg("chaos run complete")

	return nil
}

// runChaosTUI drives the Bubbletea live-run program. Returns once the
// user presses 'q' or the context is cancelled. Reading proxy
// observations every tick is O(tests) under a mutex — fast enough for
// v1 at the default 100ms tick. See AUDIT.md §10 for the push-channel
// follow-up.
func runChaosTUI(ctx context.Context, stop context.CancelFunc, prog *ui.Program) error {
	if prog == nil {
		<-ctx.Done()
		return nil
	}

	go func() {
		<-ctx.Done()
		prog.Quit()
	}()

	final, err := prog.Run()
	stop()
	if err != nil {
		return fmt.Errorf("ui: %w", err)
	}
	if ui.Aborted(final) {
		return nil
	}
	return nil
}

// snapshotFn builds the callback the TUI polls every tick. The
// durationTracker is authoritative for per-experiment lifecycle —
// orchestrator marks each test running/done unambiguously — while
// proxy observations decide pass vs fail once the experiment ends.
//
// The pass/fail projection mirrors the evaluator's rules (see
// internal/evaluator/rules/classify.go) so the live TUI and
// the final Robustness Score agree. A `HadError=true` that came
// from the injected fault itself (a 504 on tool_timeout, a 5xx on
// tool_error / llm_error) is the expected response — treating it as
// a TUI failure would contradict the evaluator and show a 62 % bar
// that immediately jumps to 100 % on the completion screen.
func snapshotFn(tests []config.TestConfig, p *proxy.Proxy, d *durationTracker, maxIterations int, ctx context.Context) ui.SnapshotFn {
	return func() ui.RunSnapshot {
		obs := p.Observations()
		exps := make([]ui.ExperimentState, 0, len(tests))
		passed, finished := 0, 0
		for _, t := range tests {
			done, running := d.status(t.ID)
			var status ui.ExperimentStatus
			switch {
			case done:
				// Delegate to the single source of truth. The TUI
				// skips the LLM judge; the judge can only further
				// downgrade a pass, never promote, so its absence
				// here is conservative.
				v := rules.ClassifyExperiment(t.Fault, maxIterations, rules.Obs{
					Hits:           obs[t.ID].Hits,
					LastStatusCode: obs[t.ID].LastStatusCode,
					HadError:       obs[t.ID].HadError,
				}, d.getAgentError(t.ID))
				if v.Passed {
					status = ui.StatusPassed
					passed++
				} else {
					status = ui.StatusFailed
				}
				finished++
			case running:
				status = ui.StatusRunning
			default:
				status = ui.StatusPending
			}
			exps = append(exps, ui.ExperimentState{
				ID:       t.ID,
				Status:   status,
				Duration: time.Duration(d.get(t.ID)) * time.Millisecond,
			})
		}
		score := 0
		if finished > 0 {
			score = (passed * 100) / finished
		}
		return ui.RunSnapshot{
			Experiments:  exps,
			ScorePercent: score,
			// Only natural completion flips Done. Aborts via ctx
			// cancellation go through the prog.Quit path in the
			// ctx-watcher goroutine — avoiding `ctx.Err() != nil`
			// here prevents a race where ctx cancels the TUI
			// before the final finished==total snapshot lands.
			Done: finished == len(tests),
		}
	}
}

// reportPathsFor describes the files the renderer wrote, so the
// completion screen can show them. Kept a pure string builder — no
// file-system probing.
func reportPathsFor(_ report.Renderer, outCfg config.OutputConfig, outputPath string) []string {
	path := outCfg.Path
	if outputPath != "" {
		return []string{outputPath}
	}
	var out []string
	switch outCfg.Format {
	case "json":
		out = append(out, path+"chaos_report.json")
	case "html":
		out = append(out, path+"chaos_report.html")
	case "both":
		out = append(out,
			path+"chaos_report.html",
			path+"chaos_report.json",
		)
	}
	return out
}

func filterTests(tests []config.TestConfig, filter string) ([]config.TestConfig, error) {
	if filter == "" {
		return tests, nil
	}
	var out []config.TestConfig
	for _, t := range tests {
		if t.ID == filter {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no test found with id %q", filter)
	}
	return out, nil
}

func rendererFor(outCfg config.OutputConfig, outputPath string) report.Renderer {
	format := outCfg.Format
	path := outCfg.Path
	if outputPath != "" {
		path = outputPath
		ext := strings.ToLower(filepath.Ext(outputPath))
		switch ext {
		case ".html":
			format = "html"
		case ".json":
			format = "json"
		default:
			format = "html"
		}
	}
	return report.NewRendererFromFormat(format, path)
}

func waitForProxy(ctx context.Context, proxyErrCh chan error, logger zerolog.Logger) error {
	select {
	case <-ctx.Done():
		logger.Info().Msg("shutting down; waiting for proxy to stop")
		if err := <-proxyErrCh; err != nil {
			return fmt.Errorf("proxy error: %w", err)
		}
	case err := <-proxyErrCh:
		if err != nil {
			return fmt.Errorf("proxy error: %w", err)
		}
	}
	return nil
}

// buildChaosReport assembles a ReliabilityReport from per-test observations.
// Tests that never received a request are still emitted (Hits=0) so the
// user sees which paths their agent did not exercise during the run.
func buildChaosReport(
	ctx context.Context,
	eval *evaluator.ChaosEvaluator,
	cfg *config.ChaosConfig,
	tests []config.TestConfig,
	obs map[string]proxy.Observation,
	durations *durationTracker,
	logger zerolog.Logger,
) (*types.ReliabilityReport, error) {
	results := make([]types.TestResult, 0, len(tests))
	passed, failed := 0, 0

	for _, t := range tests {
		r := evaluateTest(ctx, eval, t, obs[t.ID], durations.getAgentError(t.ID), cfg.Evaluation.LLMJudgePrompt, logger)
		r.DurationMs = durations.get(t.ID)
		if r.Passed {
			passed++
		} else {
			failed++
		}
		results = append(results, *r)
	}

	score := 0.0
	if len(tests) > 0 {
		score = float64(passed) / float64(len(tests))
	}

	return &types.ReliabilityReport{
		SchemaVersion: types.ReportSchemaVersion,
		RuptorVersion: version,
		AgentName:     cfg.Agent.Name,
		RunAt:         time.Now(),
		TotalTests:    len(tests),
		Passed:        passed,
		Failed:        failed,
		Score:         score,
		Results:       results,
	}, nil
}

func evaluateTest(
	ctx context.Context,
	eval *evaluator.ChaosEvaluator,
	t config.TestConfig,
	o proxy.Observation,
	agentErr error,
	judgePrompt string,
	logger zerolog.Logger,
) *types.TestResult {
	r, err := eval.Evaluate(
		ctx,
		t.ID,
		t.Fault,
		t.Tool,
		o.LastStatusCode,
		o.Hits,
		o.HadError,
		o.Recovered(),
		agentErr,
		judgePrompt,
		"", // agent behavior transcript — collected in a later PR
	)
	if err != nil {
		logger.Warn().
			Str("test_id", t.ID).
			Err(err).
			Msg("chaos evaluator failed")
		return &types.TestResult{
			TestID:    t.ID,
			FaultType: t.Fault,
			Tool:      t.Tool,
			Passed:    false,
			Error:     err.Error(),
		}
	}
	return r
}

// ---------------------------------------------------------------------------
// simulate command
// ---------------------------------------------------------------------------

func newSimulateCmd() *cobra.Command {
	var outputPath string
	var simFilter string

	cmd := &cobra.Command{
		Use:   "simulate <config-file>",
		Short: "Run simulated conversations against an AI agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSimulate(cmd.Context(), args[0], outputPath, simFilter)
		},
	}

	cmd.Flags().StringVar(&outputPath, "output", "", "output file path for the report")
	cmd.Flags().StringVar(&simFilter, "sim", "", "run only the simulation with this ID")

	return cmd
}

func runSimulate(ctx context.Context, cfgPath, outputPath, simFilter string) error {
	logger := rootLogger

	cfg, err := config.LoadSimulate(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	sims, err := filterSimulations(cfg.Simulations, simFilter)
	if err != nil {
		return err
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable is required for simulations")
	}

	llmClient := &simulateLLMAdapter{
		client: llmclient.New(apiKey, "",
			llmclient.WithHTTPClient(&http.Client{Timeout: 120 * time.Second}),
			llmclient.WithLogger(logger),
			llmclient.WithRetry(),
		),
	}

	judge, err := buildJudge(true, logger)
	if err != nil {
		return err
	}

	simEval := evaluator.NewSimulateEvaluator(judge, logger)
	if cfg.Agent.RequestTimeoutS <= 0 {
		cfg.Agent.RequestTimeoutS = 30
	}
	sim := simulate.NewSimulator(llmClient, &http.Client{Timeout: 60 * time.Second}, logger, cfg.Agent)

	renderer := rendererFor(cfg.Output, outputPath)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info().
		Str("agent", cfg.Agent.Name).
		Int("simulations", len(sims)).
		Msg("ruptor simulation starting")

	results, err := runSimulations(ctx, sims, sim, simEval, cfg.Evaluation.LLMJudgePrompt, logger)
	if err != nil {
		return err
	}

	rpt := buildSimulateReport(cfg, results)

	if err := renderer.RenderSimulate(rpt); err != nil {
		return fmt.Errorf("rendering report: %w", err)
	}

	logger.Info().
		Int("total", len(results)).
		Int("goal_reached", rpt.GoalReached).
		Float64("avg_score", rpt.AvgScore).
		Msg("simulation complete")

	return nil
}

func filterSimulations(sims []config.Simulation, filter string) ([]config.Simulation, error) {
	if filter == "" {
		return sims, nil
	}
	var out []config.Simulation
	for _, s := range sims {
		if s.ID == filter {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no simulation found with id %q", filter)
	}
	return out, nil
}

func runSimulations(
	ctx context.Context,
	sims []config.Simulation,
	sim *simulate.Simulator,
	simEval *evaluator.SimulateEvaluator,
	judgePrompt string,
	logger zerolog.Logger,
) ([]types.SimulationResult, error) {
	var results []types.SimulationResult
	for _, s := range sims {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("cancelled: %w", err)
		}

		logger.Info().
			Str("id", s.ID).
			Str("persona", s.Persona).
			Msg("running simulation")

		result, err := sim.Run(ctx, s)
		if err != nil {
			return nil, fmt.Errorf("simulation %s: %w", s.ID, err)
		}

		if judgePrompt != "" {
			applyEvaluation(ctx, simEval, s, result, judgePrompt, logger)
		}

		results = append(results, *result)
	}
	return results, nil
}

func applyEvaluation(
	ctx context.Context,
	simEval *evaluator.SimulateEvaluator,
	s config.Simulation,
	result *types.SimulationResult,
	judgePrompt string,
	logger zerolog.Logger,
) {
	history := result.History
	if history == nil {
		history = &types.ConversationHistory{}
	}
	evalResult, err := simEval.Evaluate(ctx, s.ID, s.Persona, s.Goal, history, judgePrompt)
	if err != nil {
		logger.Warn().Str("id", s.ID).Err(err).Msg("evaluation failed")
		return
	}
	result.QualityScore = evalResult.QualityScore
	result.Issues = evalResult.Issues
}

func buildSimulateReport(cfg *config.SimulateConfig, results []types.SimulationResult) *types.ConversationReport {
	goalReached := 0
	var totalScore float64
	for _, r := range results {
		if r.GoalReached {
			goalReached++
		}
		totalScore += float64(r.QualityScore)
	}
	avgScore := 0.0
	if len(results) > 0 {
		avgScore = totalScore / float64(len(results))
	}

	return &types.ConversationReport{
		SchemaVersion: types.ReportSchemaVersion,
		RuptorVersion: version,
		AgentName:     cfg.Agent.Name,
		RunAt:         time.Now(),
		TotalSims:     len(results),
		GoalReached:   goalReached,
		AvgScore:      avgScore,
		Results:       results,
	}
}

// ---------------------------------------------------------------------------
// validate command
// ---------------------------------------------------------------------------

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <config-file>",
		Short: "Validate a chaos or simulation configuration file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(args[0])
		},
	}
}

func runValidate(cfgPath string) error {
	logger := rootLogger
	_, chaosErr := config.LoadChaos(cfgPath)
	if chaosErr == nil {
		logger.Info().Str("type", "chaos").Msg("config valid")
		return nil
	}

	_, simErr := config.LoadSimulate(cfgPath)
	if simErr == nil {
		logger.Info().Str("type", "simulate").Msg("config valid")
		return nil
	}

	return fmt.Errorf("config validation failed:\n  as chaos: %v\n  as simulate: %v", chaosErr, simErr)
}

// ---------------------------------------------------------------------------
// version command
// ---------------------------------------------------------------------------

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print Ruptor version",
		Run: func(cmd *cobra.Command, args []string) {
			ui.Printf("ruptor version %s\n", version)
			ui.Printf("  commit:     %s\n", commit)
			ui.Printf("  built:      %s\n", buildDate)
		},
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func buildJudge(useLLM bool, logger zerolog.Logger) (llmjudge.Judge, error) {
	if !useLLM {
		return &llmjudge.NoopJudge{}, nil
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		logger.Warn().Msg("OPENAI_API_KEY not set, using noop judge")
		return &llmjudge.NoopJudge{}, nil
	}

	return llmjudge.NewOpenAIJudge(apiKey, "", logger), nil
}

type simulateLLMAdapter struct {
	client *llmclient.OpenAIClient
}

func (a *simulateLLMAdapter) Complete(ctx context.Context, messages []map[string]string, model string) (string, error) {
	return a.client.CompleteMap(ctx, messages, model)
}

// spoolReportForCloud writes the chaos report JSON to
// ~/.ruptor/pending/ for `ruptor sync` to pick up. The user-visible
// follow-up depends on the cloud feature flag: with reporting still
// disabled at build time we surface the waitlist line, otherwise we
// note that the next sync will upload it. Writing happens either way
// so the spool plumbing is exercised continuously and `ruptor sync`
// has something to upload the moment the flag flips.
func spoolReportForCloud(rpt *types.ReliabilityReport, logger zerolog.Logger) error {
	settings, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}
	body, err := json.MarshalIndent(rpt, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling report: %w", err)
	}
	pendingDir := filepath.Join(settings.ConfigDir, cloud.PendingDirName)
	path, err := cloud.WritePending(pendingDir, cloud.NewRunID("chaos"), body)
	if err != nil {
		return err
	}
	logger.Info().Str("path", path).Msg("ruptor: report spooled for cloud upload")

	if !cloud.CloudReportingEnabled {
		ui.Info("Cloud reporting is coming soon. Join the waitlist at https://ruptor.dev")
		ui.Dim(fmt.Sprintf("    Report queued at %s", path))
		return nil
	}
	ui.Info(fmt.Sprintf("Queued for upload — run `ruptor sync` (file: %s)", path))
	return nil
}

// ---------------------------------------------------------------------------
// agent orchestration
// ---------------------------------------------------------------------------

const (
	agentModeOneshot    = "oneshot"
	agentModePersistent = "persistent"
	defaultExperimentTO = 60 * time.Second
	experimentPoll      = 100 * time.Millisecond
)

// orchestrateExperiments launches the agent-under-test according to
// cfg.Agent.Mode and drives one experiment per test (oneshot) or a
// single lifecycle that spans the whole run (persistent). When the
// agent's Entrypoint is empty the orchestrator returns immediately so
// an externally-managed agent sees no change in behaviour. On exit it
// calls stop() to wind the proxy + TUI down, matching the user's
// expectation that `ruptor run` terminates after its experiments.
func orchestrateExperiments(
	ctx context.Context,
	stop context.CancelFunc,
	cfg *config.ChaosConfig,
	tests []config.TestConfig,
	p *proxy.Proxy,
	runDir string,
	durations *durationTracker,
	flush func(),
	logger zerolog.Logger,
) error {
	defer stop()

	if cfg.Agent.Entrypoint == "" {
		logger.Info().Msg("runner: no agent.entrypoint set — assuming external agent")
		<-ctx.Done()
		return nil
	}

	if err := waitProxyReady(ctx, p, 10*time.Second); err != nil {
		return err
	}

	timeout := time.Duration(cfg.Evaluation.TimeoutS) * time.Second
	if timeout <= 0 {
		timeout = defaultExperimentTO
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.Agent.Mode))
	if mode == "" {
		mode = agentModeOneshot
	}

	logger.Info().Str("mode", mode).Str("run_dir", runDir).Msg("runner: agent lifecycle starting")

	var runErr error
	switch mode {
	case agentModePersistent:
		runErr = runPersistent(ctx, cfg, tests, p, runDir, timeout, durations, logger)
	case agentModeOneshot:
		runErr = runOneshot(ctx, cfg, tests, p, runDir, timeout, durations, logger)
	default:
		return fmt.Errorf("runner: unknown agent.mode %q (want oneshot|persistent)", mode)
	}

	// Force the TUI to take a final snapshot now that every
	// durations.set has landed, then give Bubbletea 50ms to paint
	// the "N/N" + final-% frame before deferred stop() cancels ctx
	// and tears the UI down.
	if ctx.Err() == nil && runErr == nil && flush != nil {
		flush()
	}

	return runErr
}

func runPersistent(
	ctx context.Context,
	cfg *config.ChaosConfig,
	tests []config.TestConfig,
	p *proxy.Proxy,
	runDir string,
	timeout time.Duration,
	durations *durationTracker,
	logger zerolog.Logger,
) error {
	logPath := filepath.Join(runDir, "agent.log")
	agent, err := runner.Start(ctx, runner.Config{
		Entrypoint: cfg.Agent.Entrypoint,
		Env:        cfg.Agent.Env,
		LogPath:    logPath,
	})
	if err != nil {
		return fmt.Errorf("runner start: %w", err)
	}
	logger.Info().Int("pid", agent.PID()).Str("log", logPath).Msg("runner: persistent agent started")

	defer func() { _ = agent.Stop(runner.DefaultStopGrace) }()

	for _, t := range tests {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.ResetObservation(t.ID)
		p.SetActiveTest(t.ID)
		start := time.Now()
		durations.markRunning(t.ID)
		waitForHitsOrTimeout(ctx, p, t.ID, timeout, agent)
		durations.set(t.ID, time.Since(start).Milliseconds())
	}
	p.SetActiveTest("")
	return nil
}

func runOneshot(
	ctx context.Context,
	cfg *config.ChaosConfig,
	tests []config.TestConfig,
	p *proxy.Proxy,
	runDir string,
	timeout time.Duration,
	durations *durationTracker,
	logger zerolog.Logger,
) error {
	for i, t := range tests {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		logPath := filepath.Join(runDir, fmt.Sprintf("agent-%02d-%s.log", i+1, safeFile(t.ID)))
		p.ResetObservation(t.ID)
		p.SetActiveTest(t.ID)
		start := time.Now()
		durations.markRunning(t.ID)
		agent, err := runner.Start(ctx, runner.Config{
			Entrypoint: cfg.Agent.Entrypoint,
			Env:        cfg.Agent.Env,
			LogPath:    logPath,
		})
		if err != nil {
			// Hard fail: if we cannot spawn the entrypoint (binary
			// not on PATH, bad shebang, permission denied) then every
			// experiment will fail the same way. Marking the
			// experiment PASS because the proxy never saw a hit
			// would be lying to the user. Abort the run with the
			// actual exec error so they can fix it (typically: swap
			// `python` for `python3`, or activate the venv).
			logger.Error().Err(err).Str("test", t.ID).Msg("runner: could not start agent — aborting run")
			durations.set(t.ID, time.Since(start).Milliseconds())
			return fmt.Errorf("agent entrypoint failed to start: %w (check cfg.agent.entrypoint and $PATH)", err)
		}
		logger.Info().
			Int("pid", agent.PID()).
			Str("test", t.ID).
			Str("log", logPath).
			Msg("runner: oneshot agent started")

		// Oneshot: wait for the agent to exit ON ITS OWN. Hits>0 is
		// NOT a valid completion signal here — the agent may have
		// received the fault and still be in the middle of its
		// fallback logic (Retry-After sleep, cached-response
		// lookup, etc). Killing it at first hit truncates the
		// behaviour we're trying to measure.
		_ = agent.Wait(ctx, timeout)
		_ = agent.Stop(runner.DefaultStopGrace)
		if exitErr := agent.ExitErr(); exitErr != nil {
			durations.setAgentError(t.ID, exitErr)
		}
		durations.set(t.ID, time.Since(start).Milliseconds())
	}
	p.SetActiveTest("")
	return nil
}

// durationTracker is a mutex-guarded map[testID]ms populated by the
// orchestrator and consumed by buildChaosReport. A plain map is fine
// because the orchestrator writes sequentially and the reader runs
// only after the orchestrator goroutine has returned — the mutex
// guards the (brief) concurrent window where the TUI is being torn
// down.
type durationTracker struct {
	mu         sync.Mutex
	m          map[string]int64
	running    map[string]bool
	agentError map[string]error
}

func newDurationTracker() *durationTracker {
	return &durationTracker{
		m:          map[string]int64{},
		running:    map[string]bool{},
		agentError: map[string]error{},
	}
}

func (d *durationTracker) set(id string, ms int64) {
	d.mu.Lock()
	d.m[id] = ms
	delete(d.running, id)
	d.mu.Unlock()
}

func (d *durationTracker) get(id string) int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.m[id]
}

func (d *durationTracker) markRunning(id string) {
	d.mu.Lock()
	d.running[id] = true
	d.mu.Unlock()
}

// setAgentError records a non-nil exit from the agent child process.
// buildChaosReport combines this with the proxy hit count to decide
// whether the experiment actually exercised the fault path or the
// agent crashed immediately (ImportError, missing venv, etc).
func (d *durationTracker) setAgentError(id string, err error) {
	if err == nil {
		return
	}
	d.mu.Lock()
	d.agentError[id] = err
	d.mu.Unlock()
}

func (d *durationTracker) getAgentError(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.agentError[id]
}

// status returns (done, running) under a single lock.
func (d *durationTracker) status(id string) (done, running bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, done = d.m[id]
	running = d.running[id]
	return
}

// waitForHitsOrTimeout returns as soon as any completion signal fires:
//   - the agent process exited (oneshot mode hits this every time),
//   - the proxy recorded at least one hit for this test (persistent
//     mode, where the agent never exits between experiments), or
//   - the supplied timeout elapses (safety net).
//
// Requiring BOTH agent-exit AND hits would incorrectly block on tests
// where the proxy's first-match dispatch assigns the observation to a
// different testID sharing the same tool. Agent exit alone is enough
// to know the experiment produced whatever it was going to produce.
func waitForHitsOrTimeout(ctx context.Context, p *proxy.Proxy, testID string, timeout time.Duration, a *runner.Agent) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(experimentPoll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return
		case <-tick.C:
			if a.Exited() {
				return
			}
			if p.Observations()[testID].Hits > 0 {
				return
			}
		}
	}
}

// waitProxyReady polls p.Addr() until the listener is bound or the
// timeout expires. Needed because proxy.Start binds asynchronously in
// a goroutine and we do not want to race the agent against an
// unbound port.
func waitProxyReady(ctx context.Context, p *proxy.Proxy, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if addr := p.Addr(); addr != "" {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("runner: proxy did not become ready in time")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// newRunLogDir returns ~/.ruptor/runs/<UTC-timestamp>/ creating the
// tree as needed. Agents stream stdout/stderr into files under this
// directory so they never pollute the TUI.
// waitAndReadBoundPort polls proxy.Addr() until the listener is bound
// or timeout expires, returning the numeric port the OS actually
// accepted. listenWithFallback may have stepped past the requested
// port when busy; the TUI header must reflect reality so users
// retarget their agent correctly. Returns 0 on timeout.
func waitAndReadBoundPort(ctx context.Context, p *proxy.Proxy, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if addr := p.Addr(); addr != "" {
			if _, portStr, err := net.SplitHostPort(addr); err == nil {
				if port, err := strconv.Atoi(portStr); err == nil {
					return port
				}
			}
		}
		select {
		case <-ctx.Done():
			return 0
		case <-time.After(50 * time.Millisecond):
		}
	}
	return 0
}

func newRunLogDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(home, ".ruptor", "runs", ts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// mcpUnscoredWarning is the copy shown once per run when proxy.mode
// could plausibly route traffic through the MCP handler. The MCP
// handler records its own observations; they are not merged into the
// proxy's observation map the evaluator consumes. Users running a
// chaos experiment in that state see faults fire correctly but get
// zero hits in the Robustness Score. The warning stays live until
// docs/specs/backlog/mcp-observations-evaluator.md lands.
const mcpUnscoredWarning = "MCP observation wiring is not yet in the evaluator " +
	"(see docs/specs/backlog/mcp-observations-evaluator.md). Faults will " +
	"fire correctly on the wire, but per-test hits and Robustness Score " +
	"will report zero for MCP tests until this lands."

// warnIfMCPModeUnscored prints the above warning when cfg.Proxy.Mode is
// ProxyModeMCP or ProxyModeAuto — the two modes that can route traffic
// through the MCP handler. Called once at the top of the run, not
// during config load, so `ruptor validate` and similar dry checks stay
// silent.
func warnIfMCPModeUnscored(cfg *config.ChaosConfig) {
	switch cfg.Proxy.Mode {
	case config.ProxyModeMCP, config.ProxyModeAuto:
		ui.Warning(mcpUnscoredWarning)
	}
}

// safeFile scrubs characters that would make a file name awkward on
// any common filesystem. Keeps ASCII alphanumerics, '-' and '_'.
func safeFile(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "unnamed"
	}
	return string(out)
}
