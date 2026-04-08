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
	configPath := filepath.Join(configHome, "tcr", "config.yaml")

	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		cfg = defaultConfig
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			return fmt.Errorf("could not create config directory: %w", err)
		}
		out, err := yaml.Marshal(defaultConfig)
		if err != nil {
			return fmt.Errorf("could not marshal default config: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0644); err != nil {
			return fmt.Errorf("could not write default config to %s: %w", configPath, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not read config file %s: %w", configPath, err)
	}

	var userConfig AgentConfig
	if err := yaml.Unmarshal(data, &userConfig); err != nil {
		return fmt.Errorf("could not parse config file %s: %w", configPath, err)
	}
	cfg = userConfig
	return nil
}
