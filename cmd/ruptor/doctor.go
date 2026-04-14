package main

import (
	"context"
	"fmt"

	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/doctor"
	"github.com/ruptor-dev/cli/internal/ui"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run a preflight check of the local install",
		Long: "Inspects Go runtime, config permissions, proxy port, " +
			"cloud reachability, and stored auth token. The output is " +
			"the canonical thing to paste in a bug report.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.Context())
		},
	}
}

func runDoctor(ctx context.Context) error {
	opts := doctor.Options{}
	if s, err := config.LoadSettings(); err == nil {
		opts.ConfigDir = s.ConfigDir
		opts.CloudURL = s.CloudURL
	}
	results := doctor.Run(ctx, opts)
	renderDoctorResults(results)
	if doctor.HasFailures(results) {
		return fmt.Errorf("one or more checks failed")
	}
	return nil
}

func renderDoctorResults(results []doctor.Result) {
	ui.Println("")
	ui.Println("  Ruptor doctor")
	ui.Println("  ──────────────────────────────────")
	for _, r := range results {
		renderDoctorResult(r)
	}
	ui.Println("")
}

func renderDoctorResult(r doctor.Result) {
	line := fmt.Sprintf("%s — %s", r.Name, r.Message)
	switch r.Status {
	case doctor.StatusOK:
		ui.Success(line)
	case doctor.StatusWarn:
		ui.Warning(line)
		if r.Hint != "" {
			ui.Dim("    " + r.Hint)
		}
	case doctor.StatusFail:
		ui.Error(line)
		if r.Hint != "" {
			ui.Dim("    " + r.Hint)
		}
	}
}
