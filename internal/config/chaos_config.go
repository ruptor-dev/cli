package config

import (
	"errors"
	"fmt"

	"github.com/ruptor-dev/cli/pkg/types"
)

// SchemaVersion is the current chaos/simulate config schema version. Configs
// must declare this value explicitly so that a future breaking change (v2)
// can reject stale documents without guessing.
const SchemaVersion = "1"

// ChaosConfig represents the top-level configuration for chaos testing mode.
type ChaosConfig struct {
	Version    string           `yaml:"version"`
	Agent      AgentConfig      `yaml:"agent"`
	Proxy      ProxyConfig      `yaml:"proxy"`
	Tests      []TestConfig     `yaml:"tests"`
	Evaluation EvaluationConfig `yaml:"evaluation"`
	Output     OutputConfig     `yaml:"output"`
}

// AgentConfig describes the agent under test in chaos mode.
// Mode selects the process lifecycle:
//   - "oneshot" (default): ruptor launches the entrypoint once per
//     experiment and waits for it to exit or for timeout_s.
//   - "persistent": ruptor launches the entrypoint once before the
//     first experiment and stops it after the last. Long-running HTTP
//     servers (Flask, Bubbletea-backed shells) use this mode.
//
// An empty Entrypoint turns the runner off entirely and the user is
// expected to manage the agent process externally.
type AgentConfig struct {
	Name       string            `yaml:"name"`
	Entrypoint string            `yaml:"entrypoint"`
	Mode       string            `yaml:"mode"`
	Env        map[string]string `yaml:"env"`
}

// ProxyMode is the canonical, validated proxy protocol mode. Callers
// MUST compare against the exported ProxyMode* constants — the type
// exists precisely so the compiler catches stringly-typed drift.
//
// YAML/mapstructure decoding still works because ProxyMode's underlying
// type is string; Validate() is the single gate that converts user
// input into one of the allowed canonical values (or rejects it).
type ProxyMode string

const (
	// ProxyModeHTTP routes all traffic through the HTTP fault handler.
	// This is the default when proxy.mode is omitted from the config.
	ProxyModeHTTP ProxyMode = "http"
	// ProxyModeMCP treats all traffic as MCP JSON-RPC 2.0.
	ProxyModeMCP ProxyMode = "mcp"
	// ProxyModeAuto inspects each request and routes to either the MCP
	// or HTTP handler at runtime based on payload shape.
	ProxyModeAuto ProxyMode = "auto"
)

// ProxyConfig describes the fault-injecting proxy.
type ProxyConfig struct {
	Port            int    `yaml:"port"`
	PassthroughURL  string `yaml:"passthrough_url"`
	RequestTimeoutS int    `yaml:"request_timeout_s"`
	// Mode selects the proxy protocol. Allowed values are the
	// ProxyMode* constants, or the empty string, which Validate()
	// rewrites to ProxyModeHTTP. Validation is case-exact and
	// whitespace-exact: "MCP" or " mcp " are rejected.
	Mode ProxyMode `yaml:"mode"`
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

// Validate checks all required fields and returns all validation
// errors joined together. Validate also canonicalises c.Proxy.Mode:
// an empty value is rewritten in place to ProxyModeHTTP so downstream
// consumers see exactly one representation of "default". This
// mutation is the only non-check side effect; a second Validate
// call on a canonicalised config is idempotent.
func (c *ChaosConfig) Validate() error {
	var errs []error

	if c.Version == "" {
		errs = append(errs, fmt.Errorf("version is required (expected %q)", SchemaVersion))
	} else if c.Version != SchemaVersion {
		errs = append(errs, fmt.Errorf("version %q is not supported (expected %q)", c.Version, SchemaVersion))
	}

	if c.Agent.Name == "" {
		errs = append(errs, errors.New("agent.name is required"))
	}

	if c.Proxy.Port <= 0 {
		errs = append(errs, errors.New("proxy.port must be greater than 0"))
	}

	if c.Proxy.PassthroughURL == "" {
		errs = append(errs, errors.New("proxy.passthrough_url is required"))
	}

	// Proxy mode is case-exact and whitespace-exact. The empty string is
	// rewritten to ProxyModeHTTP so downstream consumers never see a
	// degenerate "" value — there is exactly one canonical representation
	// of "default HTTP" post-Validate.
	switch c.Proxy.Mode {
	case "":
		c.Proxy.Mode = ProxyModeHTTP
	case ProxyModeHTTP, ProxyModeMCP, ProxyModeAuto:
		// canonical — no rewrite needed
	default:
		errs = append(errs, fmt.Errorf(`proxy.mode must be one of "http", "mcp", "auto" (got %q)`, string(c.Proxy.Mode)))
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
