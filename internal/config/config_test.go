package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigDir_UsesXDGWhenSet(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	if got, want := Dir(), filepath.Join(xdg, "skl"); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestConfigDir_FallsBackToDotConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	home, _ := os.UserHomeDir()
	if got, want := Dir(), filepath.Join(home, ".config", "skl"); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
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

// setup points XDG at fresh dirs and returns the skl config dir.
func setup(t *testing.T) string {
	t.Helper()
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := filepath.Join(cfg, "skl")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLoad_CreatesDefaultManifestWhenNothingExists(t *testing.T) {
	dir := setup(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := `# Directories where skills are installed
directories:
  - ~/.kiro/skills       # Kiro
  - ~/.copilot/skills    # Copilot
  - ~/.claude/skills     # Claude
  - ~/.agents/skills     # Codex
  - ~/.pi/agent/skills   # Pi
skills: {}
`
	if got := read(t, filepath.Join(dir, "skl.yaml")); got != want {
		t.Errorf("skl.yaml =\n%s\nwant\n%s", got, want)
	}
	home, _ := os.UserHomeDir()
	if len(cfg.Directories) != 5 || cfg.Directories[2] != filepath.Join(home, ".claude/skills") {
		t.Errorf("Directories = %v", cfg.Directories)
	}
	if len(cfg.Skills) != 0 {
		t.Errorf("Skills = %v, want none", cfg.Skills)
	}
}

func TestLoad_SeedsFromLockWhenConfigYamlIsMissing(t *testing.T) {
	dir := setup(t)
	write(t, filepath.Join(dir, "skl.lock"), `{"skills": [
  {"name": "tdd", "repo": "owner/repo", "path": "tdd", "ref": "abc1234", "pinned": false, "links": []}
]}`)

	if _, err := Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := defaultDirectories + "skills:\n  tdd: owner/repo/tdd\n"
	if got := read(t, filepath.Join(dir, "skl.yaml")); got != want {
		t.Errorf("skl.yaml =\n%s\nwant\n%s", got, want)
	}
}

func TestLoad_ReadsManifest(t *testing.T) {
	dir := setup(t)
	write(t, filepath.Join(dir, "skl.yaml"), `directories:
  - ~/custom-dir
  - /absolute/path
skills:
  axi: kunchenguid/axi
  interview-me: mattpocock/skills/grill-me@v1.0.0
`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	home, _ := os.UserHomeDir()
	if len(cfg.Directories) != 2 || cfg.Directories[0] != filepath.Join(home, "custom-dir") || cfg.Directories[1] != "/absolute/path" {
		t.Errorf("Directories = %v", cfg.Directories)
	}
	want := map[string]Source{
		"axi":          {Repo: "kunchenguid/axi"},
		"interview-me": {Repo: "mattpocock/skills", Path: "grill-me", Ref: "v1.0.0"},
	}
	if len(cfg.Skills) != 2 || cfg.Skills["axi"] != want["axi"] || cfg.Skills["interview-me"] != want["interview-me"] {
		t.Errorf("Skills = %+v, want %+v", cfg.Skills, want)
	}
}

func TestLoad_MigratesConfigYamlAndLock(t *testing.T) {
	dir := setup(t)
	config := `# Directories where skills are installed
directories:
  - ~/.claude/skills     # Claude

  - ~/.pi/agent/skills   # Pi
`
	write(t, filepath.Join(dir, "config.yaml"), config)
	lockJSON := `{"skills": [
  {"name": "tdd", "repo": "owner/repo", "path": "tdd", "ref": "abc1234", "pinned": false, "links": []},
  {"name": "grill-me", "repo": "mattpocock/skills", "path": "grill-me", "ref": "def5678", "pinned": true, "alias": "interview-me", "links": []},
  {"name": "axi", "repo": "kunchenguid/axi", "path": "", "ref": "0123abc", "pinned": false, "links": []}
]}`
	write(t, filepath.Join(dir, "skl.lock"), lockJSON)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := config + `skills:
  axi: kunchenguid/axi
  interview-me: mattpocock/skills/grill-me@def5678
  tdd: owner/repo/tdd
`
	if got := read(t, filepath.Join(dir, "skl.yaml")); got != want {
		t.Errorf("skl.yaml =\n%s\nwant\n%s", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); !os.IsNotExist(err) {
		t.Error("config.yaml still exists after migration")
	}
	if got := read(t, filepath.Join(dir, "skl.lock")); got != lockJSON {
		t.Errorf("migration rewrote the lock:\n%s", got)
	}
	if len(cfg.Skills) != 3 || cfg.Skills["interview-me"] != (Source{Repo: "mattpocock/skills", Path: "grill-me", Ref: "def5678"}) {
		t.Errorf("Skills = %+v", cfg.Skills)
	}
}

func TestSave_EditsOnlySkillEntries(t *testing.T) {
	dir := setup(t)
	write(t, filepath.Join(dir, "skl.yaml"), `# Directories where skills are installed
directories:
  - /a/skills       # A

  - /b/skills       # B

# What I want installed
skills:
  # the AXI one
  axi: kunchenguid/axi   # keep me
  grill-me: "mattpocock/skills/grill-me" # quoted
  old: owner/old
  tdd: owner/repo/tdd
`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	cfg.Skills["aaa"] = Source{Repo: "owner/aaa"}
	cfg.Skills["beta"] = Source{Repo: "owner/beta", Ref: "v2"}
	cfg.Skills["zed"] = Source{Repo: "owner/zed"}
	cfg.Skills["axi"] = Source{Repo: "kunchenguid/axi", Ref: "v1"}
	cfg.Skills["grill-me"] = Source{Repo: "mattpocock/skills", Path: "grill-me", Ref: "main"}
	delete(cfg.Skills, "old")
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	want := `# Directories where skills are installed
directories:
  - /a/skills       # A

  - /b/skills       # B

# What I want installed
skills:
  aaa: owner/aaa
  # the AXI one
  axi: kunchenguid/axi@v1   # keep me
  beta: owner/beta@v2
  grill-me: mattpocock/skills/grill-me@main # quoted
  tdd: owner/repo/tdd
  zed: owner/zed
`
	if got := read(t, filepath.Join(dir, "skl.yaml")); got != want {
		t.Errorf("skl.yaml =\n%s\nwant\n%s", got, want)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("reloading saved file: %v", err)
	}
	if len(reloaded.Skills) != 6 || reloaded.Skills["grill-me"].Ref != "main" {
		t.Errorf("reloaded Skills = %+v", reloaded.Skills)
	}
}

func TestSave_DeletesEntryWithItsComment(t *testing.T) {
	dir := setup(t)
	write(t, filepath.Join(dir, "skl.yaml"), "directories: []\nskills:\n  a: owner/a\n  # about b\n  b: owner/b\n  c: owner/c\n")
	cfg, _ := Load()

	delete(cfg.Skills, "b")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, filepath.Join(dir, "skl.yaml")), "directories: []\nskills:\n  a: owner/a\n  c: owner/c\n"; got != want {
		t.Errorf("skl.yaml = %q, want %q", got, want)
	}
}

func TestSave_EmptyMapRoundTrip(t *testing.T) {
	dir := setup(t)
	path := filepath.Join(dir, "skl.yaml")
	write(t, path, "directories:\n  - /a   # A\nskills:\n  only: owner/only\n")
	cfg, _ := Load()

	delete(cfg.Skills, "only")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, path), "directories:\n  - /a   # A\nskills: {}\n"; got != want {
		t.Errorf("after removing the last entry skl.yaml = %q, want %q", got, want)
	}

	cfg.Skills["new"] = Source{Repo: "owner/new", Path: "x"}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, path), "directories:\n  - /a   # A\nskills:\n  new: owner/new/x\n"; got != want {
		t.Errorf("after adding to {} skl.yaml = %q, want %q", got, want)
	}
}

func TestLoad_RejectsInvalidManifestWithLine(t *testing.T) {
	for _, tc := range []struct {
		name, content, want string
	}{
		{"bad yaml", "directories: []\nskills:\n  a: [\n", "skl.yaml: yaml: line 3"},
		{"bad source", "directories: []\nskills:\n  a: owner/a\n  b: nope\n", `skl.yaml:4: b: invalid source "nope"`},
		{"empty name", "directories: []\nskills:\n  \"\": owner/a\n", "skl.yaml:3: empty skill name"},
		{"duplicate", "directories: []\nskills:\n  a: owner/a\n  a: owner/b\n", `skl.yaml:4: skill "a" is listed twice`},
		{"not a string", "directories: []\nskills:\n  a:\n    repo: x\n", "skl.yaml:4: a: source must be a single-line string"},
		{"unknown key", "directories: []\nskill:\n  a: owner/a\n", `skl.yaml:2: unknown key "skill"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := setup(t)
			path := filepath.Join(dir, "skl.yaml")
			write(t, path, tc.content)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load() error = %v, want it to contain %q", err, tc.want)
			}
			if got := read(t, path); got != tc.content {
				t.Errorf("invalid skl.yaml was rewritten to %q", got)
			}
		})
	}
}

func TestLoad_RejectsDirectoryInsideStore(t *testing.T) {
	for _, sub := range []string{"", "nested"} {
		dir := setup(t)
		bad := filepath.Join(StoreDir(), sub)
		write(t, filepath.Join(dir, "skl.yaml"), "directories:\n  - "+bad+"\nskills: {}\n")

		if _, err := Load(); err == nil {
			t.Errorf("Load() accepted %q inside the store", bad)
		}
	}
}

func TestLoad_AcceptsSiblingOfStore(t *testing.T) {
	dir := setup(t)
	sibling := filepath.Join(DataDir(), "skills-other")
	write(t, filepath.Join(dir, "skl.yaml"), "directories:\n  - "+sibling+"\nskills: {}\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Directories) != 1 || cfg.Directories[0] != sibling {
		t.Errorf("Directories = %v, want [%s]", cfg.Directories, sibling)
	}
}

func TestParseSource(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Source
		str  string
		name string
	}{
		{"owner/repo", Source{Repo: "owner/repo"}, "owner/repo", "repo"},
		{"owner/repo/skills/grill-me", Source{Repo: "owner/repo", Path: "skills/grill-me"}, "owner/repo/skills/grill-me", "grill-me"},
		{"owner/repo/grill-me@v1.0.0", Source{Repo: "owner/repo", Path: "grill-me", Ref: "v1.0.0"}, "owner/repo/grill-me@v1.0.0", "grill-me"},
		{"owner/repo@abc1234", Source{Repo: "owner/repo", Ref: "abc1234"}, "owner/repo@abc1234", "repo"},
		{"owner/repo/", Source{Repo: "owner/repo"}, "owner/repo", "repo"},
		{"https://github.com/owner/repo", Source{Repo: "owner/repo"}, "owner/repo", "repo"},
		{"https://github.com/owner/repo.git", Source{Repo: "owner/repo"}, "owner/repo", "repo"},
		{"http://www.github.com/owner/repo/", Source{Repo: "owner/repo"}, "owner/repo", "repo"},
		{"https://github.com/owner/repo/tree/main/skills/tdd", Source{Repo: "owner/repo", Path: "skills/tdd", Ref: "main"}, "owner/repo/skills/tdd@main", "tdd"},
		{"https://github.com/owner/repo/tree/v2", Source{Repo: "owner/repo", Ref: "v2"}, "owner/repo@v2", "repo"},
	} {
		got, err := ParseSource(tc.in)
		if err != nil {
			t.Errorf("ParseSource(%q) error = %v", tc.in, err)
			continue
		}
		if got != tc.want || got.String() != tc.str || got.Name() != tc.name {
			t.Errorf("ParseSource(%q) = %+v (%q, name %q), want %+v (%q, name %q)", tc.in, got, got.String(), got.Name(), tc.want, tc.str, tc.name)
		}
	}

	for _, in := range []string{"", "invalid", "owner/", "/repo", "owner//repo", "owner/repo@", "owner/repo/../x", "owner/re po", "https://gitlab.com/owner/repo", "https://github.com/owner/repo/tree/main@v1"} {
		if got, err := ParseSource(in); err == nil {
			t.Errorf("ParseSource(%q) = %+v, want error", in, got)
		}
	}
}
