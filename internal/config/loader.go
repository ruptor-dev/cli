package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadChaos reads a YAML file at path, unmarshals it into a ChaosConfig,
// validates it, and returns the result.
func LoadChaos(path string) (*ChaosConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: loading %s: %w", path, err)
	}

	var cfg ChaosConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: loading %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: validating %s: %w", path, err)
	}

	return &cfg, nil
}

// LoadSimulate reads a YAML file at path, unmarshals it into a SimulateConfig,
// validates it, and returns the result.
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
