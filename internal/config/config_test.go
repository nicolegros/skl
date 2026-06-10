package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDir_UsesXDGWhenSet(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got := Dir()
	want := filepath.Join(xdg, "skl")

	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestConfigDir_FallsBackToDotConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	home, _ := os.UserHomeDir()
	got := Dir()
	want := filepath.Join(home, ".config", "skl")

	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestLoad_CreatesDefaultConfigWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := []string{
		"~/.kiro/skills",
		"~/.copilot/skills",
		"~/.claude/skills",
		"~/.agents/skills",
		"~/.pi/agent/skills",
	}
	if len(cfg.Directories) != len(want) {
		t.Fatalf("Load() directories = %v, want %v", cfg.Directories, want)
	}
	for i, d := range cfg.Directories {
		if d != want[i] {
			t.Errorf("directories[%d] = %q, want %q", i, d, want[i])
		}
	}

	// Verify file was actually written as YAML
	configPath := filepath.Join(dir, "skl", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config.yaml was not created on disk")
	}
}

func TestLoad_ReadsExistingConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configDir := filepath.Join(dir, "skl")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("directories:\n  - ~/custom-dir\n  - /absolute/path\n"), 0o644)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Directories) != 2 || cfg.Directories[0] != "~/custom-dir" || cfg.Directories[1] != "/absolute/path" {
		t.Errorf("Load() directories = %v, want [~/custom-dir /absolute/path]", cfg.Directories)
	}
}
