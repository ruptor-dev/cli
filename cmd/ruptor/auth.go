package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/ruptor-dev/cli/internal/auth"
	"github.com/ruptor-dev/cli/internal/ui"
	"github.com/spf13/cobra"
)

// deviceLoginTimeout is the hard deadline for the whole login flow.
// Matches SKILL-auth.md's 5-minute envelope.
const deviceLoginTimeout = 5 * time.Minute

// newAuthCmd wires `ruptor auth login|status|logout`. The subcommands
// share a single *auth.Client but each reads and writes the Store on
// its own to stay simple.
func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate the CLI against the Ruptor platform",
	}
	cmd.AddCommand(newAuthLoginCmd())
	cmd.AddCommand(newAuthLogoutCmd())
	cmd.AddCommand(newAuthStatusCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in via OAuth device flow",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogin(cmd.Context())
		},
	}
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke and remove the local token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogout(cmd.Context())
		},
	}
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current authentication status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthStatus()
		},
	}
}

func runAuthLogin(ctx context.Context) error {
	client := auth.NewClient()
	ctx, cancel := context.WithTimeout(ctx, deviceLoginTimeout)
	defer cancel()

	device, err := client.StartDevice(ctx)
	if err != nil {
		return err
	}

	ui.Info(fmt.Sprintf("Visit this URL to authorize: %s", device.VerificationURL))
	ui.Info(fmt.Sprintf("Code: %s", device.UserCode))
	if err := tryOpenBrowser(device.VerificationURL); err != nil {
		ui.Dim("(could not auto-open browser — copy the URL above)")
	}

	raw, err := client.PollToken(ctx,
		device.DeviceCode,
		time.Duration(device.Interval)*time.Second,
		time.Duration(device.ExpiresIn)*time.Second,
	)
	if err != nil {
		return err
	}

	token, err := auth.Parse(raw, auth.PublicKeyPEM)
	if err != nil {
		return err
	}

	store := &auth.Store{Token: raw}
	if err := store.Save(); err != nil {
		return err
	}

	ui.Success(fmt.Sprintf("Authenticated as %s (%s plan)", token.Claims.UserEmail, token.Claims.Plan))
	return nil
}

func runAuthLogout(ctx context.Context) error {
	store, err := auth.Load()
	if err != nil && !errors.Is(err, auth.ErrNotAuthenticated) {
		return err
	}

	// Best-effort remote revoke; a network failure must not block
	// local cleanup — users expect logout to always succeed.
	if store != nil && store.Token != "" {
		client := auth.NewClient()
		revokeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := client.Revoke(revokeCtx, store.Token); err != nil {
			rootLogger.Debug().Err(err).Msg("auth: revoke failed")
		}
		cancel()
	}

	if err := auth.Clear(); err != nil {
		return err
	}
	ui.Success("Logged out. Local credentials removed.")
	return nil
}

func runAuthStatus() error {
	store, err := auth.Load()
	if err != nil {
		msg, _ := ui.AuthMessage(err)
		ui.Error(msg)
		return nil
	}
	token, err := auth.Parse(store.Token, auth.PublicKeyPEM)
	if err != nil {
		msg, _ := ui.AuthMessage(err)
		ui.Error(msg)
		return nil
	}
	renderAuthStatus(token)
	return nil
}

func renderAuthStatus(token *auth.Token) {
	ui.Println("")
	ui.Println("  Authentication")
	ui.Println("  ──────────────────────────────────")
	ui.Printf("  User     %s\n", token.Claims.UserEmail)
	ui.Printf("  Plan     %s\n", title(token.Claims.Plan))
	ui.Printf("  Expires  %s\n", formatExpiry(token.Claims.ExpiresAt))
	ui.Printf("  Token    %s  (%s)\n", token.MaskedSuffix(), token.Claims.TokenType)
	ui.Println("")
}

func formatExpiry(unix int64) string {
	if unix == 0 {
		return "never"
	}
	at := time.Unix(unix, 0)
	remaining := time.Until(at)
	if remaining <= 0 {
		return fmt.Sprintf("expired (%s)", at.Format("Jan 2, 2006"))
	}
	days := int(remaining.Hours() / 24)
	return fmt.Sprintf("in %d days  (%s)", days, at.Format("Jan 2, 2006"))
}

func title(s string) string {
	if s == "" {
		return "—"
	}
	return string(s[0]-('a'-'A')) + s[1:]
}

// tryOpenBrowser shells out to the OS-appropriate opener. Failure is
// non-fatal — the URL has already been printed.
func tryOpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported os")
	}
	return cmd.Start()
}
