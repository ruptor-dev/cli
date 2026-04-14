package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validChaosYAML = `
version: "1"
agent:
  name: support_agent
  entrypoint: python agent.py
  env:
    TOOL_BASE_URL: http://localhost:8080

proxy:
  port: 8080
  passthrough_url: https://real-tool-api.com
  request_timeout_s: 30

tests:
  - id: timeout_on_search
    tool: /search
    fault: tool_timeout
    delay_ms: 30000
    probability: 1.0

  - id: bad_json_on_lookup
    tool: /lookup
    fault: invalid_json
    payload: "{ broken json %%% "
    probability: 0.5

evaluation:
  max_iterations: 20
  timeout_s: 60
  llm_judge: true
  llm_judge_prompt: "Did the agent handle failure?"

output:
  format: both
  path: ./reports/
`

const validSimulateYAML = `
version: "1"
agent:
  name: support_agent
  entrypoint: python agent.py
  base_url: http://localhost:3000
  request_timeout_s: 30

simulations:
  - id: usuario_impaciente
    persona: "Frustrated user"
    goal: "Cancel subscription"
    max_turns: 10
    success_criteria: "Agent completed cancellation"

  - id: usuario_confuso
    persona: "Confused user"
    goal: "Subscribe to premium"
    max_turns: 15
    success_criteria: "Agent guided user"

evaluation:
  goal_completion: true
  turn_efficiency: true
  tone_quality: true
  llm_judge_prompt: "Rate quality 1-10."

output:
  format: json
  path: ./reports/
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func TestLoadChaos(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "valid chaos config",
			yaml: validChaosYAML,
		},
		{
			name: "missing version",
			yaml: `
agent:
  name: test
proxy:
  port: 8080
  passthrough_url: https://example.com
tests: []
output:
  format: json
`,
			wantErr: "version is required",
		},
		{
			name: "unsupported version",
			yaml: `
version: "2"
agent:
  name: test
proxy:
  port: 8080
  passthrough_url: https://example.com
tests: []
output:
  format: json
`,
			wantErr: `version "2" is not supported`,
		},
		{
			name:    "bad yaml syntax",
			yaml:    ":\n  :\n\t- broken",
			wantErr: "config: loading",
		},
		{
			name: "missing agent name",
			yaml: `
agent:
  entrypoint: python agent.py
proxy:
  port: 8080
  passthrough_url: https://example.com
tests: []
output:
  format: json
`,
			wantErr: "agent.name is required",
		},
		{
			name: "missing proxy port",
			yaml: `
agent:
  name: test
proxy:
  passthrough_url: https://example.com
tests: []
output:
  format: json
`,
			wantErr: "proxy.port must be greater than 0",
		},
		{
			name: "missing passthrough url",
			yaml: `
agent:
  name: test
proxy:
  port: 8080
tests: []
output:
  format: json
`,
			wantErr: "proxy.passthrough_url is required",
		},
		{
			name: "test missing id",
			yaml: `
agent:
  name: test
proxy:
  port: 8080
  passthrough_url: https://example.com
tests:
  - tool: /search
    fault: tool_timeout
    probability: 1.0
output:
  format: json
`,
			wantErr: "tests[0].id is required",
		},
		{
			name: "test missing tool",
			yaml: `
agent:
  name: test
proxy:
  port: 8080
  passthrough_url: https://example.com
tests:
  - id: test1
    fault: tool_timeout
    probability: 1.0
output:
  format: json
`,
			wantErr: "tests[0].tool is required",
		},
		{
			name: "test missing fault",
			yaml: `
agent:
  name: test
proxy:
  port: 8080
  passthrough_url: https://example.com
tests:
  - id: test1
    tool: /search
    probability: 1.0
output:
  format: json
`,
			wantErr: "tests[0].fault is required",
		},
		{
			name: "invalid probability greater than 1",
			yaml: `
agent:
  name: test
proxy:
  port: 8080
  passthrough_url: https://example.com
tests:
  - id: test1
    tool: /search
    fault: tool_timeout
    probability: 1.5
output:
  format: json
`,
			wantErr: "tests[0].probability must be between 0 and 1",
		},
		{
			name: "invalid probability negative",
			yaml: `
agent:
  name: test
proxy:
  port: 8080
  passthrough_url: https://example.com
tests:
  - id: test1
    tool: /search
    fault: tool_timeout
    probability: -0.1
output:
  format: json
`,
			wantErr: "tests[0].probability must be between 0 and 1",
		},
		{
			name: "invalid output format",
			yaml: `
agent:
  name: test
proxy:
  port: 8080
  passthrough_url: https://example.com
tests: []
output:
  format: xml
`,
			wantErr: "output.format must be one of",
		},
		{
			name: "multiple validation errors collected",
			yaml: `
agent: {}
proxy: {}
tests:
  - probability: 1.5
output:
  format: xml
`,
			wantErr: "agent.name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTemp(t, tt.yaml)
			cfg, err := LoadChaos(path)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, cfg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)
			}
		})
	}
}

func TestLoadChaos_FileNotFound(t *testing.T) {
	cfg, err := LoadChaos("/nonexistent/path/config.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config: loading")
	assert.Nil(t, cfg)
}

func TestLoadChaos_ValidFieldValues(t *testing.T) {
	path := writeTemp(t, validChaosYAML)
	cfg, err := LoadChaos(path)
	require.NoError(t, err)

	assert.Equal(t, "support_agent", cfg.Agent.Name)
	assert.Equal(t, "python agent.py", cfg.Agent.Entrypoint)
	assert.Equal(t, "http://localhost:8080", cfg.Agent.Env["TOOL_BASE_URL"])

	assert.Equal(t, 8080, cfg.Proxy.Port)
	assert.Equal(t, "https://real-tool-api.com", cfg.Proxy.PassthroughURL)
	assert.Equal(t, 30, cfg.Proxy.RequestTimeoutS)

	require.Len(t, cfg.Tests, 2)
	assert.Equal(t, "timeout_on_search", cfg.Tests[0].ID)
	assert.Equal(t, "/search", cfg.Tests[0].Tool)
	assert.Equal(t, types.FaultToolTimeout, cfg.Tests[0].Fault)
	assert.Equal(t, 30000, cfg.Tests[0].DelayMS)
	assert.Equal(t, 1.0, cfg.Tests[0].Probability)

	assert.Equal(t, "bad_json_on_lookup", cfg.Tests[1].ID)
	assert.Equal(t, types.FaultInvalidJSON, cfg.Tests[1].Fault)
	assert.Equal(t, 0.5, cfg.Tests[1].Probability)

	assert.Equal(t, 20, cfg.Evaluation.MaxIterations)
	assert.Equal(t, 60, cfg.Evaluation.TimeoutS)
	assert.True(t, cfg.Evaluation.LLMJudge)
	assert.Equal(t, "Did the agent handle failure?", cfg.Evaluation.LLMJudgePrompt)

	assert.Equal(t, "both", cfg.Output.Format)
	assert.Equal(t, "./reports/", cfg.Output.Path)
}

func TestLoadSimulate(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "valid simulate config",
			yaml: validSimulateYAML,
		},
		{
			name:    "bad yaml syntax",
			yaml:    ":\n  :\n\t- broken",
			wantErr: "config: loading",
		},
		{
			name: "missing agent name",
			yaml: `
agent:
  base_url: http://localhost:3000
simulations: []
output:
  format: json
`,
			wantErr: "agent.name is required",
		},
		{
			name: "missing base url",
			yaml: `
agent:
  name: test
simulations: []
output:
  format: json
`,
			wantErr: "agent.base_url is required",
		},
		{
			name: "simulation missing id",
			yaml: `
agent:
  name: test
  base_url: http://localhost:3000
simulations:
  - persona: "user"
    goal: "do something"
    max_turns: 5
output:
  format: json
`,
			wantErr: "simulations[0].id is required",
		},
		{
			name: "simulation missing persona",
			yaml: `
agent:
  name: test
  base_url: http://localhost:3000
simulations:
  - id: test1
    goal: "do something"
    max_turns: 5
output:
  format: json
`,
			wantErr: "simulations[0].persona is required",
		},
		{
			name: "simulation missing goal",
			yaml: `
agent:
  name: test
  base_url: http://localhost:3000
simulations:
  - id: test1
    persona: "user"
    max_turns: 5
output:
  format: json
`,
			wantErr: "simulations[0].goal is required",
		},
		{
			name: "simulation max_turns zero",
			yaml: `
agent:
  name: test
  base_url: http://localhost:3000
simulations:
  - id: test1
    persona: "user"
    goal: "do something"
    max_turns: 0
output:
  format: json
`,
			wantErr: "simulations[0].max_turns must be greater than 0",
		},
		{
			name: "invalid output format",
			yaml: `
agent:
  name: test
  base_url: http://localhost:3000
simulations: []
output:
  format: csv
`,
			wantErr: "output.format must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTemp(t, tt.yaml)
			cfg, err := LoadSimulate(path)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, cfg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)
			}
		})
	}
}

func TestLoadSimulate_FileNotFound(t *testing.T) {
	cfg, err := LoadSimulate("/nonexistent/path/config.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config: loading")
	assert.Nil(t, cfg)
}

func TestLoadSimulate_ValidFieldValues(t *testing.T) {
	path := writeTemp(t, validSimulateYAML)
	cfg, err := LoadSimulate(path)
	require.NoError(t, err)

	assert.Equal(t, "support_agent", cfg.Agent.Name)
	assert.Equal(t, "python agent.py", cfg.Agent.Entrypoint)
	assert.Equal(t, "http://localhost:3000", cfg.Agent.BaseURL)
	assert.Equal(t, 30, cfg.Agent.RequestTimeoutS)

	require.Len(t, cfg.Simulations, 2)
	assert.Equal(t, "usuario_impaciente", cfg.Simulations[0].ID)
	assert.Equal(t, "Frustrated user", cfg.Simulations[0].Persona)
	assert.Equal(t, "Cancel subscription", cfg.Simulations[0].Goal)
	assert.Equal(t, 10, cfg.Simulations[0].MaxTurns)
	assert.Equal(t, "Agent completed cancellation", cfg.Simulations[0].SuccessCriteria)

	assert.Equal(t, "usuario_confuso", cfg.Simulations[1].ID)
	assert.Equal(t, 15, cfg.Simulations[1].MaxTurns)

	assert.True(t, cfg.Evaluation.GoalCompletion)
	assert.True(t, cfg.Evaluation.TurnEfficiency)
	assert.True(t, cfg.Evaluation.ToneQuality)
	assert.Contains(t, cfg.Evaluation.LLMJudgePrompt, "Rate quality")

	assert.Equal(t, "json", cfg.Output.Format)
	assert.Equal(t, "./reports/", cfg.Output.Path)
}

func TestChaosValidate_MultipleErrors(t *testing.T) {
	cfg := &ChaosConfig{
		Tests: []TestConfig{
			{Probability: 1.5},
		},
		Output: OutputConfig{Format: "xml"},
	}
	err := cfg.Validate()
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "version is required")
	assert.Contains(t, msg, "agent.name is required")
	assert.Contains(t, msg, "proxy.port must be greater than 0")
	assert.Contains(t, msg, "proxy.passthrough_url is required")
	assert.Contains(t, msg, "tests[0].id is required")
	assert.Contains(t, msg, "tests[0].probability must be between 0 and 1")
	assert.Contains(t, msg, "output.format must be one of")
}

func TestSimulateValidate_MultipleErrors(t *testing.T) {
	cfg := &SimulateConfig{
		Simulations: []Simulation{
			{MaxTurns: -1},
		},
		Output: OutputConfig{Format: "xml"},
	}
	err := cfg.Validate()
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "version is required")
	assert.Contains(t, msg, "agent.name is required")
	assert.Contains(t, msg, "agent.base_url is required")
	assert.Contains(t, msg, "simulations[0].id is required")
	assert.Contains(t, msg, "simulations[0].max_turns must be greater than 0")
	assert.Contains(t, msg, "output.format must be one of")
}
