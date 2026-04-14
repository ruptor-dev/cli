package cloud

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PendingDirName is the subdirectory under ConfigDir that holds
// reports awaiting upload. Kept exported so cmd/ruptor and tests can
// build the same path.
const PendingDirName = "pending"

// WritePending atomically writes body to <dir>/<runID>.json with
// 0600 permissions. Creates the directory tree with 0700 if missing.
// Returns the absolute path written.
//
// The atomic write follows the same temp-file + rename pattern as the
// auth Store: a crash mid-write leaves no half-written report in the
// spool that the next `ruptor sync` could attempt to upload.
func WritePending(dir, runID string, body []byte) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("cloud: WritePending: dir is required")
	}
	if runID == "" {
		return "", fmt.Errorf("cloud: WritePending: runID is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("cloud: creating %s: %w", dir, err)
	}

	final := filepath.Join(dir, runID+".json")

	// CreateTemp prefixes "." so the file is hidden from sync's
	// scanner (sync skips dotfiles) until we rename it into place.
	tmp, err := os.CreateTemp(dir, ".pending.*.json")
	if err != nil {
		return "", fmt.Errorf("cloud: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("cloud: chmod: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("cloud: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("cloud: close: %w", err)
	}
	if err := os.Rename(tmpPath, final); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("cloud: rename: %w", err)
	}
	return final, nil
}

// NewRunID builds a filename-safe identifier of the form
// "chaos-<unix-nanos>-<rand>". The random suffix avoids collisions
// when two runs land in the same nanosecond (rare on real hardware,
// trivially possible in a tight test loop).
func NewRunID(prefix string) string {
	var buf [4]byte
	_, _ = rand.Read(buf[:])
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UnixNano(), hex.EncodeToString(buf[:]))
}
