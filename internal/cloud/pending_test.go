package cloud_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ruptor-dev/cli/internal/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWritePending_LandsInDirAndRoundtrips(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pending")
	body, _ := json.Marshal(map[string]any{"score": 0.67, "passed": 2})

	path, err := cloud.WritePending(dir, "chaos-test-1", body)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "chaos-test-1.json"), path)

	read, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, string(body), string(read))
}

func TestWritePending_CreatesMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deep", "nest", "pending")
	_, err := cloud.WritePending(dir, "id1", []byte(`{}`))
	require.NoError(t, err)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	if runtime.GOOS != "windows" {
		// Spec mandates 0700 for the spool dir so a multi-user box
		// does not leak pending reports across accounts.
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

func TestWritePending_FilePermsAre0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix permission bits not enforced on windows")
	}
	dir := t.TempDir()
	path, err := cloud.WritePending(dir, "id1", []byte(`{}`))
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestWritePending_AtomicRename(t *testing.T) {
	// After WritePending returns successfully there must be exactly
	// one file in the dir (the renamed final) and zero leftover
	// dotfile temps. A botched implementation that forgets to clean
	// up the temp would leave a hidden ".pending.*.json" behind.
	dir := t.TempDir()
	_, err := cloud.WritePending(dir, "atomic", []byte(`{"ok":true}`))
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "atomic.json", entries[0].Name())
	assert.False(t, strings.HasPrefix(entries[0].Name(), "."))
}

func TestWritePending_OverwritesExistingFile(t *testing.T) {
	// A rerun with the same runID (vanishingly unlikely with the
	// nanosecond + random suffix in NewRunID, but still) must
	// replace the prior file rather than fail. os.Rename on POSIX
	// is atomic over an existing target.
	dir := t.TempDir()
	_, err := cloud.WritePending(dir, "dup", []byte(`{"v":1}`))
	require.NoError(t, err)
	_, err = cloud.WritePending(dir, "dup", []byte(`{"v":2}`))
	require.NoError(t, err)

	read, err := os.ReadFile(filepath.Join(dir, "dup.json"))
	require.NoError(t, err)
	assert.Contains(t, string(read), `"v":2`)
}

func TestWritePending_RejectsEmptyArgs(t *testing.T) {
	_, err := cloud.WritePending("", "id", []byte(`{}`))
	require.Error(t, err)
	_, err = cloud.WritePending(t.TempDir(), "", []byte(`{}`))
	require.Error(t, err)
}

func TestNewRunID_IsUnique(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 50; i++ {
		id := cloud.NewRunID("chaos")
		assert.True(t, strings.HasPrefix(id, "chaos-"))
		_, dup := seen[id]
		assert.False(t, dup, "NewRunID returned a duplicate: %s", id)
		seen[id] = struct{}{}
	}
}
