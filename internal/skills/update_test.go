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

func TestUpdate_ByAlias_FetchesAndRepatches(t *testing.T) {
	tarballV2 := makeTarball(t, "owner-repo-def456", map[string]string{
		"grill-me/SKILL.md":   "---\nname: grill-me\n---\n# Grill Me v2\nSee /grill-me/helpers.sh",
		"grill-me/helpers.sh": "#!/bin/bash\n# v2",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarballV2)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	// Pre-populate: installed as "interview-me" via alias
	os.MkdirAll(filepath.Join(installDir, "interview-me"), 0o755)
	os.WriteFile(filepath.Join(installDir, "interview-me", "SKILL.md"), []byte("# old"), 0o644)

	lf := &lock.File{Skills: []lock.Skill{
		{Name: "grill-me", Repo: "owner/repo", Path: "grill-me", Ref: "abc123", Alias: "interview-me"},
	}}
	lock.Save(lf, lockPath)

	result, err := Update(UpdateOptions{
		Name:     "interview-me",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if len(result.Updated) != 1 {
		t.Fatalf("Updated = %v, want 1 entry", result.Updated)
	}

	// Should be installed under alias directory
	data, err := os.ReadFile(filepath.Join(installDir, "interview-me", "SKILL.md"))
	if err != nil {
		t.Fatalf("reading SKILL.md: %v", err)
	}
	content := string(data)

	// Should contain v2 content
	if !strings.Contains(content, "v2") {
		t.Errorf("SKILL.md not updated to v2, got:\n%s", content)
	}
	// Frontmatter name should be patched to alias
	if !strings.Contains(content, "name: interview-me") {
		t.Errorf("frontmatter name not patched, got:\n%s", content)
	}
	// Path refs should be patched
	if !strings.Contains(content, "/interview-me/helpers.sh") {
		t.Errorf("path ref not patched, got:\n%s", content)
	}

	// Lock should retain alias and update ref
	loaded, _ := lock.Load(lockPath)
	if loaded.Skills[0].Alias != "interview-me" {
		t.Errorf("lock alias = %q, want interview-me", loaded.Skills[0].Alias)
	}
	if loaded.Skills[0].Ref != "def456" {
		t.Errorf("lock ref = %q, want def456", loaded.Skills[0].Ref)
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

func TestUpdate_DetectsModificationsAndDoesNotOverwrite(t *testing.T) {
	tarballV2 := makeTarball(t, "owner-repo-def456", map[string]string{
		"SKILL.md": "# Updated Skill v2",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarballV2)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	// Pre-install: write the original file and store its checksum in lock
	os.MkdirAll(filepath.Join(installDir, "repo"), 0o755)
	originalContent := []byte("# Original v1")
	os.WriteFile(filepath.Join(installDir, "repo", "SKILL.md"), originalContent, 0o644)
	checksums, _ := computeChecksums(filepath.Join(installDir, "repo"))

	lf := &lock.File{Skills: []lock.Skill{
		{Name: "repo", Repo: "owner/repo", Path: "", Ref: "abc123", Pinned: false, Files: checksums},
	}}
	lock.Save(lf, lockPath)

	// User modifies the file locally
	os.WriteFile(filepath.Join(installDir, "repo", "SKILL.md"), []byte("# User's custom version"), 0o644)

	result, err := Update(UpdateOptions{
		Name:     "",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
		Force:    false,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Should NOT have updated
	if len(result.Updated) != 0 {
		t.Errorf("expected no updates, got %v", result.Updated)
	}

	// Should report modifications
	if len(result.Modifications) != 1 {
		t.Fatalf("expected 1 skill with modifications, got %d", len(result.Modifications))
	}
	if result.Modifications[0].SkillName != "repo" {
		t.Errorf("modification skill = %q, want %q", result.Modifications[0].SkillName, "repo")
	}
	if len(result.Modifications[0].Dirs) != 1 {
		t.Fatalf("expected 1 dir modification, got %d", len(result.Modifications[0].Dirs))
	}

	// File should still be the user's version
	data, _ := os.ReadFile(filepath.Join(installDir, "repo", "SKILL.md"))
	if string(data) != "# User's custom version" {
		t.Errorf("file was overwritten! got: %s", data)
	}
}

func TestUpdate_ForceTrueOverwritesModifications(t *testing.T) {
	tarballV2 := makeTarball(t, "owner-repo-def456", map[string]string{
		"SKILL.md": "# Updated Skill v2",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarballV2)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	// Pre-install with checksums
	os.MkdirAll(filepath.Join(installDir, "repo"), 0o755)
	os.WriteFile(filepath.Join(installDir, "repo", "SKILL.md"), []byte("# Original v1"), 0o644)
	checksums, _ := computeChecksums(filepath.Join(installDir, "repo"))

	lf := &lock.File{Skills: []lock.Skill{
		{Name: "repo", Repo: "owner/repo", Path: "", Ref: "abc123", Pinned: false, Files: checksums},
	}}
	lock.Save(lf, lockPath)

	// User modifies locally
	os.WriteFile(filepath.Join(installDir, "repo", "SKILL.md"), []byte("# User's custom"), 0o644)

	result, err := Update(UpdateOptions{
		Name:     "",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
		Force:    true,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Should have updated despite modifications
	if len(result.Updated) != 1 || result.Updated[0] != "repo" {
		t.Errorf("Updated = %v, want [repo]", result.Updated)
	}

	// File should be the new version
	data, _ := os.ReadFile(filepath.Join(installDir, "repo", "SKILL.md"))
	if string(data) != "# Updated Skill v2" {
		t.Errorf("file not updated, got: %s", data)
	}
}
