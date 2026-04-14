package config

import (
	"errors"
	"fmt"

	"github.com/faultforge/faultforge/pkg/types"
)

// ChaosConfig represents the top-level configuration for chaos testing mode.
type ChaosConfig struct {
	Agent      AgentConfig      `yaml:"agent"`
	Proxy      ProxyConfig      `yaml:"proxy"`
	Tests      []TestConfig     `yaml:"tests"`
	Evaluation EvaluationConfig `yaml:"evaluation"`
	Output     OutputConfig     `yaml:"output"`
}

// AgentConfig describes the agent under test in chaos mode.
type AgentConfig struct {
	Name       string            `yaml:"name"`
	Entrypoint string            `yaml:"entrypoint"`
	Env        map[string]string `yaml:"env"`
}

// ProxyConfig describes the fault-injecting proxy.
type ProxyConfig struct {
	Port            int    `yaml:"port"`
	PassthroughURL  string `yaml:"passthrough_url"`
	RequestTimeoutS int    `yaml:"request_timeout_s"`
}

// TestConfig describes a single fault injection test case.
type TestConfig struct {
	ID          string          `yaml:"id"`
	Tool        string          `yaml:"tool"`
	Fault       types.FaultType `yaml:"fault"`
	DelayMS     int             `yaml:"delay_ms"`
	Probability float64         `yaml:"probability"`
	Payload     string          `yaml:"payload"`
	StatusCode  int             `yaml:"status_code"`
	Body        string          `yaml:"body"`
	RetryAfterS int             `yaml:"retry_after_s"`
}

// EvaluationConfig controls how test results are evaluated.
type EvaluationConfig struct {
	MaxIterations  int    `yaml:"max_iterations"`
	TimeoutS       int    `yaml:"timeout_s"`
	LLMJudge       bool   `yaml:"llm_judge"`
	LLMJudgePrompt string `yaml:"llm_judge_prompt"`
}

// OutputConfig controls report generation.
type OutputConfig struct {
	Format string `yaml:"format"`
	Path   string `yaml:"path"`
}

// Validate checks all required fields and returns all validation errors joined together.
func (c *ChaosConfig) Validate() error {
	var errs []error

	if c.Agent.Name == "" {
		errs = append(errs, errors.New("agent.name is required"))
	}

	if c.Proxy.Port <= 0 {
		errs = append(errs, errors.New("proxy.port must be greater than 0"))
	}

	if c.Proxy.PassthroughURL == "" {
		errs = append(errs, errors.New("proxy.passthrough_url is required"))
	}

	for i, t := range c.Tests {
		if t.ID == "" {
			errs = append(errs, fmt.Errorf("tests[%d].id is required", i))
		}
		if t.Tool == "" {
			errs = append(errs, fmt.Errorf("tests[%d].tool is required", i))
		}
		if t.Fault == "" {
			errs = append(errs, fmt.Errorf("tests[%d].fault is required", i))
		}
		if t.Probability < 0 || t.Probability > 1 {
			errs = append(errs, fmt.Errorf("tests[%d].probability must be between 0 and 1", i))
		}
	}

	if err := validateOutputFormat(c.Output.Format); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func validateOutputFormat(format string) error {
	switch format {
	case "stdout", "json", "html", "both", "":
		return nil
	default:
		return fmt.Errorf("output.format must be one of: stdout, json, html, both (got %q)", format)
	}
}
