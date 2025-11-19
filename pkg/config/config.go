package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the configuration file structure
type Config struct {
	Output   string         `yaml:"output"`
	Packages []PackageEntry `yaml:"packages"`
}

// PackageEntry represents a package and its types to extract
type PackageEntry struct {
	Package string   `yaml:"package"`
	Types   []string `yaml:"types"`
}

// LoadConfig loads the configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// Validate checks if the config is valid
func (c *Config) Validate() error {
	if c.Output == "" {
		return fmt.Errorf("output directory is required")
	}

	if len(c.Packages) == 0 {
		return fmt.Errorf("at least one package entry is required")
	}

	for i, pkg := range c.Packages {
		if pkg.Package == "" {
			return fmt.Errorf("package path is required for entry %d", i)
		}
		if len(pkg.Types) == 0 {
			return fmt.Errorf("at least one type is required for package %s", pkg.Package)
		}
	}

	return nil
}
