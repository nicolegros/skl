package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config represents the skills tool configuration.
type Config struct {
	Directories []string `json:"directories"`
}

// Dir returns the directory where the skills config and lock files live.
func Dir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "skl")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "skl")
}

// Load reads the config from disk, creating it with defaults if missing.
func Load() (*Config, error) {
	dir := Dir()
	path := filepath.Join(dir, "config.json")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := &Config{Directories: []string{"~/.skills"}}
		if err := save(cfg, path); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func save(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
