package simulate

import "fmt"

// PersonaPromptBuilder constructs system prompts for the simulated user persona.
type PersonaPromptBuilder struct{}

// Build returns a system prompt instructing the LLM to act as the given persona,
// pursuing the specified goal with the provided success criteria.
func (b *PersonaPromptBuilder) Build(persona, goal, successCriteria string) string {
	return fmt.Sprintf(`You are simulating a user with the following persona: %s
Your goal is: %s
Success criteria: %s

Stay in character. Be natural and realistic. When your goal is achieved, respond with something that naturally ends the conversation.
Do not break character or mention that you are a simulation.`, persona, goal, successCriteria)
}
