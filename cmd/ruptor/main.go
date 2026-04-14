package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

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

const version = "0.1.0"

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ruptor",
		Short: "Ruptor - chaos testing and simulation for AI agents",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))
			slog.SetDefault(logger)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newSimulateCmd())
	cmd.AddCommand(newValidateCmd())
	cmd.AddCommand(newVersionCmd())

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
	logger := slog.Default()

	cfg, err := config.LoadChaos(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Filter tests if --test is set.
	tests := cfg.Tests
	if testFilter != "" {
		var filtered []config.TestConfig
		for _, t := range tests {
			if t.ID == testFilter {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("no test found with id %q", testFilter)
		}
		tests = filtered
	}

	// Build dependencies.
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

	// Determine output format and path.
	format := cfg.Output.Format
	path := cfg.Output.Path
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
	renderer := report.NewRendererFromFormat(format, path)

	// Set up graceful shutdown.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start proxy in background.
	proxyErrCh := make(chan error, 1)
	go func() {
		proxyErrCh <- p.Start(ctx)
	}()

	logger.Info("ruptor chaos proxy starting",
		slog.String("agent", cfg.Agent.Name),
		slog.Int("tests", len(tests)),
		slog.Int("port", cfg.Proxy.Port),
	)

	// MVP: wait for context cancellation (agent orchestration is out of scope).
	// The proxy runs and intercepts requests until the user stops the process.
	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-proxyErrCh:
		if err != nil {
			return fmt.Errorf("proxy error: %w", err)
		}
	}

	// Placeholder: in the full version the evaluator and renderer would be
	// called after the agent has completed its run. For now we render an
	// empty report to verify wiring.
	_ = eval
	_ = renderer

	return nil
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
	logger := slog.Default()

	cfg, err := config.LoadSimulate(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Filter simulations if --sim is set.
	sims := cfg.Simulations
	if simFilter != "" {
		var filtered []config.Simulation
		for _, s := range sims {
			if s.ID == simFilter {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("no simulation found with id %q", simFilter)
		}
		sims = filtered
	}

	// Build dependencies.
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable is required for simulations")
	}

	llmClient := &simulateLLMAdapter{
		client: llmclient.New(apiKey, "",
			llmclient.WithHTTPClient(&http.Client{Timeout: 120 * time.Second}),
			llmclient.WithLogger(logger),
		),
	}

	judge, err := buildJudge(true, logger)
	if err != nil {
		return err
	}

	simEval := evaluator.NewSimulateEvaluator(judge, logger)
	timeout := cfg.Agent.RequestTimeoutS
	if timeout <= 0 {
		timeout = 30
	}
	sim := simulate.NewSimulator(llmClient, &http.Client{Timeout: 60 * time.Second}, logger, cfg.Agent.BaseURL, timeout)

	// Determine output format and path.
	format := cfg.Output.Format
	path := cfg.Output.Path
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
	renderer := report.NewRendererFromFormat(format, path)

	// Set up graceful shutdown.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("ruptor simulation starting",
		slog.String("agent", cfg.Agent.Name),
		slog.Int("simulations", len(sims)),
	)

	// Run each simulation.
	var results []types.SimulationResult
	for _, s := range sims {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("cancelled: %w", err)
		}

		logger.Info("running simulation", slog.String("id", s.ID), slog.String("persona", s.Persona))

		result, err := sim.Run(ctx, s)
		if err != nil {
			return fmt.Errorf("simulation %s: %w", s.ID, err)
		}

		// Evaluate the simulation if judge prompt is configured.
		if cfg.Evaluation.LLMJudgePrompt != "" {
			history := result.History
			if history == nil {
				history = &types.ConversationHistory{}
			}
			evalResult, err := simEval.Evaluate(
				ctx,
				s.ID,
				s.Persona,
				s.Goal,
				history,
				cfg.Evaluation.LLMJudgePrompt,
			)
			if err != nil {
				logger.Warn("evaluation failed", slog.String("id", s.ID), slog.String("error", err.Error()))
			} else {
				result.QualityScore = evalResult.QualityScore
				result.Issues = evalResult.Issues
			}
		}

		results = append(results, *result)
	}

	// Build and render report.
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

	rpt := &types.ConversationReport{
		SchemaVersion: types.ReportSchemaVersion,
		RuptorVersion: version,
		AgentName:     cfg.Agent.Name,
		RunAt:         time.Now(),
		TotalSims:     len(results),
		GoalReached:   goalReached,
		AvgScore:      avgScore,
		Results:       results,
	}

	if err := renderer.RenderSimulate(rpt); err != nil {
		return fmt.Errorf("rendering report: %w", err)
	}

	logger.Info("simulation complete",
		slog.Int("total", len(results)),
		slog.Int("goal_reached", goalReached),
		slog.Float64("avg_score", avgScore),
	)

	return nil
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
	logger := slog.Default()
	_, chaosErr := config.LoadChaos(cfgPath)
	if chaosErr == nil {
		logger.Info("config valid", slog.String("type", "chaos"))
		return nil
	}

	_, simErr := config.LoadSimulate(cfgPath)
	if simErr == nil {
		logger.Info("config valid", slog.String("type", "simulate"))
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
		},
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// buildJudge creates the appropriate LLM judge based on configuration.
func buildJudge(useLLM bool, logger *slog.Logger) (llmjudge.Judge, error) {
	if !useLLM {
		return &llmjudge.NoopJudge{}, nil
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		logger.Warn("OPENAI_API_KEY not set, using noop judge")
		return &llmjudge.NoopJudge{}, nil
	}

	return llmjudge.NewOpenAIJudge(apiKey, "", logger), nil
}

// ---------------------------------------------------------------------------
// simulateLLMAdapter bridges simulate.LLMClient (map-shaped messages) to the
// shared llmclient.OpenAIClient.
// ---------------------------------------------------------------------------

type simulateLLMAdapter struct {
	client *llmclient.OpenAIClient
}

func (a *simulateLLMAdapter) Complete(ctx context.Context, messages []map[string]string, model string) (string, error) {
	return a.client.CompleteMap(ctx, messages, model)
}
