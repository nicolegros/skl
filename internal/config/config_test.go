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

	home, _ := os.UserHomeDir()
	want := []string{
		filepath.Join(home, ".kiro/skills"),
		filepath.Join(home, ".copilot/skills"),
		filepath.Join(home, ".claude/skills"),
		filepath.Join(home, ".agents/skills"),
		filepath.Join(home, ".pi/agent/skills"),
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

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "custom-dir")
	if len(cfg.Directories) != 2 || cfg.Directories[0] != want || cfg.Directories[1] != "/absolute/path" {
		t.Errorf("Load() directories = %v, want [%s /absolute/path]", cfg.Directories, want)
	}
}

func TestDataDir_UsesXDGWhenSet(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)

	if got, want := DataDir(), filepath.Join(xdg, "skl"); got != want {
		t.Errorf("DataDir() = %q, want %q", got, want)
	}
}

func TestDataDir_FallsBackToLocalShare(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")

	home, _ := os.UserHomeDir()
	if got, want := DataDir(), filepath.Join(home, ".local", "share", "skl"); got != want {
		t.Errorf("DataDir() = %q, want %q", got, want)
	}
}

func TestLoad_RejectsDirectoryInsideStore(t *testing.T) {
	for _, sub := range []string{"", "nested"} {
		dir := t.TempDir()
		data := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		t.Setenv("XDG_DATA_HOME", data)

		bad := filepath.Join(data, "skl", "skills", sub)
		os.MkdirAll(filepath.Join(dir, "skl"), 0o755)
		os.WriteFile(filepath.Join(dir, "skl", "config.yaml"), []byte("directories:\n  - "+bad+"\n"), 0o644)

		if _, err := Load(); err == nil {
			t.Errorf("Load() accepted %q inside the store", bad)
		}
	}
}

func TestLoad_AcceptsSiblingOfStore(t *testing.T) {
	dir := t.TempDir()
	data := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_DATA_HOME", data)

	sibling := filepath.Join(data, "skl", "skills-other")
	os.MkdirAll(filepath.Join(dir, "skl"), 0o755)
	os.WriteFile(filepath.Join(dir, "skl", "config.yaml"), []byte("directories:\n  - "+sibling+"\n"), 0o644)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Directories) != 1 || cfg.Directories[0] != sibling {
		t.Errorf("Load() directories = %v, want [%s]", cfg.Directories, sibling)
	}
}
