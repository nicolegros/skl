package skills

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nicolegros/skl/internal/config"
)

func TestUpdate_MovesDefaultBranchKeepsTagAndNeverWritesManifest(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"tdd/SKILL.md":   "# TDD v1",
		"grill/SKILL.md": "# Grill v1",
	})
	up.byRef["v1"] = makeTarball(t, "owner-repo-abc123", map[string]string{
		"tdd/SKILL.md":   "# TDD v1",
		"grill/SKILL.md": "# Grill v1",
	})
	env := newEnv(t, url, 2)
	manifest := "skills:\n  grill: owner/repo/grill@v1   # pinned\n  tdd: owner/repo/tdd\n"
	setSkills(t, env, manifest)
	runConverge(t, env, skipAll)
	up.tarball = makeTarball(t, "owner-repo-def456", map[string]string{
		"tdd/SKILL.md":   "# TDD v2",
		"grill/SKILL.md": "# Grill v2",
	})

	results, err := Update(loadConfig(t), "", env, skipAll)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	want := []Result{{Name: "tdd", Source: config.Source{Repo: "owner/repo", Path: "tdd"}, Action: Updated}}
	if !slices.Equal(results, want) {
		t.Errorf("Update() = %+v, want %+v", results, want)
	}
	for _, dir := range env.Dirs {
		assertLinked(t, dir, "tdd", env.Store)
		if got := readFile(t, filepath.Join(dir, "tdd", "SKILL.md")); got != "# TDD v2" {
			t.Errorf("tdd through %s = %q, want # TDD v2", dir, got)
		}
		if got := readFile(t, filepath.Join(dir, "grill", "SKILL.md")); got != "# Grill v1" {
			t.Errorf("grill through %s = %q, want the v1 tag kept", dir, got)
		}
	}
	lf := loadLock(t, env)
	if s, _ := lf.Get("tdd"); s.Ref != "" || s.Commit != "def456" || s.Files["SKILL.md"] != sha("# TDD v2") {
		t.Errorf("lock tdd = %+v", s)
	}
	if s, _ := lf.Get("grill"); s.Ref != "v1" || s.Commit != "abc123" {
		t.Errorf("lock grill = %+v", s)
	}
	if got := readFile(t, manifestPath()); got != dirsBlock(env)+manifest {
		t.Errorf("update rewrote skl.yaml:\n%s", got)
	}
}

func TestUpdate_SpecificSkillOnly(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-v1", map[string]string{
		"tdd/SKILL.md":   "# TDD v1",
		"grill/SKILL.md": "# Grill v1",
	})
	env := newEnv(t, url, 1)
	mustInstall(t, env, "tdd", "owner/repo/tdd")
	mustInstall(t, env, "grill", "owner/repo/grill")
	up.tarball = makeTarball(t, "owner-repo-v2", map[string]string{
		"tdd/SKILL.md":   "# TDD v2",
		"grill/SKILL.md": "# Grill v2",
	})

	results, err := Update(loadConfig(t), "tdd", env, skipAll)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(results) != 1 || results[0].Name != "tdd" {
		t.Errorf("Update() = %+v, want [tdd]", results)
	}
	if got := readFile(t, filepath.Join(env.Dirs[0], "grill", "SKILL.md")); got != "# Grill v1" {
		t.Errorf("grill = %q, want untouched", got)
	}
	if got := readFile(t, filepath.Join(env.Dirs[0], "tdd", "SKILL.md")); got != "# TDD v2" {
		t.Errorf("tdd = %q, want v2", got)
	}
}

func TestUpdate_ErrorsOnSkillNotInManifest(t *testing.T) {
	env := newEnv(t, "", 1)
	_, err := Update(loadConfig(t), "nope", env, skipAll)
	if err == nil || err.Error() != `skill "nope" not found in skl.yaml` {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestUpdate_StoreEditedConflict(t *testing.T) {
	setup := func(t *testing.T) (Env, string) {
		up, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# v1"})
		env := newEnv(t, url, 1)
		mustInstall(t, env, "repo", "owner/repo")
		writeFile(t, filepath.Join(env.Dirs[0], "repo", "SKILL.md"), "# edited through the link")
		up.tarball = makeTarball(t, "owner-repo-def456", map[string]string{"SKILL.md": "# v2"})
		return env, filepath.Join(env.Store, "repo")
	}

	t.Run("skip", func(t *testing.T) {
		env, store := setup(t)
		r := &recorder{resolution: Skip}

		results, err := Update(loadConfig(t), "", env, r.resolve)
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		if len(results) != 1 || !results[0].Skipped || results[0].Action != Updated {
			t.Errorf("Update() = %+v, want repo skipped", results)
		}
		if len(r.calls) != 1 || len(r.calls[0]) != 1 {
			t.Fatalf("resolver calls = %+v", r.calls)
		}
		if c := r.calls[0][0]; c.Kind != StoreEdited || c.Path != store || !slices.Equal(c.Files, []string{"SKILL.md"}) {
			t.Errorf("conflict = %+v", c)
		}
		if got := readFile(t, filepath.Join(store, "SKILL.md")); got != "# edited through the link" {
			t.Errorf("store SKILL.md = %q, want the edit kept", got)
		}
		if commit := loadLock(t, env).Skills[0].Commit; commit != "abc123" {
			t.Errorf("lock commit = %q, want abc123", commit)
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		env, store := setup(t)

		if _, err := Update(loadConfig(t), "", env, func(string, []Conflict) Resolution { return Overwrite }); err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		if got := readFile(t, filepath.Join(store, "SKILL.md")); got != "# v2" {
			t.Errorf("store SKILL.md = %q, want # v2", got)
		}
		if commit := loadLock(t, env).Skills[0].Commit; commit != "def456" {
			t.Errorf("lock commit = %q, want def456", commit)
		}
		entries, _ := os.ReadDir(env.Store)
		if len(entries) != 1 {
			t.Errorf("store has %d entries, want no leftover temp dirs", len(entries))
		}
	})
}
