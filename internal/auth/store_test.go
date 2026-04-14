package auth_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruptor-dev/cli/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_SaveCreates0600File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	s := &auth.Store{Token: "test-token"}
	require.NoError(t, s.SaveTo(path))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"config must be written 0600 so secrets don't leak to group/other")
}

func TestStore_LoadRejectsWorldReadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("token: foo\n"), 0o644))

	_, err := auth.LoadFrom(path)
	require.ErrorIs(t, err, auth.ErrBadConfigPerms)
}

func TestStore_LoadMissingReturnsNotAuthenticated(t *testing.T) {
	_, err := auth.LoadFrom(filepath.Join(t.TempDir(), "missing.yaml"))
	require.ErrorIs(t, err, auth.ErrNotAuthenticated)
}

func TestStore_RoundTripPreservesExtras(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	raw := "token: initial\ncloud:\n  endpoint: https://api.ruptor.dev\n  timeout: 30s\n"
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	s, err := auth.LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, "initial", s.Token)

	// Rotate the token and save — the cloud block from before must
	// survive the rewrite.
	s.Token = "rotated"
	require.NoError(t, s.SaveTo(path))

	reloaded, err := auth.LoadFrom(path)
	require.NoError(t, err)
	assert.Equal(t, "rotated", reloaded.Token)

	// Read the file directly and confirm the user's custom block is
	// still there.
	bytesOnDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(bytesOnDisk), "cloud:")
	assert.Contains(t, string(bytesOnDisk), "api.ruptor.dev")
}

func TestStore_SaveToCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested", "dir", "config.yaml")

	s := &auth.Store{Token: "x"}
	require.NoError(t, s.SaveTo(nested))

	info, err := os.Stat(filepath.Dir(nested))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(),
		"parent directory should be 0700 so neighbours cannot list it")
}
