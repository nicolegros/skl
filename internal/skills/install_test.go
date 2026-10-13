package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func sha(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestInstall_StoresOnceAndLinksEveryDir(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"SKILL.md":   "# My Skill",
		"helpers.sh": "#!/bin/bash",
	})
	env := newEnv(t, url, 2)
	r := &recorder{}

	result, err := Install(InstallOptions{Owner: "owner", Repo: "repo", Ref: "abc123", Pinned: true}, env, r.resolve)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result != (Result{Name: "repo"}) {
		t.Errorf("Install() = %+v, want {Name: repo}", result)
	}

	if got := readFile(t, filepath.Join(env.Store, "repo", "helpers.sh")); got != "#!/bin/bash" {
		t.Errorf("store helpers.sh = %q", got)
	}
	for _, dir := range env.Dirs {
		assertLinked(t, dir, "repo", env.Store)
		if got := readFile(t, filepath.Join(dir, "repo", "SKILL.md")); got != "# My Skill" {
			t.Errorf("SKILL.md through link in %s = %q", dir, got)
		}
	}

	lf := loadLock(t, env)
	if len(lf.Skills) != 1 {
		t.Fatalf("lock has %d skills, want 1", len(lf.Skills))
	}
	s := lf.Skills[0]
	if s.Name != "repo" || s.Repo != "owner/repo" || s.Ref != "abc123" || !s.Pinned {
		t.Errorf("lock entry = %+v", s)
	}
	wantFiles := map[string]string{"SKILL.md": sha("# My Skill"), "helpers.sh": sha("#!/bin/bash")}
	if len(s.Files) != 2 || s.Files["SKILL.md"] != wantFiles["SKILL.md"] || s.Files["helpers.sh"] != wantFiles["helpers.sh"] {
		t.Errorf("lock files = %v, want %v", s.Files, wantFiles)
	}
	if !slices.Equal(s.Links, env.Dirs) {
		t.Errorf("lock links = %v, want %v", s.Links, env.Dirs)
	}
	if len(r.calls) != 0 {
		t.Errorf("resolver called %d times on a clean install", len(r.calls))
	}
}

func TestInstall_SubdirectorySkill(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "# Grill Me",
		"tdd/SKILL.md":      "# TDD",
		"README.md":         "# Repo",
	})
	env := newEnv(t, url, 1)

	result, err := Install(InstallOptions{Owner: "owner", Repo: "repo", Path: "grill-me", Ref: "abc123"}, env, skipAll)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.Name != "grill-me" {
		t.Errorf("Install() name = %q, want grill-me", result.Name)
	}

	entries, _ := os.ReadDir(env.Store)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"grill-me"}) {
		t.Errorf("store contains %v, want [grill-me]", names)
	}
	if got := readFile(t, filepath.Join(env.Dirs[0], "grill-me", "SKILL.md")); got != "# Grill Me" {
		t.Errorf("SKILL.md = %q", got)
	}
	if s := loadLock(t, env).Skills[0]; s.Path != "grill-me" {
		t.Errorf("lock path = %q, want grill-me", s.Path)
	}
}

func TestInstall_WithAlias_PatchesStoreAndChecksumsMatch(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md":   "---\nname: grill-me\ndescription: Grill the user\n---\nSee /grill-me/helpers.sh and /grill-me-harder",
		"grill-me/helpers.sh": "#!/bin/bash\n# Source: /grill-me/lib.sh",
	})
	env := newEnv(t, url, 2)

	result, err := Install(InstallOptions{Owner: "owner", Repo: "repo", Path: "grill-me", Ref: "abc123", Alias: "interview-me"}, env, skipAll)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.Name != "interview-me" {
		t.Errorf("Install() name = %q, want interview-me", result.Name)
	}

	wantSkill := "---\nname: interview-me\ndescription: Grill the user\n---\nSee /interview-me/helpers.sh and /grill-me-harder"
	if got := readFile(t, filepath.Join(env.Store, "interview-me", "SKILL.md")); got != wantSkill {
		t.Errorf("store SKILL.md = %q, want %q", got, wantSkill)
	}
	if got := readFile(t, filepath.Join(env.Store, "interview-me", "helpers.sh")); got != "#!/bin/bash\n# Source: /interview-me/lib.sh" {
		t.Errorf("store helpers.sh = %q", got)
	}
	for _, dir := range env.Dirs {
		assertLinked(t, dir, "interview-me", env.Store)
		if _, err := os.Lstat(filepath.Join(dir, "grill-me")); !os.IsNotExist(err) {
			t.Errorf("%s/grill-me exists, want only the alias", dir)
		}
	}

	s := loadLock(t, env).Skills[0]
	if s.Name != "grill-me" || s.Alias != "interview-me" {
		t.Errorf("lock entry = %+v", s)
	}
	if s.Files["SKILL.md"] != sha(wantSkill) {
		t.Errorf("lock checksum for SKILL.md = %q, want checksum of patched store copy", s.Files["SKILL.md"])
	}

	r := &recorder{}
	update, err := Update("interview-me", env, r.resolve)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("update after aliased install reported conflicts: %+v", r.calls)
	}
	if len(update.Results) != 1 || update.Results[0] != (Result{Name: "interview-me"}) {
		t.Errorf("Update() results = %+v", update.Results)
	}
}

func TestInstall_WithAlias_WarnsIfNoFrontmatterName(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "# Grill Me\nNo frontmatter here",
	})
	env := newEnv(t, url, 1)
	var logs []string
	env.Logf = func(format string, a ...any) { logs = append(logs, fmt.Sprintf(format, a...)) }

	if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo", Path: "grill-me", Alias: "interview-me"}, env, skipAll); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if !slices.Equal(logs, []string{"warning: SKILL.md has no frontmatter name field to patch"}) {
		t.Errorf("logs = %v", logs)
	}
	assertLinked(t, env.Dirs[0], "interview-me", env.Store)
}

func TestInstall_TwoAliasesOfSameSkillCoexist(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "---\nname: grill-me\n---\n",
	})
	env := newEnv(t, url, 1)

	for _, alias := range []string{"interview-me", "quiz-me"} {
		if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo", Path: "grill-me", Alias: alias}, env, skipAll); err != nil {
			t.Fatalf("Install(--as %s) error = %v", alias, err)
		}
	}

	lf := loadLock(t, env)
	var aliases []string
	for _, s := range lf.Skills {
		aliases = append(aliases, s.Alias)
	}
	if !slices.Equal(aliases, []string{"interview-me", "quiz-me"}) {
		t.Errorf("lock aliases = %v, want [interview-me quiz-me]", aliases)
	}
	for _, alias := range aliases {
		assertLinked(t, env.Dirs[0], alias, env.Store)
		if got := readFile(t, filepath.Join(env.Dirs[0], alias, "SKILL.md")); got != "---\nname: "+alias+"\n---\n" {
			t.Errorf("%s SKILL.md = %q", alias, got)
		}
	}
}

func TestInstall_WithAlias_BlocksNameOwnedByOther(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md":     "# Grill Me",
		"interview-me/SKILL.md": "# Interview Me",
	})

	t.Run("lock entry", func(t *testing.T) {
		env := newEnv(t, url, 1)
		if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo", Path: "interview-me"}, env, skipAll); err != nil {
			t.Fatal(err)
		}
		_, err := Install(InstallOptions{Owner: "owner", Repo: "repo", Path: "grill-me", Alias: "interview-me"}, env, skipAll)
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("Install() error = %v, want already exists", err)
		}
		if got := readFile(t, filepath.Join(env.Store, "interview-me", "SKILL.md")); got != "# Interview Me" {
			t.Errorf("store was overwritten: %q", got)
		}
	})

	t.Run("store dir", func(t *testing.T) {
		env := newEnv(t, url, 1)
		writeFile(t, filepath.Join(env.Store, "interview-me", "SKILL.md"), "# Orphan")
		_, err := Install(InstallOptions{Owner: "owner", Repo: "repo", Path: "grill-me", Alias: "interview-me"}, env, skipAll)
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("Install() error = %v, want already exists", err)
		}
	})
}

func TestInstall_FailsWithoutSkillMd(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"README.md": "# Not a skill"})
	env := newEnv(t, url, 1)

	if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo"}, env, skipAll); err == nil {
		t.Fatal("Install() should error when SKILL.md is missing")
	}
	if _, err := os.Stat(env.Store); err == nil {
		entries, _ := os.ReadDir(env.Store)
		if len(entries) != 0 {
			t.Errorf("store has %d entries after a failed install", len(entries))
		}
	}
}

func TestInstall_LocalDirConflict(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"SKILL.md":  "# Upstream",
		"prompt.md": "upstream prompt",
	})

	setup := func(t *testing.T) (Env, string) {
		env := newEnv(t, url, 2)
		local := filepath.Join(env.Dirs[0], "repo")
		writeFile(t, filepath.Join(local, "SKILL.md"), "# Hand written")
		writeFile(t, filepath.Join(local, "notes.md"), "my notes")
		return env, local
	}

	t.Run("skip", func(t *testing.T) {
		env, local := setup(t)
		r := &recorder{resolution: Skip}

		result, err := Install(InstallOptions{Owner: "owner", Repo: "repo"}, env, r.resolve)
		if err != nil {
			t.Fatalf("Install() error = %v", err)
		}
		if result != (Result{Name: "repo"}) {
			t.Errorf("Install() = %+v, want the store written", result)
		}
		if len(r.calls) != 1 || len(r.calls[0]) != 1 {
			t.Fatalf("resolver calls = %+v, want one call with one conflict", r.calls)
		}
		c := r.calls[0][0]
		if c.Kind != LocalDir || c.Path != local || !slices.Equal(c.Files, []string{"SKILL.md", "notes.md", "prompt.md"}) {
			t.Errorf("conflict = %+v", c)
		}
		if got := readFile(t, filepath.Join(local, "SKILL.md")); got != "# Hand written" {
			t.Errorf("skipped dir was changed: %q", got)
		}
		if info, _ := os.Lstat(local); info.Mode()&os.ModeSymlink != 0 {
			t.Error("skipped dir was replaced by a link")
		}
		assertLinked(t, env.Dirs[1], "repo", env.Store)
		if links := loadLock(t, env).Skills[0].Links; !slices.Equal(links, []string{env.Dirs[1]}) {
			t.Errorf("lock links = %v, want [%s]", links, env.Dirs[1])
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		env, _ := setup(t)
		r := &recorder{resolution: Overwrite}

		if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo"}, env, r.resolve); err != nil {
			t.Fatalf("Install() error = %v", err)
		}
		for _, dir := range env.Dirs {
			assertLinked(t, dir, "repo", env.Store)
		}
		if got := readFile(t, filepath.Join(env.Dirs[0], "repo", "SKILL.md")); got != "# Upstream" {
			t.Errorf("SKILL.md = %q", got)
		}
		if _, err := os.Stat(env.Backups); !os.IsNotExist(err) {
			t.Error("overwrite without backup wrote a backup")
		}
	})

	t.Run("backup and overwrite", func(t *testing.T) {
		env, local := setup(t)
		r := &recorder{resolution: BackupAndOverwrite}
		var logs []string
		env.Logf = func(format string, a ...any) { logs = append(logs, fmt.Sprintf(format, a...)) }

		if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo"}, env, r.resolve); err != nil {
			t.Fatalf("Install() error = %v", err)
		}
		assertLinked(t, env.Dirs[0], "repo", env.Store)

		matches, _ := filepath.Glob(filepath.Join(env.Backups, "repo", "*", local, "notes.md"))
		if len(matches) != 1 {
			t.Fatalf("backup matches = %v, want one mirrored copy of %s", matches, local)
		}
		if got := readFile(t, matches[0]); got != "my notes" {
			t.Errorf("backup notes.md = %q", got)
		}
		backupDir := filepath.Dir(matches[0])
		if got := readFile(t, filepath.Join(backupDir, "SKILL.md")); got != "# Hand written" {
			t.Errorf("backup SKILL.md = %q", got)
		}
		if !slices.Equal(logs, []string{fmt.Sprintf("Backed up %s to %s", local, backupDir)}) {
			t.Errorf("logs = %v", logs)
		}
	})
}

func TestInstall_IdenticalLocalDirIsReplacedWithoutConflict(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Same"})
	env := newEnv(t, url, 1)
	writeFile(t, filepath.Join(env.Dirs[0], "repo", "SKILL.md"), "# Same")
	r := &recorder{}

	if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo"}, env, r.resolve); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("resolver called with %+v", r.calls)
	}
	assertLinked(t, env.Dirs[0], "repo", env.Store)
}

func TestInstall_ForeignLinkConflict(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Upstream"})

	setup := func(t *testing.T) (Env, string, string) {
		env := newEnv(t, url, 1)
		elsewhere := filepath.Join(t.TempDir(), "mine")
		writeFile(t, filepath.Join(elsewhere, "SKILL.md"), "# Mine")
		link := filepath.Join(env.Dirs[0], "repo")
		os.MkdirAll(env.Dirs[0], 0o755)
		if err := os.Symlink(elsewhere, link); err != nil {
			t.Fatal(err)
		}
		return env, link, elsewhere
	}

	t.Run("skip", func(t *testing.T) {
		env, link, elsewhere := setup(t)
		r := &recorder{resolution: Skip}

		if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo"}, env, r.resolve); err != nil {
			t.Fatalf("Install() error = %v", err)
		}
		want := []Conflict{{Path: link, Kind: ForeignLink, Target: elsewhere}}
		if len(r.calls) != 1 || len(r.calls[0]) != 1 || r.calls[0][0].Path != want[0].Path || r.calls[0][0].Kind != ForeignLink || r.calls[0][0].Target != elsewhere {
			t.Fatalf("resolver calls = %+v, want [%+v]", r.calls, want)
		}
		if target, _ := os.Readlink(link); target != elsewhere {
			t.Errorf("foreign link now -> %q", target)
		}
	})

	t.Run("backup and overwrite", func(t *testing.T) {
		env, link, elsewhere := setup(t)

		if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo"}, env, func(string, []Conflict) Resolution { return BackupAndOverwrite }); err != nil {
			t.Fatalf("Install() error = %v", err)
		}
		assertLinked(t, env.Dirs[0], "repo", env.Store)
		matches, _ := filepath.Glob(filepath.Join(env.Backups, "repo", "*", link, "SKILL.md"))
		if len(matches) != 1 || readFile(t, matches[0]) != "# Mine" {
			t.Errorf("backup of link target = %v, want the target's SKILL.md", matches)
		}
		if got := readFile(t, filepath.Join(elsewhere, "SKILL.md")); got != "# Mine" {
			t.Errorf("link target was modified: %q", got)
		}
	})
}

func TestInstallAll_LinksEverySkill(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"tdd/SKILL.md":   "# TDD",
		"grill/SKILL.md": "# Grill",
	})
	env := newEnv(t, url, 2)

	results, err := InstallAll(InstallOptions{Owner: "owner", Repo: "repo"}, env, skipAll)
	if err != nil {
		t.Fatalf("InstallAll() error = %v", err)
	}
	var names []string
	for _, r := range results {
		names = append(names, r.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"grill", "tdd"}) {
		t.Errorf("InstallAll() names = %v", names)
	}
	for _, dir := range env.Dirs {
		assertLinked(t, dir, "tdd", env.Store)
		assertLinked(t, dir, "grill", env.Store)
	}
	lf := loadLock(t, env)
	if s, ok := lf.Get("tdd"); !ok || s.Path != "tdd" || s.Ref != "abc123" || !slices.Equal(s.Links, env.Dirs) {
		t.Errorf("lock tdd = %+v", s)
	}
}
