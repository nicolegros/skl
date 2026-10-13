package skills

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nicolegros/skl/internal/lock"
)

// legacyInstall writes a pre-symlink install: a lock entry without links and real copies in every dir.
func legacyInstall(t *testing.T, env Env, content string) {
	t.Helper()
	lock.Save(&lock.File{Skills: []lock.Skill{
		{Name: "repo", Repo: "owner/repo", Commit: "abc123", Files: map[string]string{"SKILL.md": sha(content)}},
	}}, env.Lock)
	for _, dir := range env.Dirs {
		writeFile(t, filepath.Join(dir, "repo", "SKILL.md"), content)
	}
}

func TestReconcile_MigratesIdenticalCopiesWithoutPrompting(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Upstream"})
	env := newEnv(t, url, 2)
	legacyInstall(t, env, "# Upstream")
	r := &recorder{}

	if err := Reconcile(env, r.resolve); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if !slices.Equal(up.refs, []string{"abc123"}) {
		t.Errorf("fetched refs = %v, want the locked ref only", up.refs)
	}
	if len(r.calls) != 0 {
		t.Errorf("resolver called with %+v", r.calls)
	}
	for _, dir := range env.Dirs {
		assertLinked(t, dir, "repo", env.Store)
	}
	if s := loadLock(t, env).Skills[0]; !slices.Equal(s.Links, env.Dirs) || s.Commit != "abc123" {
		t.Errorf("lock entry = %+v", s)
	}
}

func TestReconcile_MigrationWithEditedCopy(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Upstream"})
	env := newEnv(t, url, 2)
	legacyInstall(t, env, "# Upstream")
	edited := filepath.Join(env.Dirs[0], "repo")
	writeFile(t, filepath.Join(edited, "SKILL.md"), "# Edited")
	r := &recorder{resolution: Skip}

	if err := Reconcile(env, r.resolve); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(r.calls) != 1 || len(r.calls[0]) != 1 || r.calls[0][0].Kind != LocalDir || r.calls[0][0].Path != edited {
		t.Fatalf("resolver calls = %+v, want one LocalDir conflict at %s", r.calls, edited)
	}
	assertLinked(t, env.Dirs[1], "repo", env.Store)
	if links := loadLock(t, env).Skills[0].Links; !slices.Equal(links, []string{env.Dirs[1]}) {
		t.Errorf("lock links = %v", links)
	}

	second := &recorder{resolution: Overwrite}
	if err := Reconcile(env, second.resolve); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if len(second.calls) != 0 {
		t.Errorf("second reconcile prompted again: %+v", second.calls)
	}
	if got := readFile(t, filepath.Join(edited, "SKILL.md")); got != "# Edited" {
		t.Errorf("skipped dir changed to %q", got)
	}
}

func TestReconcile_MigrationSkippedEverywhereStillMarksMigrated(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Upstream"})
	env := newEnv(t, url, 1)
	legacyInstall(t, env, "# Edited")

	if err := Reconcile(env, skipAll); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if links := loadLock(t, env).Skills[0].Links; links == nil || len(links) != 0 {
		t.Errorf("lock links = %#v, want empty non-nil", links)
	}
}

func TestReconcile_RemovesOurLinksFromUnconfiguredDirs(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"tdd/SKILL.md":   "# TDD",
		"grill/SKILL.md": "# Grill",
	})
	env := newEnv(t, url, 2)
	if _, err := InstallAll(InstallOptions{Owner: "owner", Repo: "repo"}, env, skipAll); err != nil {
		t.Fatal(err)
	}
	dropped := env.Dirs[1]
	foreign := filepath.Join(dropped, "grill")
	os.Remove(foreign)
	os.Symlink(t.TempDir(), foreign)

	env.Dirs = env.Dirs[:1]
	if err := Reconcile(env, skipAll); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if _, err := os.Lstat(filepath.Join(dropped, "tdd")); !os.IsNotExist(err) {
		t.Error("our link in the dropped dir was kept")
	}
	if info, err := os.Lstat(foreign); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("foreign link in the dropped dir was removed")
	}
	assertLinked(t, env.Dirs[0], "tdd", env.Store)
	for _, s := range loadLock(t, env).Skills {
		if !slices.Equal(s.Links, env.Dirs) {
			t.Errorf("%s links = %v, want %v", s.Name, s.Links, env.Dirs)
		}
	}
}

func TestReconcile_IsIdempotent(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Upstream"})
	env := newEnv(t, url, 2)
	legacyInstall(t, env, "# Upstream")
	writeFile(t, filepath.Join(env.Dirs[0], "repo", "SKILL.md"), "# Edited")

	if err := Reconcile(env, func(string, []Conflict) Resolution { return Overwrite }); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, env)

	r := &recorder{}
	if err := Reconcile(env, r.resolve); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("second reconcile called the resolver: %+v", r.calls)
	}
	if after := snapshot(t, env); !slices.Equal(before, after) {
		t.Errorf("second reconcile changed disk:\nbefore %v\nafter  %v", before, after)
	}
}

func TestReconcile_ErrorsOnEmptyLock(t *testing.T) {
	if err := Reconcile(newEnv(t, "", 1), skipAll); err == nil {
		t.Fatal("Reconcile() should error for an empty lock")
	}
}

// snapshot lists every path under the store and agent dirs with its link target or content, plus the lock.
func snapshot(t *testing.T, env Env) []string {
	t.Helper()
	var out []string
	for _, root := range append([]string{env.Store}, env.Dirs...) {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			entry := path
			if d.Type()&fs.ModeSymlink != 0 {
				target, _ := os.Readlink(path)
				entry += " -> " + target
			} else if !d.IsDir() {
				entry += " = " + readFile(t, path)
			}
			out = append(out, entry)
			return nil
		})
	}
	return append(out, readFile(t, env.Lock))
}
