package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nicolegros/skl/internal/lock"
)

func TestRemove_DeletesLinksStoreAndLockEntry(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "---\nname: grill-me\n---\n",
	})
	env := newEnv(t, url, 2)
	if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo", Path: "grill-me", Alias: "interview-me"}, env, skipAll); err != nil {
		t.Fatal(err)
	}
	foreignTarget := t.TempDir()
	foreign := filepath.Join(env.Dirs[1], "interview-me")
	os.Remove(foreign)
	os.Symlink(foreignTarget, foreign)
	unrelated := filepath.Join(env.Dirs[0], "unrelated", "SKILL.md")
	writeFile(t, unrelated, "# Unrelated")
	r := &recorder{}

	if err := Remove("interview-me", env, r.resolve); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if _, err := os.Lstat(filepath.Join(env.Dirs[0], "interview-me")); !os.IsNotExist(err) {
		t.Error("our link still exists")
	}
	if _, err := os.Stat(filepath.Join(env.Store, "interview-me")); !os.IsNotExist(err) {
		t.Error("store dir still exists")
	}
	if target, _ := os.Readlink(foreign); target != foreignTarget {
		t.Errorf("foreign link -> %q, want untouched", target)
	}
	if readFile(t, unrelated) != "# Unrelated" {
		t.Error("unrelated skill changed")
	}
	if len(r.calls) != 0 {
		t.Errorf("resolver called with %+v", r.calls)
	}
	if lf := loadLock(t, env); len(lf.Skills) != 0 {
		t.Errorf("lock still has %+v", lf.Skills)
	}
}

func TestRemove_ErrorsOnUnknownSkill(t *testing.T) {
	if err := Remove("nonexistent", newEnv(t, "", 1), skipAll); err == nil {
		t.Fatal("Remove() should error for unknown skill")
	}
}

func TestRemove_LegacyCopyGoesThroughResolver(t *testing.T) {
	for _, tc := range []struct {
		resolution Resolution
		kept       bool
	}{{Skip, true}, {Overwrite, false}} {
		env := newEnv(t, "", 1)
		lock.Save(&lock.File{Skills: []lock.Skill{{Name: "tdd", Repo: "owner/repo", Path: "tdd", Ref: "abc"}}}, env.Lock)
		copyPath := filepath.Join(env.Dirs[0], "tdd")
		writeFile(t, filepath.Join(copyPath, "SKILL.md"), "# TDD")
		r := &recorder{resolution: tc.resolution}

		if err := Remove("tdd", env, r.resolve); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}
		if len(r.calls) != 1 || len(r.calls[0]) != 1 || r.calls[0][0].Kind != LocalDir || r.calls[0][0].Path != copyPath {
			t.Errorf("resolution %v: resolver calls = %+v", tc.resolution, r.calls)
		}
		if _, err := os.Stat(copyPath); (err == nil) != tc.kept {
			t.Errorf("resolution %v: copy exists = %v, want %v", tc.resolution, err == nil, tc.kept)
		}
		if lf := loadLock(t, env); len(lf.Skills) != 0 {
			t.Errorf("resolution %v: lock still has %+v", tc.resolution, lf.Skills)
		}
	}
}

func TestRemove_LeavesUserDirOfMigratedSkill(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "---\nname: grill-me\n---\n",
	})
	env := newEnv(t, url, 2)
	mine := filepath.Join(env.Dirs[1], "grill-me", "SKILL.md")
	writeFile(t, mine, "# Mine")
	if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo", Path: "grill-me"}, env, skipAll); err != nil {
		t.Fatal(err)
	}
	r := &recorder{resolution: Overwrite}

	if err := Remove("grill-me", env, r.resolve); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if readFile(t, mine) != "# Mine" {
		t.Error("user-owned dir changed")
	}
	if len(r.calls) != 0 {
		t.Errorf("resolver called with %+v", r.calls)
	}
	if _, err := os.Lstat(filepath.Join(env.Dirs[0], "grill-me")); !os.IsNotExist(err) {
		t.Error("our link still exists")
	}
}
