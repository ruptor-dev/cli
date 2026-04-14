package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConversationHistory_Add(t *testing.T) {
	tests := []struct {
		name    string
		turns   []struct{ role, content string }
		wantLen int
	}{
		{
			name:    "single turn",
			turns:   []struct{ role, content string }{{role: "user", content: "hello"}},
			wantLen: 1,
		},
		{
			name: "multiple turns",
			turns: []struct{ role, content string }{
				{role: "user", content: "hello"},
				{role: "assistant", content: "hi there"},
				{role: "user", content: "how are you?"},
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &ConversationHistory{}
			for _, turn := range tt.turns {
				h.Add(turn.role, turn.content)
			}

			require.Len(t, h.Turns, tt.wantLen)
			for i, turn := range tt.turns {
				assert.Equal(t, turn.role, h.Turns[i].Role)
				assert.Equal(t, turn.content, h.Turns[i].Content)
				assert.False(t, h.Turns[i].Timestamp.IsZero(), "timestamp should be set")
			}
		})
	}
}

func TestConversationHistory_ToOpenAIMessages(t *testing.T) {
	tests := []struct {
		name   string
		turns  []struct{ role, content string }
		expect []map[string]string
	}{
		{
			name:   "empty history",
			turns:  nil,
			expect: []map[string]string{},
		},
		{
			name: "single turn",
			turns: []struct{ role, content string }{
				{role: "user", content: "hello"},
			},
			expect: []map[string]string{
				{"role": "user", "content": "hello"},
			},
		},
		{
			name: "multi-turn conversation",
			turns: []struct{ role, content string }{
				{role: "system", content: "you are helpful"},
				{role: "user", content: "hello"},
				{role: "assistant", content: "hi there"},
			},
			expect: []map[string]string{
				{"role": "system", "content": "you are helpful"},
				{"role": "user", "content": "hello"},
				{"role": "assistant", "content": "hi there"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &ConversationHistory{}
			for _, turn := range tt.turns {
				h.Add(turn.role, turn.content)
			}

			msgs := h.ToOpenAIMessages()
			require.Len(t, msgs, len(tt.expect))
			for i, expected := range tt.expect {
				assert.Equal(t, expected["role"], msgs[i]["role"])
				assert.Equal(t, expected["content"], msgs[i]["content"])
			}
		})
	}
}

func TestConversationHistory_EmptyHistory(t *testing.T) {
	h := &ConversationHistory{}

	assert.Empty(t, h.Turns)
	msgs := h.ToOpenAIMessages()
	assert.NotNil(t, msgs, "ToOpenAIMessages should return non-nil empty slice")
	assert.Empty(t, msgs)
}
