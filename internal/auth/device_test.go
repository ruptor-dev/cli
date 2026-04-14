package auth_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ruptor-dev/cli/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevice_StartDevice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/auth/device/code", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "dev_xyz",
			"user_code":        "ABCD-1234",
			"verification_url": "https://ruptor.dev/device",
			"expires_in":       300,
			"interval":         1,
		})
	}))
	defer srv.Close()

	c := auth.NewClient(auth.WithBaseURL(srv.URL))
	got, err := c.StartDevice(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "dev_xyz", got.DeviceCode)
	assert.Equal(t, "ABCD-1234", got.UserCode)
	assert.Equal(t, 300, got.ExpiresIn)
}

func TestDevice_PollToken_Success(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "JWT"})
	}))
	defer srv.Close()

	c := auth.NewClient(auth.WithBaseURL(srv.URL))
	tok, err := c.PollToken(context.Background(), "dev_xyz",
		10*time.Millisecond, 2*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "JWT", tok)
	assert.GreaterOrEqual(t, calls.Load(), int32(3))
}

func TestDevice_PollToken_Denied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "access_denied"})
	}))
	defer srv.Close()

	c := auth.NewClient(auth.WithBaseURL(srv.URL))
	_, err := c.PollToken(context.Background(), "dev_xyz",
		10*time.Millisecond, 2*time.Second)
	require.ErrorIs(t, err, auth.ErrDeviceDenied)
}

func TestDevice_PollToken_ExpiredToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "expired_token"})
	}))
	defer srv.Close()

	c := auth.NewClient(auth.WithBaseURL(srv.URL))
	_, err := c.PollToken(context.Background(), "dev_xyz",
		10*time.Millisecond, 2*time.Second)
	require.ErrorIs(t, err, auth.ErrDeviceTimeout)
}

func TestDevice_PollToken_DeadlineReached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	}))
	defer srv.Close()

	c := auth.NewClient(auth.WithBaseURL(srv.URL))
	_, err := c.PollToken(context.Background(), "dev_xyz",
		10*time.Millisecond, 50*time.Millisecond)
	require.ErrorIs(t, err, auth.ErrDeviceTimeout)
}

func TestDevice_PollToken_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
	}))
	defer srv.Close()

	c := auth.NewClient(auth.WithBaseURL(srv.URL))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	_, err := c.PollToken(ctx, "dev_xyz",
		10*time.Millisecond, 10*time.Second)
	require.ErrorIs(t, err, context.Canceled)
}

func TestClient_Refresh_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/auth/refresh", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "NEW_JWT"})
	}))
	defer srv.Close()

	c := auth.NewClient(auth.WithBaseURL(srv.URL))
	newTok, err := c.Refresh(context.Background(), "OLD_JWT")
	require.NoError(t, err)
	assert.Equal(t, "NEW_JWT", newTok)
}

func TestClient_Refresh_Revoked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "revoked"})
	}))
	defer srv.Close()

	c := auth.NewClient(auth.WithBaseURL(srv.URL))
	_, err := c.Refresh(context.Background(), "OLD")
	require.ErrorIs(t, err, auth.ErrTokenRevoked)
}

func TestClient_Revoke(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		assert.Equal(t, "/v1/auth/revoke", r.URL.Path)
		fmt.Fprintln(w, "{}")
	}))
	defer srv.Close()

	c := auth.NewClient(auth.WithBaseURL(srv.URL))
	require.NoError(t, c.Revoke(context.Background(), "OLD"))
	assert.True(t, hit)
}
