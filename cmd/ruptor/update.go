package main

import (
	"context"
	"fmt"

	"github.com/ruptor-dev/cli/internal/ui"
	"github.com/ruptor-dev/cli/internal/updater"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Check whether a newer ruptor release is available",
		Long: "Queries the GitHub releases feed and prints the upgrade " +
			"command when a newer tag exists. Non-blocking: a network " +
			"failure produces a muted warning and exits 0.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(cmd.Context())
		},
	}
}

func runUpdate(ctx context.Context) error {
	r := updater.Check(ctx, version, updater.Options{})
	renderUpdateResult(r)
	return nil
}

func renderUpdateResult(r updater.Result) {
	switch r.Status {
	case updater.StatusUpToDate:
		ui.Success(fmt.Sprintf("ruptor is up to date (%s)", r.Current))
	case updater.StatusBehind:
		ui.Info(fmt.Sprintf("ruptor %s available (current: %s)", r.Latest, r.Current))
		if r.UpgradeHint != "" {
			ui.Println("")
			ui.Println("  " + r.UpgradeHint)
			ui.Println("")
		}
	case updater.StatusUnknown:
		ui.Dim(fmt.Sprintf("could not check for updates: %s", r.Reason))
	}
}
