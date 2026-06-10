package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nicolaslegros/skills/internal/lock"
)

func TestRemove_DeletesFromDirsAndLock(t *testing.T) {
	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skills-lock.json")

	// Pre-populate
	os.MkdirAll(filepath.Join(installDir, "tdd"), 0o755)
	os.WriteFile(filepath.Join(installDir, "tdd", "SKILL.md"), []byte("# TDD"), 0o644)

	lf := &lock.File{Skills: []lock.Skill{
		{Name: "tdd", Repo: "owner/repo", Path: "tdd", Ref: "abc", Pinned: false},
	}}
	lock.Save(lf, lockPath)

	err := Remove(RemoveOptions{
		Name:     "tdd",
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Directory should be gone
	if _, err := os.Stat(filepath.Join(installDir, "tdd")); !os.IsNotExist(err) {
		t.Error("tdd directory still exists")
	}

	// Lock should be empty
	loaded, _ := lock.Load(lockPath)
	if len(loaded.Skills) != 0 {
		t.Errorf("lock still has %d skills", len(loaded.Skills))
	}
}

func TestRemove_ErrorsOnUnknownSkill(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "skills-lock.json")
	lock.Save(&lock.File{}, lockPath)

	err := Remove(RemoveOptions{
		Name:     "nonexistent",
		Dirs:     []string{t.TempDir()},
		LockPath: lockPath,
	})
	if err == nil {
		t.Fatal("Remove() should error for unknown skill")
	}
}
