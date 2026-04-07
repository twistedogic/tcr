package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type AgentSection struct {
	Agent string   `yaml:"agent,omitempty"`
	Args  []string `yaml:"args,omitempty"`
}

type AgentConfig struct {
	Interactive    AgentSection `yaml:"interactive"`
	NonInteractive AgentSection `yaml:"non_interactive"`
}

var defaultConfig = AgentConfig{
	Interactive: AgentSection{
		Agent: "crush",
		Args:  []string{"--yolo"},
	},
	NonInteractive: AgentSection{
		Agent: "pi",
		Args:  []string{"--model", "claude-sonnet", "--print"},
	},
}

var cfg = defaultConfig

func loadConfig() error {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not determine home directory: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	configPath := filepath.Join(configHome, ".tcr", "config.yaml")

	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		cfg = defaultConfig
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not read config file %s: %w", configPath, err)
	}

	merged := defaultConfig
	if err := yaml.Unmarshal(data, &merged); err != nil {
		return fmt.Errorf("could not parse config file %s: %w", configPath, err)
	}
	cfg = merged
	return nil
}
