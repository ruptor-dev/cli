package updater_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruptor-dev/cli/internal/updater"
	"github.com/stretchr/testify/assert"
)

func releaseHandler(tag string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `","html_url":"https://github.com/ruptor-dev/cli/releases/tag/` + tag + `"}`))
	}
}

func TestCheck_UpToDate(t *testing.T) {
	srv := httptest.NewServer(releaseHandler("v1.2.3"))
	defer srv.Close()
	r := updater.Check(context.Background(), "1.2.3", updater.Options{URL: srv.URL, HTTPClient: srv.Client()})
	assert.Equal(t, updater.StatusUpToDate, r.Status)
	assert.Equal(t, "v1.2.3", r.Latest)
}

func TestCheck_UpToDateWithLeadingV(t *testing.T) {
	srv := httptest.NewServer(releaseHandler("v1.2.3"))
	defer srv.Close()
	r := updater.Check(context.Background(), "v1.2.3", updater.Options{URL: srv.URL, HTTPClient: srv.Client()})
	assert.Equal(t, updater.StatusUpToDate, r.Status)
}

func TestCheck_Behind(t *testing.T) {
	srv := httptest.NewServer(releaseHandler("v1.3.0"))
	defer srv.Close()
	r := updater.Check(context.Background(), "1.2.9", updater.Options{URL: srv.URL, HTTPClient: srv.Client()})
	assert.Equal(t, updater.StatusBehind, r.Status)
	assert.Equal(t, "v1.3.0", r.Latest)
	assert.Contains(t, r.UpgradeHint, "go install github.com/ruptor-dev/cli/cmd/ruptor@v1.3.0")
}

func TestCheck_DevBuildIsUnknown(t *testing.T) {
	srv := httptest.NewServer(releaseHandler("v1.0.0"))
	defer srv.Close()
	r := updater.Check(context.Background(), "0.0.0-dev", updater.Options{URL: srv.URL, HTTPClient: srv.Client()})
	assert.Equal(t, updater.StatusUnknown, r.Status)
	assert.NotEmpty(t, r.UpgradeHint, "dev builds still see an install hint, just labelled Unknown")
}

func TestCheck_GitHubError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	r := updater.Check(context.Background(), "1.0.0", updater.Options{URL: srv.URL, HTTPClient: srv.Client()})
	assert.Equal(t, updater.StatusUnknown, r.Status)
	assert.Contains(t, r.Reason, "403")
}

func TestCheck_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	r := updater.Check(context.Background(), "1.0.0", updater.Options{URL: srv.URL, HTTPClient: srv.Client()})
	assert.Equal(t, updater.StatusUnknown, r.Status)
}

func TestCheck_EmptyTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	r := updater.Check(context.Background(), "1.0.0", updater.Options{URL: srv.URL, HTTPClient: srv.Client()})
	assert.Equal(t, updater.StatusUnknown, r.Status)
	assert.Contains(t, r.Reason, "tag_name")
}
