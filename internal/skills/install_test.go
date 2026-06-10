package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nicolegros/skl/internal/lock"
)

// makeTarball creates a .tar.gz in memory with the given files.
// GitHub tarballs have a top-level prefix dir like "owner-repo-sha/".
func makeTarball(t *testing.T, prefix string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		path := prefix + "/" + name
		tw.WriteHeader(&tar.Header{Name: path, Size: int64(len(content)), Mode: 0o644, Typeflag: tar.TypeReg})
		tw.Write([]byte(content))
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func TestInstall_SingleSkillRepo(t *testing.T) {
	tarball := makeTarball(t, "owner-repo-abc123", map[string]string{
		"SKILL.md":   "# My Skill",
		"helpers.sh": "#!/bin/bash",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	err := Install(InstallOptions{
		Owner:    "owner",
		Repo:     "repo",
		Path:     "",
		Ref:      "abc123",
		Pinned:   false,
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	// Verify skill was copied
	skillMd := filepath.Join(installDir, "repo", "SKILL.md")
	if _, err := os.Stat(skillMd); os.IsNotExist(err) {
		t.Error("SKILL.md not found in install directory")
	}

	// Verify helpers.sh was also copied
	helpers := filepath.Join(installDir, "repo", "helpers.sh")
	if _, err := os.Stat(helpers); os.IsNotExist(err) {
		t.Error("helpers.sh not found in install directory")
	}

	// Verify lock file updated
	lf, _ := lock.Load(lockPath)
	if len(lf.Skills) != 1 {
		t.Fatalf("lock has %d skills, want 1", len(lf.Skills))
	}
	if lf.Skills[0].Name != "repo" || lf.Skills[0].Ref != "abc123" {
		t.Errorf("lock entry = %+v", lf.Skills[0])
	}
}

func TestInstall_SubdirectorySkill(t *testing.T) {
	tarball := makeTarball(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "# Grill Me",
		"tdd/SKILL.md":      "# TDD",
		"README.md":         "# Repo",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	err := Install(InstallOptions{
		Owner:    "owner",
		Repo:     "repo",
		Path:     "grill-me",
		Ref:      "abc123",
		Pinned:   false,
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	// Only grill-me should be installed
	if _, err := os.Stat(filepath.Join(installDir, "grill-me", "SKILL.md")); os.IsNotExist(err) {
		t.Error("grill-me/SKILL.md not installed")
	}
	if _, err := os.Stat(filepath.Join(installDir, "tdd")); !os.IsNotExist(err) {
		t.Error("tdd should NOT be installed")
	}

	lf, _ := lock.Load(lockPath)
	if lf.Skills[0].Name != "grill-me" || lf.Skills[0].Path != "grill-me" {
		t.Errorf("lock entry = %+v", lf.Skills[0])
	}
}

func TestInstallFromLock_InstallsMissingSkills(t *testing.T) {
	tarball := makeTarball(t, "owner-myskill-abc123", map[string]string{
		"SKILL.md": "# My Skill",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	// Pre-populate lock file with one skill
	lf := &lock.File{Skills: []lock.Skill{
		{Name: "myskill", Repo: "owner/myskill", Ref: "abc123"},
	}}
	lock.Save(lf, lockPath)

	var logs []string
	logf := func(format string, a ...any) {
		logs = append(logs, fmt.Sprintf(format, a...))
	}

	err := InstallFromLock(lockPath, srv.URL, "", []string{installDir}, logf)
	if err != nil {
		t.Fatalf("InstallFromLock() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(installDir, "myskill", "SKILL.md")); os.IsNotExist(err) {
		t.Error("myskill/SKILL.md not installed")
	}

	if len(logs) != 1 {
		t.Errorf("expected 1 log message, got %d: %v", len(logs), logs)
	}
	if logs[0] != "Installing myskill from owner/myskill@abc123" {
		t.Errorf("unexpected log: %s", logs[0])
	}
}

func TestInstallFromLock_SkipsAlreadyInstalled(t *testing.T) {
	tarball := makeTarball(t, "owner-repo-abc123", map[string]string{
		"SKILL.md": "# My Skill",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	// Pre-install one skill
	os.MkdirAll(filepath.Join(installDir, "existing"), 0o755)
	os.WriteFile(filepath.Join(installDir, "existing", "SKILL.md"), []byte("# Existing"), 0o644)

	lf := &lock.File{Skills: []lock.Skill{
		{Name: "existing", Repo: "owner/repo", Ref: "abc123"},
		{Name: "missing", Repo: "owner/repo", Ref: "abc123"},
	}}
	lock.Save(lf, lockPath)

	var logs []string
	logf := func(format string, a ...any) {
		logs = append(logs, fmt.Sprintf(format, a...))
	}

	err := InstallFromLock(lockPath, srv.URL, "", []string{installDir}, logf)
	if err != nil {
		t.Fatalf("InstallFromLock() error = %v", err)
	}

	// Should have skipped existing and installed missing
	if len(logs) != 2 {
		t.Fatalf("expected 2 log messages, got %d: %v", len(logs), logs)
	}
	if logs[0] != "Skipping existing (already installed)" {
		t.Errorf("unexpected log[0]: %s", logs[0])
	}
	if logs[1] != "Installing missing from owner/repo@abc123" {
		t.Errorf("unexpected log[1]: %s", logs[1])
	}
}

func TestInstallFromLock_ErrorsOnEmptyLock(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	err := InstallFromLock(lockPath, "", "", nil, func(string, ...any) {})
	if err == nil {
		t.Fatal("expected error for empty lock file")
	}
}

func TestInstall_FailsWithoutSkillMd(t *testing.T) {
	tarball := makeTarball(t, "owner-repo-abc123", map[string]string{
		"README.md": "# Not a skill",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	err := Install(InstallOptions{
		Owner:    "owner",
		Repo:     "repo",
		Path:     "",
		Ref:      "abc123",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err == nil {
		t.Fatal("Install() should error when SKILL.md is missing")
	}
}
