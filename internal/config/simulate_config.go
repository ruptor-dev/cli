package config

import (
	"errors"
	"fmt"
)

// SimulateConfig represents the top-level configuration for simulation mode.
type SimulateConfig struct {
	Version     string              `yaml:"version"`
	Agent       SimAgentConfig      `yaml:"agent"`
	Simulations []Simulation        `yaml:"simulations"`
	Evaluation  SimEvaluationConfig `yaml:"evaluation"`
	Output      OutputConfig        `yaml:"output"`
}

// Protocol values accepted by SimAgentConfig.Protocol.
const (
	ProtocolHTTP = "http"
	ProtocolMCP  = "mcp" // v2 scope — accepted in config but not wired yet
)

// ResponseFormat values accepted by SimAgentConfig.ResponseFormat.
const (
	ResponseFormatAuto      = "auto"
	ResponseFormatOpenAI    = "openai"
	ResponseFormatAnthropic = "anthropic"
	ResponseFormatCustom    = "custom"
)

// Auth types accepted by AgentAuth.Type.
const (
	AuthNone   = "none"
	AuthBearer = "bearer"
	AuthBasic  = "basic"
	AuthHeader = "header"
)

// SimAgentConfig describes the agent under test in simulation mode. Mirrors
// ADR-009 — the full HTTP contract for `ruptor simulate`.
type SimAgentConfig struct {
	Name            string     `yaml:"name"`
	Entrypoint      string     `yaml:"entrypoint"`
	BaseURL         string     `yaml:"base_url"`
	Endpoint        string     `yaml:"endpoint"`
	Protocol        string     `yaml:"protocol"`
	ResponseFormat  string     `yaml:"response_format"`
	ResponseField   string     `yaml:"response_field"`
	Streaming       bool       `yaml:"streaming"`
	Auth            AgentAuth  `yaml:"auth"`
	RequestTimeoutS int        `yaml:"request_timeout_s"`
}

// AgentAuth describes how the simulator authenticates to the agent under
// test per ADR-009. Token may be a ${ENV_VAR} reference resolved at runtime.
type AgentAuth struct {
	Type       string `yaml:"type"`
	Token      string `yaml:"token"`
	Username   string `yaml:"username"`
	HeaderName string `yaml:"header_name"`
}

// Simulation describes a single simulated user interaction scenario.
type Simulation struct {
	ID              string `yaml:"id"`
	Persona         string `yaml:"persona"`
	Goal            string `yaml:"goal"`
	MaxTurns        int    `yaml:"max_turns"`
	SuccessCriteria string `yaml:"success_criteria"`
}

// SimEvaluationConfig controls how simulation results are evaluated.
type SimEvaluationConfig struct {
	GoalCompletion bool   `yaml:"goal_completion"`
	TurnEfficiency bool   `yaml:"turn_efficiency"`
	ToneQuality    bool   `yaml:"tone_quality"`
	LLMJudgePrompt string `yaml:"llm_judge_prompt"`
}

// Validate checks all required fields and returns all validation errors joined together.
func (c *SimulateConfig) Validate() error {
	var errs []error

	if c.Version == "" {
		errs = append(errs, fmt.Errorf("version is required (expected %q)", SchemaVersion))
	} else if c.Version != SchemaVersion {
		errs = append(errs, fmt.Errorf("version %q is not supported (expected %q)", c.Version, SchemaVersion))
	}

	if c.Agent.Name == "" {
		errs = append(errs, errors.New("agent.name is required"))
	}

	if c.Agent.BaseURL == "" {
		errs = append(errs, errors.New("agent.base_url is required"))
	}

	if err := validateProtocol(c.Agent.Protocol); err != nil {
		errs = append(errs, err)
	}

	if err := validateResponseFormat(c.Agent.ResponseFormat); err != nil {
		errs = append(errs, err)
	}

	if err := validateAuthType(c.Agent.Auth.Type); err != nil {
		errs = append(errs, err)
	}

	for i, s := range c.Simulations {
		if s.ID == "" {
			errs = append(errs, fmt.Errorf("simulations[%d].id is required", i))
		}
		if s.Persona == "" {
			errs = append(errs, fmt.Errorf("simulations[%d].persona is required", i))
		}
		if s.Goal == "" {
			errs = append(errs, fmt.Errorf("simulations[%d].goal is required", i))
		}
		if s.MaxTurns <= 0 {
			errs = append(errs, fmt.Errorf("simulations[%d].max_turns must be greater than 0", i))
		}
	}

	if err := validateOutputFormat(c.Output.Format); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func validateProtocol(p string) error {
	switch p {
	case "", ProtocolHTTP, ProtocolMCP:
		return nil
	default:
		return fmt.Errorf("agent.protocol must be one of: http, mcp (got %q)", p)
	}
}

func validateResponseFormat(f string) error {
	switch f {
	case "", ResponseFormatAuto, ResponseFormatOpenAI, ResponseFormatAnthropic, ResponseFormatCustom:
		return nil
	default:
		return fmt.Errorf("agent.response_format must be one of: auto, openai, anthropic, custom (got %q)", f)
	}
}

func validateAuthType(t string) error {
	switch t {
	case "", AuthNone, AuthBearer, AuthBasic, AuthHeader:
		return nil
	default:
		return fmt.Errorf("agent.auth.type must be one of: none, bearer, basic, header (got %q)", t)
	}
}
