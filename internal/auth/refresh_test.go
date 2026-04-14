package auth_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func silentLog() zerolog.Logger {
	return zerolog.New(io.Discard).Level(zerolog.Disabled)
}

func TestMaybeRefresh_NoOpWhenOutsideWindow(t *testing.T) {
	k := loadTestKey(t)
	// 30 days out — well outside the 7-day refresh window.
	raw := signTestJWT(t, k, testClaims{
		TokenType: auth.TokenTypePersonal,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour).Unix(),
	})
	tok, err := auth.Parse(raw, k.public)
	require.NoError(t, err)

	var hit atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit.Add(1)
	}))
	defer srv.Close()

	client := auth.NewClient(auth.WithBaseURL(srv.URL))
	store := &auth.Store{Token: raw}
	auth.MaybeRefresh(context.Background(), client, store, tok, silentLog())
	assert.Equal(t, int32(0), hit.Load(), "must not touch the network outside the refresh window")
}

func TestMaybeRefresh_RotatesAndPersistsInsideWindow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	k := loadTestKey(t)
	// 3 days remaining — inside the 7-day window.
	oldRaw := signTestJWT(t, k, testClaims{
		TokenType: auth.TokenTypePersonal,
		ExpiresAt: time.Now().Add(3 * 24 * time.Hour).Unix(),
	})
	oldTok, err := auth.Parse(oldRaw, k.public)
	require.NoError(t, err)

	// Platform returns a freshly-signed JWT with a longer exp.
	newRaw := signTestJWT(t, k, testClaims{
		TokenType: auth.TokenTypePersonal,
		ExpiresAt: time.Now().Add(90 * 24 * time.Hour).Unix(),
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"token": newRaw})
	}))
	defer srv.Close()

	client := auth.NewClient(auth.WithBaseURL(srv.URL))
	store := &auth.Store{Token: oldRaw}
	// Seed the on-disk config so the Save path has something to rewrite.
	require.NoError(t, store.Save())

	auth.MaybeRefresh(context.Background(), client, store, oldTok, silentLog())
	assert.Equal(t, newRaw, store.Token, "store should hold the freshly-issued token")

	// Disk must match, still 0600.
	path := filepath.Join(dir, ".ruptor", "config.yaml")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	reloaded, err := auth.LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, newRaw, reloaded.Token)
}

func TestMaybeRefresh_SwallowsRefreshErrors(t *testing.T) {
	k := loadTestKey(t)
	raw := signTestJWT(t, k, testClaims{
		TokenType: auth.TokenTypePersonal,
		ExpiresAt: time.Now().Add(3 * 24 * time.Hour).Unix(),
	})
	tok, err := auth.Parse(raw, k.public)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream flaked", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := auth.NewClient(auth.WithBaseURL(srv.URL))
	store := &auth.Store{Token: raw}
	// Must not panic, must not mutate store.
	auth.MaybeRefresh(context.Background(), client, store, tok, silentLog())
	assert.Equal(t, raw, store.Token, "failed refresh must leave the token untouched")
}
