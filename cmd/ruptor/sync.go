package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ruptor-dev/cli/internal/cloud"
	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/ui"
	"github.com/spf13/cobra"
)

const waitlistMessage = "Cloud reporting is coming soon. Join the waitlist at https://ruptor.dev"

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Upload pending local reports to the cloud",
		Long: "Walks ~/.ruptor/pending/ and uploads each report. " +
			"Successful uploads are removed; failures are left in " +
			"place so the next sync retries them.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd.Context())
		},
	}
}

func runSync(ctx context.Context) error {
	// Gate is checked before any I/O so the waitlist message fires
	// even if the user has no config dir or has never logged in.
	if !cloud.CloudReportingEnabled {
		ui.Info(waitlistMessage)
		return nil
	}

	settings, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}
	pendingDir := filepath.Join(settings.ConfigDir, "pending")

	client := cloud.NewClient(
		cloud.WithBaseURL(settings.CloudURL),
		cloud.WithLogger(rootLogger),
	)

	result, err := cloud.Sync(ctx, cloud.SyncOptions{
		Enabled:    cloud.CloudReportingEnabled,
		PendingDir: pendingDir,
		Client:     client,
		Logger:     rootLogger,
	})
	if err != nil {
		return err
	}
	renderSyncResult(result)
	return nil
}

func renderSyncResult(r cloud.SyncResult) {
	if r.Skipped {
		ui.Info(waitlistMessage)
		return
	}
	if r.Empty {
		ui.Info("Nothing to sync.")
		return
	}
	for _, f := range r.Files {
		if f.OK {
			ui.Success(fmt.Sprintf("Uploaded %s", f.Name))
			continue
		}
		ui.Warning(fmt.Sprintf("Kept %s in pending/ — %s", f.Name, f.Error))
	}
	ui.Println("")
	ui.Info(fmt.Sprintf("%d uploaded, %d remaining in pending/", r.Uploaded(), r.Failed()))
}
