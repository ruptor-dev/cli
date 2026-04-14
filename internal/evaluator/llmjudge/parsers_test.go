package llmjudge

// Parser tests live in the llmjudge package itself (white-box) so we
// exercise parseChaosResponse / parseConversationResponse directly
// without standing up an OpenAI mock for every case. The parsers are
// the most fault-prone part of the judge — every variation of LLM
// formatting drift bottoms out here.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseChaosResponse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantVerdict string
		wantReason  string
	}{
		{
			name:        "canonical pass",
			input:       "VERDICT: PASS\nREASON: agent recovered cleanly",
			wantVerdict: "PASS",
			wantReason:  "agent recovered cleanly",
		},
		{
			name:        "canonical fail",
			input:       "VERDICT: FAIL\nREASON: crashed without retry",
			wantVerdict: "FAIL",
			wantReason:  "crashed without retry",
		},
		{
			name:        "partial verdict",
			input:       "VERDICT: PARTIAL\nREASON: degraded mode",
			wantVerdict: "PARTIAL",
			wantReason:  "degraded mode",
		},
		{
			name:        "lowercase verdict header",
			input:       "verdict: pass\nreason: ok",
			wantVerdict: "PASS",
			// Reason line was uppercased into "REASON: OK" by the
			// parser's matching pass; the original line value is
			// kept so downstream renderers see the human form.
			wantReason: "ok",
		},
		{
			name:        "extra whitespace",
			input:       "  VERDICT:   PASS  \n  REASON:   ok  ",
			wantVerdict: "PASS",
			wantReason:  "ok",
		},
		{
			name:        "verdict pass-with-suffix is still PASS",
			input:       "VERDICT: PASSED\nREASON: fine",
			wantVerdict: "PASS",
			wantReason:  "fine",
		},
		{
			name:        "unknown verdict falls back to FAIL",
			input:       "VERDICT: MAYBE\nREASON: unsure",
			wantVerdict: "FAIL",
			wantReason:  "unsure",
		},
		{
			name:        "missing verdict header defaults to FAIL with full content as reason",
			input:       "the agent did fine",
			wantVerdict: "FAIL",
			wantReason:  "the agent did fine",
		},
		{
			name:        "missing reason header keeps content as fallback",
			input:       "VERDICT: PASS",
			wantVerdict: "PASS",
			wantReason:  "VERDICT: PASS",
		},
		{
			name:        "extra preamble before headers",
			input:       "Sure! Here is my analysis.\nVERDICT: PASS\nREASON: clean",
			wantVerdict: "PASS",
			wantReason:  "clean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, reason := parseChaosResponse(tt.input)
			assert.Equal(t, tt.wantVerdict, verdict)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
}

func TestParseConversationResponse(t *testing.T) {
	t.Run("plain JSON object", func(t *testing.T) {
		score, issues, err := parseConversationResponse(`{"score": 85, "issues": ["repeated apologies"]}`)
		require.NoError(t, err)
		assert.Equal(t, 85, score)
		assert.Equal(t, []string{"repeated apologies"}, issues)
	})

	t.Run("JSON wrapped in a fenced code block", func(t *testing.T) {
		body := "```json\n{\"score\": 60, \"issues\": []}\n```"
		score, issues, err := parseConversationResponse(body)
		require.NoError(t, err)
		assert.Equal(t, 60, score)
		assert.Empty(t, issues)
	})

	t.Run("JSON with leading prose", func(t *testing.T) {
		body := "Here is the evaluation:\n{\"score\": 92, \"issues\": [\"minor tone\"]}"
		score, issues, err := parseConversationResponse(body)
		require.NoError(t, err)
		assert.Equal(t, 92, score)
		assert.Equal(t, []string{"minor tone"}, issues)
	})

	t.Run("JSON with trailing prose", func(t *testing.T) {
		body := `{"score": 50, "issues": ["off-topic"]} (model commentary follows)`
		score, issues, err := parseConversationResponse(body)
		require.NoError(t, err)
		assert.Equal(t, 50, score)
		assert.Equal(t, []string{"off-topic"}, issues)
	})

	t.Run("empty issues array", func(t *testing.T) {
		score, issues, err := parseConversationResponse(`{"score": 100, "issues": []}`)
		require.NoError(t, err)
		assert.Equal(t, 100, score)
		assert.Empty(t, issues)
	})

	t.Run("missing fields default to zero values", func(t *testing.T) {
		score, issues, err := parseConversationResponse(`{}`)
		require.NoError(t, err)
		assert.Equal(t, 0, score)
		assert.Empty(t, issues)
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		_, _, err := parseConversationResponse(`{"score": 80, "issues": [`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid JSON")
	})

	t.Run("no JSON object at all", func(t *testing.T) {
		_, _, err := parseConversationResponse(`I refuse to answer in JSON.`)
		require.Error(t, err)
	})

	t.Run("extracts the largest object span", func(t *testing.T) {
		// Some models echo a partial example before the real result.
		// The parser uses the first '{' and last '}' which spans the
		// entire payload — verify that behavior holds so a future
		// "use the first matched object" change doesn't silently
		// regress it.
		body := strings.Join([]string{
			"Example shape: {score: number, issues: string[]}",
			`{"score": 77, "issues": ["x"]}`,
		}, "\n")
		_, _, err := parseConversationResponse(body)
		require.Error(t, err, "first { is in the example so the span is invalid JSON — documents current behavior")
	})
}
