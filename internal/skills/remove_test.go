package skills

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/lock"
)

func TestRemove_DropsLineAndDeletesLinksStoreAndLockEntry(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "---\nname: grill-me\n---\n",
		"tdd/SKILL.md":      "# TDD",
	})
	env := newEnv(t, url, 2)
	mustInstall(t, env, "interview-me", "owner/repo/grill-me")
	mustInstall(t, env, "tdd", "owner/repo/tdd")
	foreignTarget := t.TempDir()
	foreign := filepath.Join(env.Dirs[1], "interview-me")
	os.Remove(foreign)
	os.Symlink(foreignTarget, foreign)
	unrelated := filepath.Join(env.Dirs[0], "unrelated", "SKILL.md")
	writeFile(t, unrelated, "# Unrelated")
	r := &recorder{}

	results, err := Remove(loadConfig(t), "interview-me", env, r.resolve)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	want := []Result{{Name: "interview-me", Source: config.Source{Repo: "owner/repo", Path: "grill-me"}, Action: Removed}}
	if !slices.Equal(results, want) {
		t.Errorf("Remove() = %+v, want %+v", results, want)
	}
	if got, want := readFile(t, manifestPath()), dirsBlock(env)+"skills:\n  tdd: owner/repo/tdd\n"; got != want {
		t.Errorf("skl.yaml =\n%s\nwant\n%s", got, want)
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
	if lf := loadLock(t, env); len(lf.Skills) != 1 || lf.Skills[0].Name != "tdd" {
		t.Errorf("lock = %+v, want only tdd", lf.Skills)
	}
}

func TestRemove_EditedStoreCopyGoesThroughResolver(t *testing.T) {
	for _, tc := range []struct {
		resolution Resolution
		kept       bool
	}{{Skip, true}, {Overwrite, false}} {
		_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Upstream"})
		env := newEnv(t, url, 1)
		mustInstall(t, env, "repo", "owner/repo")
		writeFile(t, filepath.Join(env.Store, "repo", "SKILL.md"), "# Edited")
		r := &recorder{resolution: tc.resolution}

		results, err := Remove(loadConfig(t), "repo", env, r.resolve)
		if err != nil {
			t.Fatalf("Remove() error = %v", err)
		}
		if len(results) != 1 || results[0].Skipped != tc.kept {
			t.Errorf("resolution %v: Remove() = %+v, want skipped %v", tc.resolution, results, tc.kept)
		}
		if len(r.calls) != 1 || r.calls[0][0].Kind != StoreEdited {
			t.Errorf("resolution %v: resolver calls = %+v, want one StoreEdited conflict", tc.resolution, r.calls)
		}
		if _, err := os.Stat(filepath.Join(env.Dirs[0], "repo", "SKILL.md")); (err == nil) != tc.kept {
			t.Errorf("resolution %v: skill reachable through link = %v, want %v", tc.resolution, err == nil, tc.kept)
		}
		if lf := loadLock(t, env); (len(lf.Skills) == 1) != tc.kept {
			t.Errorf("resolution %v: lock = %+v, want entry kept %v", tc.resolution, lf.Skills, tc.kept)
		}
		want := dirsBlock(env) + "skills: {}\n"
		if tc.kept {
			want = dirsBlock(env) + "skills:\n  repo: owner/repo\n"
		}
		if got := readFile(t, manifestPath()); got != want {
			t.Errorf("resolution %v: skl.yaml =\n%s\nwant\n%s", tc.resolution, got, want)
		}
	}
}

func TestRemove_EntryNeverInstalledOnlyDropsLine(t *testing.T) {
	env := newEnv(t, "", 1)
	setSkills(t, env, "skills:\n  ghost: owner/ghost\n")

	results, err := Remove(loadConfig(t), "ghost", env, skipAll)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	want := []Result{{Name: "ghost", Source: config.Source{Repo: "owner/ghost"}, Action: Removed}}
	if !slices.Equal(results, want) {
		t.Errorf("Remove() = %+v, want %+v", results, want)
	}
	if got, want := readFile(t, manifestPath()), dirsBlock(env)+"skills: {}\n"; got != want {
		t.Errorf("skl.yaml =\n%s\nwant\n%s", got, want)
	}
}

func TestRemove_ErrorsOnUnknownSkill(t *testing.T) {
	env := newEnv(t, "", 1)
	_, err := Remove(loadConfig(t), "nonexistent", env, skipAll)
	if err == nil || err.Error() != `skill "nonexistent" is not in skl.yaml or the lock file` {
		t.Fatalf("Remove() error = %v", err)
	}
}

func TestRemove_LegacyCopyGoesThroughResolver(t *testing.T) {
	for _, tc := range []struct {
		resolution Resolution
		kept       bool
	}{{Skip, true}, {Overwrite, false}} {
		env := newEnv(t, "", 1)
		lock.Save(&lock.File{Skills: []lock.Skill{{Name: "tdd", Repo: "owner/repo", Path: "tdd", Commit: "abc"}}}, env.Lock)
		copyPath := filepath.Join(env.Dirs[0], "tdd")
		writeFile(t, filepath.Join(copyPath, "SKILL.md"), "# TDD")
		r := &recorder{resolution: tc.resolution}

		if _, err := Remove(loadConfig(t), "tdd", env, r.resolve); err != nil {
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
	mustInstall(t, env, "grill-me", "owner/repo/grill-me")
	r := &recorder{resolution: Overwrite}

	if _, err := Remove(loadConfig(t), "grill-me", env, r.resolve); err != nil {
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
