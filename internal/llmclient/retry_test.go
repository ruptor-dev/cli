package llmclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ruptor-dev/cli/internal/llmclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetry_Succeeds_After_5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"upstream"}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"recovered"}}]}`))
	}))
	defer srv.Close()

	c := llmclient.New("k", "", llmclient.WithURL(srv.URL), llmclient.WithRetry())
	out, err := c.Complete(context.Background(),
		[]llmclient.Message{{Role: "user", Content: "hi"}}, "")
	require.NoError(t, err)
	assert.Equal(t, "recovered", out)
	assert.Equal(t, int32(3), calls.Load(), "two failures then success")
}

func TestRetry_Succeeds_After_429(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate"}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	c := llmclient.New("k", "", llmclient.WithURL(srv.URL), llmclient.WithRetry())
	out, err := c.Complete(context.Background(),
		[]llmclient.Message{{Role: "user", Content: "hi"}}, "")
	require.NoError(t, err)
	assert.Equal(t, "ok", out)
}

func TestRetry_GivesUp_OnPermanent4xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer srv.Close()

	c := llmclient.New("k", "", llmclient.WithURL(srv.URL), llmclient.WithRetry())
	_, err := c.Complete(context.Background(),
		[]llmclient.Message{{Role: "user", Content: "hi"}}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 401")
	assert.Equal(t, int32(1), calls.Load(), "permanent 4xx must not retry")
}

func TestRetry_Disabled_BehavesAsBefore(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := llmclient.New("k", "", llmclient.WithURL(srv.URL)) // no WithRetry
	_, err := c.Complete(context.Background(),
		[]llmclient.Message{{Role: "user", Content: "hi"}}, "")
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load(), "without WithRetry, 5xx is a one-shot failure")
}
