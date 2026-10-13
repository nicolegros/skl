package skills

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestUpdate_RefreshesStoreBehindLinks(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# v1"})
	env := newEnv(t, url, 2)
	if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo"}, env, skipAll); err != nil {
		t.Fatal(err)
	}
	up.tarball = makeTarball(t, "owner-repo-def456", map[string]string{"SKILL.md": "# v2"})

	result, err := Update("", env, skipAll)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(result.Results) != 1 || result.Results[0] != (Result{Name: "repo"}) {
		t.Errorf("Update() results = %+v", result.Results)
	}
	for _, dir := range env.Dirs {
		assertLinked(t, dir, "repo", env.Store)
		if got := readFile(t, filepath.Join(dir, "repo", "SKILL.md")); got != "# v2" {
			t.Errorf("SKILL.md through %s = %q, want # v2", dir, got)
		}
	}
	s := loadLock(t, env).Skills[0]
	if s.Commit != "def456" || s.Files["SKILL.md"] != sha("# v2") {
		t.Errorf("lock entry = %+v", s)
	}
	if up.refs[len(up.refs)-1] != "tarball" {
		t.Errorf("update fetched ref %q, want latest", up.refs[len(up.refs)-1])
	}
}

func TestUpdate_ResolvesDeclaredRef(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-old123", map[string]string{"SKILL.md": "# Old"})
	env := newEnv(t, url, 1)
	if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo", Ref: "v1"}, env, skipAll); err != nil {
		t.Fatal(err)
	}
	up.tarball = makeTarball(t, "owner-repo-new999", map[string]string{"SKILL.md": "# New"})

	if _, err := Update("repo", env, skipAll); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !slices.Equal(up.refs, []string{"v1", "v1"}) {
		t.Errorf("fetched refs = %v, want the declared ref each time", up.refs)
	}
	if s := loadLock(t, env).Skills[0]; s.Ref != "v1" || s.Commit != "new999" {
		t.Errorf("lock entry = %+v, want v1 at new999", s)
	}
}

func TestUpdate_SpecificSkillOnly(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-v1", map[string]string{
		"tdd/SKILL.md":   "# TDD v1",
		"grill/SKILL.md": "# Grill v1",
	})
	env := newEnv(t, url, 1)
	if _, err := InstallAll(InstallOptions{Owner: "owner", Repo: "repo"}, env, skipAll); err != nil {
		t.Fatal(err)
	}
	up.tarball = makeTarball(t, "owner-repo-v2", map[string]string{
		"tdd/SKILL.md":   "# TDD v2",
		"grill/SKILL.md": "# Grill v2",
	})

	result, err := Update("tdd", env, skipAll)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Name != "tdd" {
		t.Errorf("Update() results = %+v, want [tdd]", result.Results)
	}
	if got := readFile(t, filepath.Join(env.Dirs[0], "grill", "SKILL.md")); got != "# Grill v1" {
		t.Errorf("grill = %q, want untouched", got)
	}
	if got := readFile(t, filepath.Join(env.Dirs[0], "tdd", "SKILL.md")); got != "# TDD v2" {
		t.Errorf("tdd = %q, want v2", got)
	}
}

func TestUpdate_ErrorsOnUnknownSkill(t *testing.T) {
	env := newEnv(t, "", 1)
	if _, err := Update("nope", env, skipAll); err == nil {
		t.Fatal("Update() should error for a skill not in the lock")
	}
}

func TestUpdate_StoreEditedConflict(t *testing.T) {
	setup := func(t *testing.T) (Env, string) {
		up, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# v1"})
		env := newEnv(t, url, 1)
		if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo"}, env, skipAll); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(env.Dirs[0], "repo", "SKILL.md"), "# edited through the link")
		up.tarball = makeTarball(t, "owner-repo-def456", map[string]string{"SKILL.md": "# v2"})
		return env, filepath.Join(env.Store, "repo")
	}

	t.Run("skip", func(t *testing.T) {
		env, store := setup(t)
		r := &recorder{resolution: Skip}

		result, err := Update("", env, r.resolve)
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		if len(result.Results) != 1 || result.Results[0] != (Result{Name: "repo", Skipped: true}) {
			t.Errorf("Update() results = %+v, want repo skipped", result.Results)
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
		if ref := loadLock(t, env).Skills[0].Commit; ref != "abc123" {
			t.Errorf("lock ref = %q, want abc123", ref)
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		env, store := setup(t)

		if _, err := Update("", env, func(string, []Conflict) Resolution { return Overwrite }); err != nil {
			t.Fatalf("Update() error = %v", err)
		}
		if got := readFile(t, filepath.Join(store, "SKILL.md")); got != "# v2" {
			t.Errorf("store SKILL.md = %q, want # v2", got)
		}
		if ref := loadLock(t, env).Skills[0].Commit; ref != "def456" {
			t.Errorf("lock ref = %q, want def456", ref)
		}
		entries, _ := os.ReadDir(env.Store)
		if len(entries) != 1 {
			t.Errorf("store has %d entries, want no leftover temp dirs", len(entries))
		}
	})
}
