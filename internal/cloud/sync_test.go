package cloud_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func silentLogger() zerolog.Logger { return zerolog.Nop() }

// fastClient builds a Client whose retry budget is small enough to
// make a deliberately failing test finish in well under a second.
func fastClient(t *testing.T, baseURL string, hc *http.Client) *cloud.Client {
	t.Helper()
	return cloud.NewClient(
		cloud.WithBaseURL(baseURL),
		cloud.WithHTTPClient(hc),
		cloud.WithLogger(silentLogger()),
		cloud.WithRetryPolicy(5*time.Millisecond, 20*time.Millisecond, 100*time.Millisecond),
	)
}

func writePending(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
}

func TestSync_DisabledShowsWaitlist(t *testing.T) {
	res, err := cloud.Sync(context.Background(), cloud.SyncOptions{
		Enabled: false,
	})
	require.NoError(t, err)
	assert.True(t, res.Skipped)
	assert.False(t, res.Empty)
}

func TestSync_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("must not hit the network when nothing is pending")
	}))
	defer srv.Close()

	res, err := cloud.Sync(context.Background(), cloud.SyncOptions{
		Enabled:    true,
		PendingDir: dir,
		Client:     fastClient(t, srv.URL, srv.Client()),
		Logger:     silentLogger(),
	})
	require.NoError(t, err)
	assert.True(t, res.Empty)
	assert.Equal(t, 0, res.Uploaded())
}

func TestSync_MissingDirIsEmpty(t *testing.T) {
	// A user who has never produced a report yet may not have the
	// pending dir at all — sync must treat that as "Nothing to sync."
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	res, err := cloud.Sync(context.Background(), cloud.SyncOptions{
		Enabled:    true,
		PendingDir: dir,
		Client:     cloud.NewClient(),
		Logger:     silentLogger(),
	})
	require.NoError(t, err)
	assert.True(t, res.Empty)
}

func TestSync_UploadSuccessDeletesFile(t *testing.T) {
	dir := t.TempDir()
	writePending(t, dir, "a.json", `{"id":"a"}`)
	writePending(t, dir, "b.json", `{"id":"b"}`)
	// Hidden + non-json must be ignored, never uploaded.
	writePending(t, dir, ".tmp.json", `partial`)
	writePending(t, dir, "notes.txt", `ignore me`)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		assert.Equal(t, "/v1/runs", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res, err := cloud.Sync(context.Background(), cloud.SyncOptions{
		Enabled:    true,
		PendingDir: dir,
		Client:     fastClient(t, srv.URL, srv.Client()),
		Logger:     silentLogger(),
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Uploaded())
	assert.Equal(t, 0, res.Failed())
	assert.Equal(t, int32(2), hits.Load())

	// Successful uploads removed.
	_, err = os.Stat(filepath.Join(dir, "a.json"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, "b.json"))
	assert.True(t, os.IsNotExist(err))
	// Hidden + non-json untouched.
	_, err = os.Stat(filepath.Join(dir, ".tmp.json"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "notes.txt"))
	assert.NoError(t, err)
}

func TestSync_UploadFailureKeepsFileAndRetries(t *testing.T) {
	dir := t.TempDir()
	writePending(t, dir, "broken.json", `{"id":"x"}`)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	res, err := cloud.Sync(context.Background(), cloud.SyncOptions{
		Enabled:    true,
		PendingDir: dir,
		Client:     fastClient(t, srv.URL, srv.Client()),
		Logger:     silentLogger(),
	})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Uploaded())
	assert.Equal(t, 1, res.Failed())
	assert.Greater(t, hits.Load(), int32(1), "503 must be retried at least once before giving up")

	// File preserved for the next sync to retry.
	_, err = os.Stat(filepath.Join(dir, "broken.json"))
	assert.NoError(t, err)
}

func TestSync_Permanent4xxShortCircuits(t *testing.T) {
	// 400 means "your payload is broken" — retrying does nothing but
	// burn the budget. The file is still kept so the user can inspect
	// it, but the server must only see a single request.
	dir := t.TempDir()
	writePending(t, dir, "bad.json", `{"id":"y"}`)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	res, err := cloud.Sync(context.Background(), cloud.SyncOptions{
		Enabled:    true,
		PendingDir: dir,
		Client:     fastClient(t, srv.URL, srv.Client()),
		Logger:     silentLogger(),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed())
	assert.Equal(t, int32(1), hits.Load(), "permanent 4xx must not be retried")
}

func TestSync_MixedFiles(t *testing.T) {
	dir := t.TempDir()
	writePending(t, dir, "good.json", `{"ok":true}`)
	writePending(t, dir, "fail.json", `{"ok":false}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decide outcome from the body so the test does not depend
		// on filename → request mapping (sort order).
		buf := make([]byte, 64)
		n, _ := r.Body.Read(buf)
		if string(buf[:n]) == `{"ok":true}` {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	res, err := cloud.Sync(context.Background(), cloud.SyncOptions{
		Enabled:    true,
		PendingDir: dir,
		Client:     fastClient(t, srv.URL, srv.Client()),
		Logger:     silentLogger(),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Uploaded())
	assert.Equal(t, 1, res.Failed())

	_, err = os.Stat(filepath.Join(dir, "good.json"))
	assert.True(t, os.IsNotExist(err), "successful upload should be removed")
	_, err = os.Stat(filepath.Join(dir, "fail.json"))
	assert.NoError(t, err, "failed upload should be kept")
}
