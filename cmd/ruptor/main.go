package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/cloud"
	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/evaluator"
	"github.com/ruptor-dev/cli/internal/evaluator/llmjudge"
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

	cmd := &cobra.Command{
		Use:   "run <config-file>",
		Short: "Run chaos tests against an AI agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChaos(cmd.Context(), args[0], outputPath, testFilter, cloudFlag)
		},
	}

	cmd.Flags().StringVar(&outputPath, "output", "", "output file path for the report")
	cmd.Flags().StringVar(&testFilter, "test", "", "run only the test with this ID")
	cmd.Flags().BoolVar(&cloudFlag, "cloud", false, "spool report to ~/.ruptor/pending/ for upload by `ruptor sync`")

	return cmd
}

func runChaos(ctx context.Context, cfgPath, outputPath, testFilter string, cloudFlag bool) error {
	logger := rootLogger

	cfg, err := config.LoadChaos(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	tests, err := filterTests(cfg.Tests, testFilter)
	if err != nil {
		return err
	}

	// Create the run log dir first so the interactive TUI can silence
	// stderr logging (proxy/runner events would otherwise fight with
	// bubbletea for the terminal). The logs still land on disk.
	runDir, err := newRunLogDir()
	if err != nil {
		logger.Warn().Err(err).Msg("could not create run log dir; agent stdout will go to a temp file")
	}

	if ui.IsInteractive() && runDir != "" {
		logPath := filepath.Join(runDir, "run.log")
		if f, ferr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); ferr == nil {
			defer f.Close()
			logger = ui.NewLoggerTo(f, ui.LogInfo)
		}
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

	renderer := rendererFor(cfg.Output, outputPath, logger)

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

	// Warn once per run when MCP mode could plausibly be exercised.
	// Faults still fire on the wire, but MCP observations are not yet
	// wired to the evaluator — see docs/specs/backlog/mcp-observations-evaluator.md.
	warnIfMCPModeUnscored(cfg)

	orchErrCh := make(chan error, 1)
	go func() {
		orchErrCh <- orchestrateExperiments(ctx, stop, cfg, tests, p, runDir, durations, logger)
	}()

	if err := runChaosTUI(ctx, stop, cfg, tests, p); err != nil {
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
func runChaosTUI(ctx context.Context, stop context.CancelFunc, cfg *config.ChaosConfig, tests []config.TestConfig, p *proxy.Proxy) error {
	if !ui.IsInteractive() {
		<-ctx.Done()
		return nil
	}

	prog := ui.NewRunProgress(ui.RunContext{
		ConfigFile: cfg.Agent.Name,
		AgentName:  cfg.Agent.Name,
		Port:       cfg.Proxy.Port,
		Entrypoint: cfg.Agent.Entrypoint,
		Snapshot:   snapshotFn(tests, p, ctx),
	})

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

// snapshotFn builds the callback the TUI polls on every tick. It maps
// the proxy's observation map into the UI's ExperimentState slice,
// preserving the configured test order (observations map is
// unordered).
func snapshotFn(tests []config.TestConfig, p *proxy.Proxy, ctx context.Context) ui.SnapshotFn {
	return func() ui.RunSnapshot {
		obs := p.Observations()
		exps := make([]ui.ExperimentState, 0, len(tests))
		passed, total := 0, 0
		for _, t := range tests {
			o := obs[t.ID]
			seen := o.Hits > 0
			exps = append(exps, ui.ExperimentState{
				ID:     t.ID,
				Status: statusFromObs(o, seen),
			})
			if seen && !o.HadError {
				passed++
			}
			if seen {
				total++
			}
		}
		score := 0
		if total > 0 {
			score = (passed * 100) / total
		}
		return ui.RunSnapshot{
			Experiments:  exps,
			ScorePercent: score,
			Done:         ctx.Err() != nil,
		}
	}
}

func statusFromObs(o proxy.Observation, seen bool) ui.ExperimentStatus {
	switch {
	case !seen:
		return ui.StatusPending
	case o.HadError:
		return ui.StatusFailed
	default:
		return ui.StatusPassed
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

func rendererFor(outCfg config.OutputConfig, outputPath string, logger zerolog.Logger) report.Renderer {
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
	return report.NewRendererFromFormat(format, path, logger)
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
		r := evaluateTest(ctx, eval, t, obs[t.ID], nil, cfg.Evaluation.LLMJudgePrompt, logger)
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
	transcript io.Reader,
	judgePrompt string,
	logger zerolog.Logger,
) *types.TestResult {
	behavior := ""
	if transcript != nil {
		if b, err := io.ReadAll(transcript); err == nil {
			behavior = string(b)
		}
	}
	r, err := eval.Evaluate(
		ctx,
		t.ID,
		t.Fault,
		t.Tool,
		o.LastStatusCode,
		o.Hits,
		o.HadError,
		o.Recovered(),
		judgePrompt,
		behavior,
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

	renderer := rendererFor(cfg.Output, outputPath, logger)

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

// mcpUnscoredWarning is surfaced once per run whenever proxy mode is
// "mcp" or "auto" — i.e. whenever MCP traffic could plausibly be
// exercised during this run. MCP observations are not yet wired into
// the evaluator (see docs/specs/backlog/mcp-observations-evaluator.md),
// so the Robustness Score and per-test hits will read zero for MCP
// tests even though the faults fire correctly on the wire. The
// warning stays live until that spec lands.
const mcpUnscoredWarning = "MCP observation wiring is not yet in the evaluator (see " +
	"docs/specs/backlog/mcp-observations-evaluator.md). Faults will " +
	"fire correctly on the wire, but per-test hits and Robustness " +
	"Score will report zero for MCP tests until this lands."

// warnIfMCPModeUnscored emits mcpUnscoredWarning once per run when the
// config's proxy.mode could cause MCP traffic to flow through the
// handler (either explicit ProxyModeMCP or ProxyModeAuto which may pick
// MCP at runtime). Pure HTTP runs are silent. Called at the top of the
// run orchestration — not during config load — so `ruptor validate`
// and similar one-off checks do not pollute the terminal.
//
// Validate() is the single gate that canonicalises cfg.Proxy.Mode, so
// this helper compares against the typed constants directly. Anything
// non-canonical (e.g. "MCP", " mcp ") was rejected at load time and
// cannot reach here.
func warnIfMCPModeUnscored(cfg *config.ChaosConfig) {
	switch cfg.Proxy.Mode {
	case config.ProxyModeMCP, config.ProxyModeAuto:
		ui.Warning(mcpUnscoredWarning)
	}
}

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

	switch mode {
	case agentModePersistent:
		return runPersistent(ctx, cfg, tests, p, runDir, timeout, durations, logger)
	case agentModeOneshot:
		return runOneshot(ctx, cfg, tests, p, runDir, timeout, durations, logger)
	default:
		return fmt.Errorf("runner: unknown agent.mode %q (want oneshot|persistent)", mode)
	}
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
		start := time.Now()
		waitForHitsOrTimeout(ctx, p, t.ID, timeout, agent)
		durations.set(t.ID, time.Since(start).Milliseconds())
	}
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
		start := time.Now()
		agent, err := runner.Start(ctx, runner.Config{
			Entrypoint: cfg.Agent.Entrypoint,
			Env:        cfg.Agent.Env,
			LogPath:    logPath,
		})
		if err != nil {
			logger.Warn().Err(err).Str("test", t.ID).Msg("runner: could not start agent")
			durations.set(t.ID, time.Since(start).Milliseconds())
			continue
		}
		logger.Info().
			Int("pid", agent.PID()).
			Str("test", t.ID).
			Str("log", logPath).
			Msg("runner: oneshot agent started")

		waitForHitsOrTimeout(ctx, p, t.ID, timeout, agent)
		_ = agent.Stop(runner.DefaultStopGrace)
		durations.set(t.ID, time.Since(start).Milliseconds())
	}
	return nil
}

// durationTracker is a mutex-guarded map[testID]ms populated by the
// orchestrator and consumed by buildChaosReport. A plain map is fine
// because the orchestrator writes sequentially and the reader runs
// only after the orchestrator goroutine has returned — the mutex
// guards the (brief) concurrent window where the TUI is being torn
// down.
type durationTracker struct {
	mu sync.Mutex
	m  map[string]int64
}

func newDurationTracker() *durationTracker { return &durationTracker{m: map[string]int64{}} }

func (d *durationTracker) set(id string, ms int64) {
	d.mu.Lock()
	d.m[id] = ms
	d.mu.Unlock()
}

func (d *durationTracker) get(id string) int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.m[id]
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
