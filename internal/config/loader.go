package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// LoadChaos reads a YAML file at path, unmarshals it into a ChaosConfig,
// applies RUPTOR_* env-var overrides on the fields that support them,
// validates the result, and returns it.
//
// YAML parsing uses gopkg.in/yaml.v3 directly rather than viper because
// chaos.yaml is a user-authored test plan whose map keys (e.g.
// agent.env.TOOL_BASE_URL) must preserve case. Viper lowercases every
// key during its internal normalisation pass, which breaks env-var maps.
//
// Env overrides are plumbed through ApplySettings using viper so the
// full precedence chain (CLI flag > env > .ruptor.yaml > ~/.ruptor/config.yaml
// > default) still applies to ruptor's own settings.
func LoadChaos(path string) (*ChaosConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: loading %s: %w", path, err)
	}

	var cfg ChaosConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: loading %s: %w", path, err)
	}

	applyChaosEnvOverrides(&cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: validating %s: %w", path, err)
	}

	return &cfg, nil
}

// LoadSimulate reads a YAML file at path, unmarshals it into a
// SimulateConfig, validates it, and returns the result.
func LoadSimulate(path string) (*SimulateConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: loading %s: %w", path, err)
	}

	var cfg SimulateConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: loading %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: validating %s: %w", path, err)
	}

	return &cfg, nil
}

// applyChaosEnvOverrides honours the subset of RUPTOR_* env vars that
// map onto ChaosConfig fields. Per CLAUDE.md's config hierarchy, an env
// value takes precedence over the YAML document.
func applyChaosEnvOverrides(cfg *ChaosConfig) {
	if v := os.Getenv("RUPTOR_PROXY_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Proxy.Port = port
		}
	}
	if v := os.Getenv("RUPTOR_OUTPUT_PATH"); v != "" {
		cfg.Output.Path = v
	}
}
