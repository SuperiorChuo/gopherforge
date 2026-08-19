package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

var (
	Cfg Config
)

// LoadConfig loads the configuration file.
func LoadConfig(filePath string) error {
	// Read the configuration file.
	file, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML.
	if err := yaml.Unmarshal(file, &Cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Replace environment variables.
	replaceEnvVars(&Cfg)

	return nil
}
