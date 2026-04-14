package simulate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAgentResponse_AutoPriority(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "openai wins over anthropic",
			body: `{"choices":[{"message":{"content":"openai reply"}}],"content":[{"text":"anthropic reply"}]}`,
			want: "openai reply",
		},
		{
			name: "anthropic when no openai",
			body: `{"content":[{"text":"anthropic reply"}]}`,
			want: "anthropic reply",
		},
		{
			name: "response field fallback",
			body: `{"response":"plain response"}`,
			want: "plain response",
		},
		{
			name: "message field fallback",
			body: `{"message":"plain message"}`,
			want: "plain message",
		},
		{
			name: "content string fallback",
			body: `{"content":"plain content"}`,
			want: "plain content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAgentResponse([]byte(tt.body), "", "")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseAgentResponse_FormatOverride(t *testing.T) {
	body := `{"choices":[{"message":{"content":"openai"}}],"content":[{"text":"anthropic"}]}`

	openai, err := parseAgentResponse([]byte(body), config.ResponseFormatOpenAI, "")
	require.NoError(t, err)
	assert.Equal(t, "openai", openai)

	anthropic, err := parseAgentResponse([]byte(body), config.ResponseFormatAnthropic, "")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", anthropic)
}

func TestParseAgentResponse_CustomField(t *testing.T) {
	body := `{"reply":"weirdly named"}`

	got, err := parseAgentResponse([]byte(body), config.ResponseFormatCustom, "reply")
	require.NoError(t, err)
	assert.Equal(t, "weirdly named", got)

	_, err = parseAgentResponse([]byte(body), config.ResponseFormatCustom, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "response_field")
}

func TestParseAgentResponse_NonJSON(t *testing.T) {
	_, err := parseAgentResponse([]byte("not json at all"), "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-JSON")
}

func TestApplyAuth_Bearer(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://x", nil)
	err := applyAuth(req, config.AgentAuth{Type: config.AuthBearer, Token: "abc"})
	require.NoError(t, err)
	assert.Equal(t, "Bearer abc", req.Header.Get("Authorization"))
}

func TestApplyAuth_Basic(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://x", nil)
	err := applyAuth(req, config.AgentAuth{Type: config.AuthBasic, Username: "u", Token: "p"})
	require.NoError(t, err)
	// base64("u:p") == "dTpw"
	assert.Equal(t, "Basic dTpw", req.Header.Get("Authorization"))
}

func TestApplyAuth_Header(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://x", nil)
	err := applyAuth(req, config.AgentAuth{Type: config.AuthHeader, HeaderName: "X-Api-Key", Token: "secret"})
	require.NoError(t, err)
	assert.Equal(t, "secret", req.Header.Get("X-Api-Key"))
}

func TestApplyAuth_None(t *testing.T) {
	req, _ := http.NewRequest("POST", "http://x", nil)
	require.NoError(t, applyAuth(req, config.AgentAuth{}))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestApplyAuth_EnvToken(t *testing.T) {
	t.Setenv("RUPTOR_TEST_TOKEN", "from-env")
	req, _ := http.NewRequest("POST", "http://x", nil)
	err := applyAuth(req, config.AgentAuth{Type: config.AuthBearer, Token: "${RUPTOR_TEST_TOKEN}"})
	require.NoError(t, err)
	assert.Equal(t, "Bearer from-env", req.Header.Get("Authorization"))
}

// TestCallAgentHTTP_ContractSurface verifies ADR-009 headers, body shape,
// and response parsing end-to-end against an in-process server.
func TestCallAgentHTTP_ContractSurface(t *testing.T) {
	var gotHeaders http.Header
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hello back"}}]}`)
	}))
	defer srv.Close()

	stub := &stubLLMClient{responses: []string{"user says hi"}}
	s := NewSimulator(stub, http.DefaultClient, ui.SilentLogger(), config.SimAgentConfig{
		BaseURL:        srv.URL,
		Endpoint:       "/chat",
		ResponseFormat: config.ResponseFormatAuto,
		Auth: config.AgentAuth{
			Type:  config.AuthBearer,
			Token: "tok-123",
		},
		RequestTimeoutS: 5,
	})

	_, err := s.Run(context.Background(), config.Simulation{
		ID:              "contract-test",
		Persona:         "tester",
		Goal:            "verify headers",
		MaxTurns:        1,
		SuccessCriteria: "hello back",
	})
	require.NoError(t, err)

	assert.NotEmpty(t, gotHeaders.Get("X-Ruptor-Session-Id"))
	assert.NotEmpty(t, gotHeaders.Get("X-Ruptor-Run-Id"))
	assert.Equal(t, "1", gotHeaders.Get("X-Ruptor-Turn"))
	assert.Equal(t, "false", gotHeaders.Get("X-Ruptor-Fault-Active"))
	assert.Equal(t, "Bearer tok-123", gotHeaders.Get("Authorization"))

	assert.Equal(t, "ruptor-simulate", gotBody["model"])
	meta, ok := gotBody["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "tester", meta["persona"])
	assert.Equal(t, "verify headers", meta["goal"])
	assert.EqualValues(t, 1, meta["turn"])
}
