package simulate

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/pkg/types"
)

// LLMClient abstracts the language model used for generating simulated user
// messages and optional agent responses when no HTTP agent is configured.
type LLMClient interface {
	Complete(ctx context.Context, messages []map[string]string, model string) (string, error)
}

// Simulator orchestrates simulated conversations between a persona-driven user
// and an AI agent under test per ADR-009.
type Simulator struct {
	llmClient   LLMClient
	httpClient  *http.Client
	logger      zerolog.Logger
	agent       config.SimAgentConfig
	runID       string
	goalChecker GoalChecker
}

// NewSimulator creates a Simulator with the provided dependencies. The agent
// configuration is captured by value so that a single Simulator instance
// serves one configured run. Goal detection defaults to an LLM-backed
// checker that reuses llmClient; override with WithGoalChecker.
func NewSimulator(llmClient LLMClient, httpClient *http.Client, logger zerolog.Logger, agent config.SimAgentConfig) *Simulator {
	return &Simulator{
		llmClient:   llmClient,
		httpClient:  httpClient,
		logger:      logger,
		agent:       agent,
		runID:       newUUID(),
		goalChecker: &LLMGoalChecker{Client: llmClient},
	}
}

// WithGoalChecker overrides the default LLMGoalChecker. Intended for
// tests that want deterministic goal-reached decisions without calling
// through the LLM client.
func (s *Simulator) WithGoalChecker(c GoalChecker) *Simulator {
	s.goalChecker = c
	return s
}

// NewSimulatorFromBaseURL is a compatibility wrapper matching the original
// (baseURL, timeoutS) signature. Used by tests that exercise the LLM fallback
// path without building a full SimAgentConfig.
func NewSimulatorFromBaseURL(llmClient LLMClient, httpClient *http.Client, logger zerolog.Logger, baseURL string, timeoutS int) *Simulator {
	return NewSimulator(llmClient, httpClient, logger, config.SimAgentConfig{
		BaseURL:         baseURL,
		RequestTimeoutS: timeoutS,
	})
}

// Run executes a single simulation scenario, returning the result with timing
// and goal-reached status.
func (s *Simulator) Run(ctx context.Context, sim config.Simulation) (*types.SimulationResult, error) {
	start := time.Now()
	sessionID := newUUID()

	builder := &PersonaPromptBuilder{}
	systemPrompt := builder.Build(sim.Persona, sim.Goal, sim.SuccessCriteria)

	history := &types.ConversationHistory{}
	history.Add("system", systemPrompt)

	var goalReached bool
	var turnCount int

	for turn := 0; turn < sim.MaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("simulate: running %s: %w", sim.ID, err)
		}

		userMsg, err := s.generateUserMessage(ctx, history)
		if err != nil {
			return nil, fmt.Errorf("simulate: running %s: generating user message: %w", sim.ID, err)
		}
		history.Add("user", userMsg)

		s.logger.Debug().
			Str("simulation_id", sim.ID).
			Str("session_id", sessionID).
			Int("turn", turn+1).
			Msg("simulated user message")

		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("simulate: running %s: %w", sim.ID, err)
		}

		agentResp, err := s.generateAgentResponse(ctx, history, sim, sessionID, turn+1)
		if err != nil {
			return nil, fmt.Errorf("simulate: running %s: generating agent response: %w", sim.ID, err)
		}
		history.Add("assistant", agentResp)

		s.logger.Debug().
			Str("simulation_id", sim.ID).
			Str("session_id", sessionID).
			Int("turn", turn+1).
			Msg("agent response")

		turnCount = turn + 1

		reached, err := s.goalChecker.Check(ctx, agentResp, sim.Goal, sim.SuccessCriteria)
		if err != nil {
			s.logger.Warn().
				Str("simulation_id", sim.ID).
				Err(err).
				Msg("simulate: goal check failed")
		}
		if reached {
			goalReached = true
			break
		}
	}

	duration := time.Since(start)

	return &types.SimulationResult{
		SimulationID: sim.ID,
		Persona:      sim.Persona,
		Goal:         sim.Goal,
		GoalReached:  goalReached,
		TurnCount:    turnCount,
		MaxTurns:     sim.MaxTurns,
		DurationMs:   duration.Milliseconds(),
		History:      history,
	}, nil
}

// generateUserMessage calls the LLM with the conversation history to produce
// the next simulated user message.
func (s *Simulator) generateUserMessage(ctx context.Context, history *types.ConversationHistory) (string, error) {
	resp, err := s.llmClient.Complete(ctx, history.ToOpenAIMessages(), "")
	if err != nil {
		return "", fmt.Errorf("simulate: llm complete: %w", err)
	}
	return resp, nil
}

// generateAgentResponse dispatches the agent call per ADR-009. When no
// BaseURL is configured the LLM client is used as a local fallback, which
// keeps unit tests free of an HTTP dependency.
func (s *Simulator) generateAgentResponse(ctx context.Context, history *types.ConversationHistory, sim config.Simulation, sessionID string, turn int) (string, error) {
	msgs := history.ToOpenAIMessages()
	agentMsgs := make([]map[string]string, 0, len(msgs))
	for _, m := range msgs {
		if m["role"] == "system" {
			continue
		}
		agentMsgs = append(agentMsgs, m)
	}

	if s.agent.BaseURL == "" {
		resp, err := s.llmClient.Complete(ctx, agentMsgs, "")
		if err != nil {
			return "", fmt.Errorf("simulate: llm complete: %w", err)
		}
		return resp, nil
	}

	return s.callAgentHTTP(ctx, agentMsgs, sim, sessionID, turn)
}

// callAgentHTTP performs the ADR-009 POST to the agent, assembling headers,
// auth, body, and response parsing according to the configured format.
func (s *Simulator) callAgentHTTP(ctx context.Context, agentMsgs []map[string]string, sim config.Simulation, sessionID string, turn int) (string, error) {
	reqURL := strings.TrimRight(s.agent.BaseURL, "/") + s.agent.Endpoint

	body := map[string]any{
		"model":    "ruptor-simulate",
		"messages": agentMsgs,
		"stream":   s.agent.Streaming,
		"metadata": map[string]any{
			"session_id": sessionID,
			"run_id":     s.runID,
			"turn":       turn,
			"persona":    sim.Persona,
			"goal":       sim.Goal,
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("simulate: marshaling request: %w", err)
	}

	timeout := s.agent.RequestTimeoutS
	if timeout <= 0 {
		timeout = 30
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("simulate: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ruptor-Session-Id", sessionID)
	req.Header.Set("X-Ruptor-Run-Id", s.runID)
	req.Header.Set("X-Ruptor-Turn", strconv.Itoa(turn))
	req.Header.Set("X-Ruptor-Fault-Active", "false")
	if err := applyAuth(req, s.agent.Auth); err != nil {
		return "", err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("simulate: calling agent: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("simulate: reading agent response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("simulate: agent returned HTTP %d: %s", resp.StatusCode, truncate(string(raw), 512))
	}

	return parseAgentResponse(raw, s.agent.ResponseFormat, s.agent.ResponseField)
}

// applyAuth sets the Authorization / custom header per ADR-009. Token values
// of the form "${ENV_VAR}" are resolved from the environment so secrets do
// not live in YAML.
func applyAuth(req *http.Request, auth config.AgentAuth) error {
	token := resolveEnvToken(auth.Token)
	switch auth.Type {
	case "", config.AuthNone:
		return nil
	case config.AuthBearer:
		if token == "" {
			return fmt.Errorf("simulate: auth.token is required for bearer auth")
		}
		req.Header.Set("Authorization", "Bearer "+token)
	case config.AuthBasic:
		if auth.Username == "" || token == "" {
			return fmt.Errorf("simulate: auth.username and auth.token are required for basic auth")
		}
		cred := base64.StdEncoding.EncodeToString([]byte(auth.Username + ":" + token))
		req.Header.Set("Authorization", "Basic "+cred)
	case config.AuthHeader:
		if auth.HeaderName == "" || token == "" {
			return fmt.Errorf("simulate: auth.header_name and auth.token are required for header auth")
		}
		req.Header.Set(auth.HeaderName, token)
	default:
		return fmt.Errorf("simulate: unsupported auth type %q", auth.Type)
	}
	return nil
}

func resolveEnvToken(token string) string {
	if strings.HasPrefix(token, "${") && strings.HasSuffix(token, "}") {
		return os.Getenv(token[2 : len(token)-1])
	}
	return token
}

// parseAgentResponse resolves the agent's textual reply from a JSON body,
// following the ADR-009 priority order when format == "" or "auto".
func parseAgentResponse(raw []byte, format, customField string) (string, error) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", fmt.Errorf("simulate: non-JSON agent response: %s", truncate(string(raw), 256))
	}

	switch format {
	case config.ResponseFormatOpenAI:
		if s, ok := openAIContent(data); ok {
			return s, nil
		}
		return "", agentResponseErr("openai", raw)
	case config.ResponseFormatAnthropic:
		if s, ok := anthropicContent(data); ok {
			return s, nil
		}
		return "", agentResponseErr("anthropic", raw)
	case config.ResponseFormatCustom:
		if customField == "" {
			return "", fmt.Errorf("simulate: response_format=custom requires response_field")
		}
		if s, ok := data[customField].(string); ok {
			return s, nil
		}
		return "", fmt.Errorf("simulate: custom field %q not found or not a string in response: %s", customField, truncate(string(raw), 256))
	}

	// auto / unset — ADR-009 priority order.
	if s, ok := openAIContent(data); ok {
		return s, nil
	}
	if s, ok := anthropicContent(data); ok {
		return s, nil
	}
	for _, field := range []string{"response", "message", "content"} {
		if s, ok := data[field].(string); ok {
			return s, nil
		}
	}
	return "", agentResponseErr("auto", raw)
}

func openAIContent(data map[string]any) (string, bool) {
	choices, ok := data["choices"].([]any)
	if !ok || len(choices) == 0 {
		return "", false
	}
	first, ok := choices[0].(map[string]any)
	if !ok {
		return "", false
	}
	msg, ok := first["message"].(map[string]any)
	if !ok {
		return "", false
	}
	s, ok := msg["content"].(string)
	return s, ok
}

func anthropicContent(data map[string]any) (string, bool) {
	content, ok := data["content"].([]any)
	if !ok || len(content) == 0 {
		return "", false
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		return "", false
	}
	s, ok := first["text"].(string)
	return s, ok
}

func agentResponseErr(format string, raw []byte) error {
	return fmt.Errorf("simulate: could not extract text from agent response (format=%s): %s", format, truncate(string(raw), 256))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// newUUID returns an RFC 4122 v4 UUID without a third-party dependency.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("ruptor-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
