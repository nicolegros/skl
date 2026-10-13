package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nicolegros/skl/internal/config"
)

func sha(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestInstall_StoresOnceLinksEveryDirAndAddsLine(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"SKILL.md":   "# My Skill",
		"helpers.sh": "#!/bin/bash",
	})
	env := newEnv(t, url, 2)
	r := &recorder{}

	results, err := installAs(t, env, "repo", "owner/repo@v1", r.resolve)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	want := []Result{{Name: "repo", Source: config.Source{Repo: "owner/repo", Ref: "v1"}, Action: Installed}}
	if !slices.Equal(results, want) {
		t.Errorf("Install() = %+v, want %+v", results, want)
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
	if got, want := readFile(t, manifestPath()), dirsBlock(env)+"skills:\n  repo: owner/repo@v1\n"; got != want {
		t.Errorf("skl.yaml =\n%s\nwant\n%s", got, want)
	}

	lf := loadLock(t, env)
	if len(lf.Skills) != 1 {
		t.Fatalf("lock has %d skills, want 1", len(lf.Skills))
	}
	s := lf.Skills[0]
	if s.Name != "repo" || s.Repo != "owner/repo" || s.Ref != "v1" || s.Commit != "abc123" {
		t.Errorf("lock entry = %+v", s)
	}
	if len(s.Files) != 2 || s.Files["SKILL.md"] != sha("# My Skill") || s.Files["helpers.sh"] != sha("#!/bin/bash") {
		t.Errorf("lock files = %v", s.Files)
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

	mustInstall(t, env, "grill-me", "owner/repo/grill-me")

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

	mustInstall(t, env, "interview-me", "owner/repo/grill-me")

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
	if got, want := readFile(t, manifestPath()), dirsBlock(env)+"skills:\n  interview-me: owner/repo/grill-me\n"; got != want {
		t.Errorf("skl.yaml =\n%s\nwant\n%s", got, want)
	}

	s := loadLock(t, env).Skills[0]
	if s.Name != "grill-me" || s.Alias != "interview-me" {
		t.Errorf("lock entry = %+v", s)
	}
	if s.Files["SKILL.md"] != sha(wantSkill) {
		t.Errorf("lock checksum for SKILL.md = %q, want checksum of patched store copy", s.Files["SKILL.md"])
	}

	r := &recorder{}
	results, err := Converge(loadConfig(t), env, r.resolve)
	if err != nil || len(results) != 0 || len(r.calls) != 0 {
		t.Errorf("converge after aliased install = %+v, %v, conflicts %+v; want a silent no-op", results, err, r.calls)
	}
}

func TestInstall_WithAlias_WarnsIfNoFrontmatterName(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "# Grill Me\nNo frontmatter here",
	})
	env := newEnv(t, url, 1)
	var logs []string
	env.Logf = func(format string, a ...any) { logs = append(logs, fmt.Sprintf(format, a...)) }

	mustInstall(t, env, "interview-me", "owner/repo/grill-me")

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
		mustInstall(t, env, alias, "owner/repo/grill-me")
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

func TestInstall_NameListedFromAnotherSourceIsRefused(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md":     "# Grill Me",
		"interview-me/SKILL.md": "# Interview Me",
	})
	env := newEnv(t, url, 1)
	mustInstall(t, env, "interview-me", "owner/repo/interview-me")
	before := readFile(t, manifestPath())
	fetches := len(up.refs)

	_, err := installAs(t, env, "interview-me", "owner/repo/grill-me", skipAll)

	want := "interview-me is already installed from owner/repo/interview-me. Use --as <name>, or edit skl.yaml to switch source"
	if err == nil || err.Error() != want {
		t.Fatalf("Install() error = %v, want %q", err, want)
	}
	if got := readFile(t, manifestPath()); got != before {
		t.Errorf("skl.yaml changed to\n%s", got)
	}
	if got := readFile(t, filepath.Join(env.Store, "interview-me", "SKILL.md")); got != "# Interview Me" {
		t.Errorf("store was overwritten: %q", got)
	}
	if len(up.refs) != fetches {
		t.Errorf("refused install fetched %v", up.refs[fetches:])
	}
}

func TestInstall_SameSourceNewRefUpdatesLineAndReinstalls(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-abc123", map[string]string{"tdd/SKILL.md": "# TDD v1"})
	up.byRef["v2"] = makeTarball(t, "owner-repo-def456", map[string]string{"tdd/SKILL.md": "# TDD v2"})
	env := newEnv(t, url, 1)
	setSkills(t, env, "skills:\n  # testing\n  tdd: owner/repo/tdd   # mine\n")
	if _, err := Converge(loadConfig(t), env, skipAll); err != nil {
		t.Fatal(err)
	}

	results, err := installAs(t, env, "tdd", "owner/repo/tdd@v2", skipAll)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	want := []Result{{Name: "tdd", Source: config.Source{Repo: "owner/repo", Path: "tdd", Ref: "v2"}, Action: Installed}}
	if !slices.Equal(results, want) {
		t.Errorf("Install() = %+v, want %+v", results, want)
	}
	if got, want := readFile(t, manifestPath()), dirsBlock(env)+"skills:\n  # testing\n  tdd: owner/repo/tdd@v2   # mine\n"; got != want {
		t.Errorf("skl.yaml =\n%s\nwant\n%s", got, want)
	}
	if got := readFile(t, filepath.Join(env.Dirs[0], "tdd", "SKILL.md")); got != "# TDD v2" {
		t.Errorf("SKILL.md = %q, want # TDD v2", got)
	}
	if s := loadLock(t, env).Skills[0]; s.Ref != "v2" || s.Commit != "def456" {
		t.Errorf("lock entry = %+v, want v2 at def456", s)
	}
}

func TestInstall_FailedFetchLeavesManifestAndLockUnchanged(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"README.md": "# Not a skill"})
	env := newEnv(t, url, 1)
	before := readFile(t, manifestPath())

	results, err := installAs(t, env, "repo", "owner/repo", skipAll)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(results) != 1 || results[0].Err == nil || results[0].Err.Error() != "no SKILL.md found in " {
		t.Fatalf("Install() = %+v, want one result failing with no SKILL.md", results)
	}
	if got := readFile(t, manifestPath()); got != before {
		t.Errorf("skl.yaml changed to\n%s", got)
	}
	if lf := loadLock(t, env); len(lf.Skills) != 0 {
		t.Errorf("lock = %+v, want empty", lf.Skills)
	}
	entries, _ := os.ReadDir(env.Store)
	if len(entries) != 0 {
		t.Errorf("store has %d entries after a failed install", len(entries))
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

		results, err := installAs(t, env, "repo", "owner/repo", r.resolve)
		if err != nil {
			t.Fatalf("Install() error = %v", err)
		}
		if len(results) != 1 || results[0].Skipped || results[0].Err != nil {
			t.Errorf("Install() = %+v, want the store written", results)
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

		if _, err := installAs(t, env, "repo", "owner/repo", func(string, []Conflict) Resolution { return Overwrite }); err != nil {
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
		var logs []string
		env.Logf = func(format string, a ...any) { logs = append(logs, fmt.Sprintf(format, a...)) }

		if _, err := installAs(t, env, "repo", "owner/repo", func(string, []Conflict) Resolution { return BackupAndOverwrite }); err != nil {
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

	if _, err := installAs(t, env, "repo", "owner/repo", r.resolve); err != nil {
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

		if _, err := installAs(t, env, "repo", "owner/repo", r.resolve); err != nil {
			t.Fatalf("Install() error = %v", err)
		}
		if len(r.calls) != 1 || len(r.calls[0]) != 1 || r.calls[0][0].Path != link || r.calls[0][0].Kind != ForeignLink || r.calls[0][0].Target != elsewhere {
			t.Fatalf("resolver calls = %+v, want one ForeignLink conflict at %s", r.calls, link)
		}
		if target, _ := os.Readlink(link); target != elsewhere {
			t.Errorf("foreign link now -> %q", target)
		}
	})

	t.Run("backup and overwrite", func(t *testing.T) {
		env, link, elsewhere := setup(t)

		if _, err := installAs(t, env, "repo", "owner/repo", func(string, []Conflict) Resolution { return BackupAndOverwrite }); err != nil {
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

func TestInstallAll_WritesOneLinePerSkill(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"skills/tdd/SKILL.md":   "# TDD",
		"skills/grill/SKILL.md": "# Grill",
	})
	env := newEnv(t, url, 2)
	src, _ := config.ParseSource("owner/repo@v1")

	wanted, err := DiscoverAll(src, env)
	if err != nil {
		t.Fatalf("DiscoverAll() error = %v", err)
	}
	results, err := Install(loadConfig(t), wanted, env, skipAll)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	var printed []string
	for _, r := range results {
		printed = append(printed, r.Name+" from "+r.Source.String())
	}
	if want := []string{"grill from owner/repo/skills/grill@v1", "tdd from owner/repo/skills/tdd@v1"}; !slices.Equal(printed, want) {
		t.Errorf("results = %v, want %v", printed, want)
	}
	if got, want := readFile(t, manifestPath()), dirsBlock(env)+"skills:\n  grill: owner/repo/skills/grill@v1\n  tdd: owner/repo/skills/tdd@v1\n"; got != want {
		t.Errorf("skl.yaml =\n%s\nwant\n%s", got, want)
	}
	for _, dir := range env.Dirs {
		assertLinked(t, dir, "tdd", env.Store)
		assertLinked(t, dir, "grill", env.Store)
	}
}
