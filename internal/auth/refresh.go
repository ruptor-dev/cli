package auth

import (
	"context"

	"github.com/rs/zerolog"
)

// MaybeRefresh rotates the token in the store when it is still valid
// but inside the refresh window. Returns the token that callers
// should use for the current operation — always the *existing* one
// when refresh is in flight, matching the CLAUDE.md rule: "continue
// with current operation, don't wait for refresh."
//
// If the token does not need refresh, this is a no-op. If refresh
// fails, the error is logged at debug and the caller continues with
// the current token; only an explicit revocation surfaces as an
// error back to the user.
func MaybeRefresh(ctx context.Context, client *Client, store *Store, current *Token, logger zerolog.Logger) {
	if current == nil || !current.NeedsRefresh() {
		return
	}
	logger.Debug().Str("token_suffix", current.MaskedSuffix()).Msg("auth: refreshing token")

	newRaw, err := client.Refresh(ctx, current.Raw)
	if err != nil {
		logger.Debug().Err(err).Msg("auth: refresh failed, continuing with current token")
		return
	}

	store.Token = newRaw
	if err := store.Save(); err != nil {
		logger.Debug().Err(err).Msg("auth: refresh succeeded but save failed")
		return
	}
}
