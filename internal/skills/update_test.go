package skills

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicolegros/skl/internal/lock"
)

func TestUpdate_RefreshesUnpinnedSkill(t *testing.T) {
	// Initial tarball (v1)
	tarballV2 := makeTarball(t, "owner-repo-def456", map[string]string{
		"SKILL.md": "# Updated Skill v2",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarballV2)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	// Pre-populate installed state
	os.MkdirAll(filepath.Join(installDir, "repo"), 0o755)
	os.WriteFile(filepath.Join(installDir, "repo", "SKILL.md"), []byte("# Old v1"), 0o644)

	lf := &lock.File{Skills: []lock.Skill{
		{Name: "repo", Repo: "owner/repo", Path: "", Ref: "abc123", Pinned: false},
	}}
	lock.Save(lf, lockPath)

	result, err := Update(UpdateOptions{
		Name:     "",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if len(result.Updated) != 1 || result.Updated[0] != "repo" {
		t.Errorf("Updated = %v, want [repo]", result.Updated)
	}

	// Verify file was updated
	data, _ := os.ReadFile(filepath.Join(installDir, "repo", "SKILL.md"))
	if !strings.Contains(string(data), "v2") {
		t.Errorf("SKILL.md not updated, got: %s", data)
	}

	// Verify lock ref updated
	loaded, _ := lock.Load(lockPath)
	if loaded.Skills[0].Ref != "def456" {
		t.Errorf("lock ref = %q, want def456", loaded.Skills[0].Ref)
	}
}

func TestUpdate_WarnsOnPinnedButStillUpdates(t *testing.T) {
	tarball := makeTarball(t, "owner-repo-new999", map[string]string{
		"SKILL.md": "# Pinned Updated",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	os.MkdirAll(filepath.Join(installDir, "repo"), 0o755)
	os.WriteFile(filepath.Join(installDir, "repo", "SKILL.md"), []byte("# Old"), 0o644)

	lf := &lock.File{Skills: []lock.Skill{
		{Name: "repo", Repo: "owner/repo", Path: "", Ref: "old123", Pinned: true},
	}}
	lock.Save(lf, lockPath)

	result, err := Update(UpdateOptions{
		Name:     "repo",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Should warn about pinned
	if len(result.Warnings) == 0 {
		t.Error("expected a warning for pinned skill")
	}

	// Should still update
	if len(result.Updated) != 1 {
		t.Errorf("Updated = %v, want [repo]", result.Updated)
	}

	// Should stay pinned with new ref
	loaded, _ := lock.Load(lockPath)
	if !loaded.Skills[0].Pinned {
		t.Error("skill should remain pinned")
	}
	if loaded.Skills[0].Ref != "new999" {
		t.Errorf("lock ref = %q, want new999", loaded.Skills[0].Ref)
	}
}

func TestUpdate_SpecificSkillOnly(t *testing.T) {
	tarball := makeTarball(t, "owner-repo-updated", map[string]string{
		"tdd/SKILL.md": "# TDD Updated",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	os.MkdirAll(filepath.Join(installDir, "tdd"), 0o755)
	os.MkdirAll(filepath.Join(installDir, "grill"), 0o755)
	os.WriteFile(filepath.Join(installDir, "tdd", "SKILL.md"), []byte("# Old"), 0o644)
	os.WriteFile(filepath.Join(installDir, "grill", "SKILL.md"), []byte("# Grill"), 0o644)

	lf := &lock.File{Skills: []lock.Skill{
		{Name: "tdd", Repo: "owner/repo", Path: "tdd", Ref: "old", Pinned: false},
		{Name: "grill", Repo: "other/repo", Path: "grill", Ref: "old", Pinned: false},
	}}
	lock.Save(lf, lockPath)

	result, err := Update(UpdateOptions{
		Name:     "tdd",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if len(result.Updated) != 1 || result.Updated[0] != "tdd" {
		t.Errorf("Updated = %v, want [tdd]", result.Updated)
	}

	// grill should be untouched
	data, _ := os.ReadFile(filepath.Join(installDir, "grill", "SKILL.md"))
	if string(data) != "# Grill" {
		t.Error("grill was modified but shouldn't have been")
	}
}
