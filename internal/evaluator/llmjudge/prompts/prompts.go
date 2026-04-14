// Package prompts ships the LLM-judge system prompts as embedded text
// files. architecture.md rule: "Never hardcode prompts in Go source."
// The prompts are versioned alongside code via //go:embed so they are
// immutable at runtime and visible in git history.
package prompts

import _ "embed"

//go:embed chaos_system.txt
var ChaosSystem string

//go:embed conversation_system.txt
var ConversationSystem string
