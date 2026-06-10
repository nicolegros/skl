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

	want := []string{"~/.skills"}
	if len(cfg.Directories) != 1 || cfg.Directories[0] != want[0] {
		t.Errorf("Load() directories = %v, want %v", cfg.Directories, want)
	}

	// Verify file was actually written
	configPath := filepath.Join(dir, "skl", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config.json was not created on disk")
	}
}

func TestLoad_ReadsExistingConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configDir := filepath.Join(dir, "skl")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{
		"directories": ["~/custom-dir", "/absolute/path"]
	}`), 0o644)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Directories) != 2 || cfg.Directories[0] != "~/custom-dir" || cfg.Directories[1] != "/absolute/path" {
		t.Errorf("Load() directories = %v, want [~/custom-dir /absolute/path]", cfg.Directories)
	}
}
