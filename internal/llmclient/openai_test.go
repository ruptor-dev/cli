package llmclient_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/faultforge/faultforge/internal/llmclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIClient_Complete(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    string
		want       string
	}{
		{
			name:       "successful response",
			statusCode: http.StatusOK,
			body:       `{"choices":[{"message":{"content":"hello"}}]}`,
			want:       "hello",
		},
		{
			name:       "HTTP 401 returns error without decode",
			statusCode: http.StatusUnauthorized,
			body:       `{"error":{"message":"invalid key"}}`,
			wantErr:    "openai HTTP 401",
		},
		{
			name:       "HTTP 429 returns error without decode",
			statusCode: http.StatusTooManyRequests,
			body:       `{"error":{"message":"rate limit"}}`,
			wantErr:    "openai HTTP 429",
		},
		{
			name:       "API error in 200 body",
			statusCode: http.StatusOK,
			body:       `{"error":{"message":"model not found"}}`,
			wantErr:    "api error: model not found",
		},
		{
			name:       "empty choices",
			statusCode: http.StatusOK,
			body:       `{"choices":[]}`,
			wantErr:    "no choices in response",
		},
		{
			name:       "malformed JSON",
			statusCode: http.StatusOK,
			body:       `not-json`,
			wantErr:    "parsing response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				body, _ := io.ReadAll(r.Body)
				var req struct {
					Model    string              `json:"model"`
					Messages []llmclient.Message `json:"messages"`
				}
				require.NoError(t, json.Unmarshal(body, &req))
				assert.NotEmpty(t, req.Model)
				assert.NotEmpty(t, req.Messages)

				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := llmclient.New("test-key", "", llmclient.WithURL(srv.URL))
			got, err := c.Complete(context.Background(),
				[]llmclient.Message{{Role: "user", Content: "hi"}}, "")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOpenAIClient_CompleteMap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []llmclient.Message `json:"messages"`
		}
		require.NoError(t, json.Unmarshal(body, &req))
		assert.Equal(t, "system", req.Messages[0].Role)
		assert.Equal(t, "you are a bot", req.Messages[0].Content)

		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	c := llmclient.New("k", "m", llmclient.WithURL(srv.URL))
	got, err := c.CompleteMap(context.Background(),
		[]map[string]string{{"role": "system", "content": "you are a bot"}}, "")
	require.NoError(t, err)
	assert.Equal(t, "ok", got)
}

func TestOpenAIClient_Model(t *testing.T) {
	c := llmclient.New("k", "")
	assert.Equal(t, llmclient.DefaultModel, c.Model())

	c2 := llmclient.New("k", "gpt-4")
	assert.Equal(t, "gpt-4", c2.Model())
}

func TestOpenAIClient_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := llmclient.New("k", "", llmclient.WithURL(srv.URL))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Complete(ctx, []llmclient.Message{{Role: "user", Content: "x"}}, "")
	require.Error(t, err)
}
