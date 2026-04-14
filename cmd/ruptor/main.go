package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/evaluator"
	"github.com/ruptor-dev/cli/internal/evaluator/llmjudge"
	"github.com/ruptor-dev/cli/internal/llmclient"
	"github.com/ruptor-dev/cli/internal/proxy"
	"github.com/ruptor-dev/cli/internal/proxy/faults"
	"github.com/ruptor-dev/cli/internal/report"
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
		Use:   "ruptor",
		Short: "Ruptor - chaos testing and simulation for AI agents",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			rootLogger = ui.NewLogger(ui.LogLevel(logLevel))
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")

	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newSimulateCmd())
	cmd.AddCommand(newValidateCmd())
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newAuthCmd())

	return cmd
}

// ---------------------------------------------------------------------------
// run command
// ---------------------------------------------------------------------------

func newRunCmd() *cobra.Command {
	var outputPath string
	var testFilter string

	cmd := &cobra.Command{
		Use:   "run <config-file>",
		Short: "Run chaos tests against an AI agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChaos(cmd.Context(), args[0], outputPath, testFilter)
		},
	}

	cmd.Flags().StringVar(&outputPath, "output", "", "output file path for the report")
	cmd.Flags().StringVar(&testFilter, "test", "", "run only the test with this ID")

	return cmd
}

func runChaos(ctx context.Context, cfgPath, outputPath, testFilter string) error {
	logger := rootLogger

	cfg, err := config.LoadChaos(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	tests, err := filterTests(cfg.Tests, testFilter)
	if err != nil {
		return err
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

	if err := runChaosTUI(ctx, stop, cfg, tests, p); err != nil {
		return err
	}

	if err := waitForProxy(ctx, proxyErrCh, logger); err != nil {
		return err
	}

	rpt, err := buildChaosReport(context.Background(), eval, cfg, tests, p.Observations(), logger)
	if err != nil {
		return err
	}

	if err := renderer.RenderChaos(rpt); err != nil {
		return fmt.Errorf("rendering chaos report: %w", err)
	}

	ui.PrintCompletion(ui.CompletionSummary{
		ScorePercent: rpt.Score,
		Passed:       rpt.Passed,
		Failed:       rpt.Failed,
		ReportPaths:  reportPathsFor(renderer, cfg.Output, outputPath),
	})

	logger.Info().
		Int("total", rpt.TotalTests).
		Int("passed", rpt.Passed).
		Int("failed", rpt.Failed).
		Int("score", rpt.Score).
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
		out = append(out, path+"chaos_report.html")
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
	logger zerolog.Logger,
) (*types.ReliabilityReport, error) {
	results := make([]types.TestResult, 0, len(tests))
	passed, failed := 0, 0

	for _, t := range tests {
		r := evaluateTest(ctx, eval, t, obs[t.ID], cfg.Evaluation.LLMJudgePrompt, logger)
		if r.Passed {
			passed++
		} else {
			failed++
		}
		results = append(results, *r)
	}

	score := 0
	if len(tests) > 0 {
		score = (passed * 100) / len(tests)
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
		false, // Recovered — wired when retry-aware proxy lands
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
			ui.Printf("ruptor %s\n", version)
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
