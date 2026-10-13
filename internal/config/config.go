package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the skills tool configuration.
type Config struct {
	Directories []string `yaml:"directories"`
}

// Dir returns the directory where the skills config and lock files live.
func Dir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "skl")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "skl")
}

// DataDir returns the directory holding the skill store and backups.
func DataDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "skl")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "skl")
}

// StoreDir returns the directory holding the single copy of each installed skill.
func StoreDir() string {
	return filepath.Join(DataDir(), "skills")
}

// BackupsDir returns the directory holding backups made before overwriting.
func BackupsDir() string {
	return filepath.Join(DataDir(), "backups")
}

var defaultConfig = `# Directories where skills are installed
directories:
  - ~/.kiro/skills       # Kiro
  - ~/.copilot/skills    # Copilot
  - ~/.claude/skills     # Claude
  - ~/.agents/skills     # Codex
  - ~/.pi/agent/skills   # Pi
`

// Load reads the config from disk, creating it with defaults if missing.
// Directories are returned ~-expanded and validated.
func Load() (*Config, error) {
	dir := Dir()
	path := filepath.Join(dir, "config.yaml")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(defaultConfig), 0o644); err != nil {
			return nil, err
		}
		data = []byte(defaultConfig)
	} else if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	store := StoreDir()
	for i, d := range cfg.Directories {
		expanded := filepath.Clean(expandPath(d))
		if rel, err := filepath.Rel(store, expanded); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("directory %q is inside the skill store %q", d, store)
		}
		cfg.Directories[i] = expanded
	}
	return &cfg, nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
