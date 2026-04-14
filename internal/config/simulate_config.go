package config

import (
	"errors"
	"fmt"
)

// SimulateConfig represents the top-level configuration for simulation mode.
type SimulateConfig struct {
	Agent       SimAgentConfig    `yaml:"agent"`
	Simulations []Simulation      `yaml:"simulations"`
	Evaluation  SimEvaluationConfig `yaml:"evaluation"`
	Output      OutputConfig      `yaml:"output"`
}

// SimAgentConfig describes the agent under test in simulation mode.
type SimAgentConfig struct {
	Name            string `yaml:"name"`
	Entrypoint      string `yaml:"entrypoint"`
	BaseURL         string `yaml:"base_url"`
	RequestTimeoutS int    `yaml:"request_timeout_s"`
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

	if c.Agent.Name == "" {
		errs = append(errs, errors.New("agent.name is required"))
	}

	if c.Agent.BaseURL == "" {
		errs = append(errs, errors.New("agent.base_url is required"))
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
