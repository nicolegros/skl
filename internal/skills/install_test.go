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
	"strings"
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

	name, err := Install(InstallOptions{
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
	if name != "repo" {
		t.Errorf("Install() name = %q, want %q", name, "repo")
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

	name, err := Install(InstallOptions{
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
	if name != "grill-me" {
		t.Errorf("Install() name = %q, want %q", name, "grill-me")
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

func TestInstall_WithAlias_InstallsUnderAliasName(t *testing.T) {
	tarball := makeTarball(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "---\nname: grill-me\n---\n# Grill Me",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	name, err := Install(InstallOptions{
		Owner:    "owner",
		Repo:     "repo",
		Path:     "grill-me",
		Ref:      "abc123",
		Alias:    "interview-me",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if name != "interview-me" {
		t.Errorf("Install() name = %q, want %q", name, "interview-me")
	}

	// Directory should be named after alias
	if _, err := os.Stat(filepath.Join(installDir, "interview-me", "SKILL.md")); os.IsNotExist(err) {
		t.Error("interview-me/SKILL.md not found — directory should use alias name")
	}
	// Original name directory should NOT exist
	if _, err := os.Stat(filepath.Join(installDir, "grill-me")); !os.IsNotExist(err) {
		t.Error("grill-me/ should NOT exist when alias is used")
	}

	// Lock should have alias field
	lf, _ := lock.Load(lockPath)
	if len(lf.Skills) != 1 {
		t.Fatalf("lock has %d skills, want 1", len(lf.Skills))
	}
	if lf.Skills[0].Name != "grill-me" {
		t.Errorf("lock Name = %q, want %q", lf.Skills[0].Name, "grill-me")
	}
	if lf.Skills[0].Alias != "interview-me" {
		t.Errorf("lock Alias = %q, want %q", lf.Skills[0].Alias, "interview-me")
	}
}

func TestInstall_WithAlias_PatchesFrontmatterName(t *testing.T) {
	tarball := makeTarball(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "---\nname: grill-me\ndescription: Grill the user\n---\n# Grill Me\nContent here",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	_, err := Install(InstallOptions{
		Owner:    "owner",
		Repo:     "repo",
		Path:     "grill-me",
		Ref:      "abc123",
		Alias:    "interview-me",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(installDir, "interview-me", "SKILL.md"))
	if err != nil {
		t.Fatalf("reading SKILL.md: %v", err)
	}
	content := string(data)

	// Frontmatter name should be patched
	if !strings.Contains(content, "name: interview-me") {
		t.Errorf("frontmatter name not patched, got:\n%s", content)
	}
	// Original name should NOT be in frontmatter
	if strings.Contains(content, "name: grill-me") {
		t.Errorf("frontmatter still has original name, got:\n%s", content)
	}
	// Description should be unchanged
	if !strings.Contains(content, "description: Grill the user") {
		t.Errorf("description was mangled, got:\n%s", content)
	}
}

func TestInstall_WithAlias_ReplacesPathReferences(t *testing.T) {
	tarball := makeTarball(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md":   "---\nname: grill-me\n---\n# Grill Me\nSee /grill-me/helpers.sh for details",
		"grill-me/helpers.sh": "#!/bin/bash\n# Source: /grill-me/lib.sh",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	_, err := Install(InstallOptions{
		Owner:    "owner",
		Repo:     "repo",
		Path:     "grill-me",
		Ref:      "abc123",
		Alias:    "interview-me",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	// SKILL.md path reference should be replaced
	skillData, _ := os.ReadFile(filepath.Join(installDir, "interview-me", "SKILL.md"))
	if !strings.Contains(string(skillData), "/interview-me/helpers.sh") {
		t.Errorf("SKILL.md path ref not replaced, got:\n%s", skillData)
	}
	if strings.Contains(string(skillData), "/grill-me/helpers.sh") {
		t.Errorf("SKILL.md still has original path ref, got:\n%s", skillData)
	}

	// helpers.sh path reference should be replaced
	helperData, _ := os.ReadFile(filepath.Join(installDir, "interview-me", "helpers.sh"))
	if !strings.Contains(string(helperData), "/interview-me/lib.sh") {
		t.Errorf("helpers.sh path ref not replaced, got:\n%s", helperData)
	}
}

func TestInstall_WithAlias_DoesNotCorruptLongerNames(t *testing.T) {
	tarball := makeTarball(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "---\nname: grill-me\n---\n# Grill Me\nSee /grill-me-harder for the advanced version\nBut /grill-me/file.md is ours",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	_, err := Install(InstallOptions{
		Owner:    "owner",
		Repo:     "repo",
		Path:     "grill-me",
		Ref:      "abc123",
		Alias:    "interview-me",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(installDir, "interview-me", "SKILL.md"))
	content := string(data)

	// Should NOT corrupt /grill-me-harder
	if !strings.Contains(content, "/grill-me-harder") {
		t.Errorf("longer name was corrupted, got:\n%s", content)
	}
	// Should replace /grill-me/file.md
	if !strings.Contains(content, "/interview-me/file.md") {
		t.Errorf("path ref not replaced, got:\n%s", content)
	}
}

func TestInstall_WithAlias_WarnsIfNoFrontmatterName(t *testing.T) {
	tarball := makeTarball(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "# Grill Me\nNo frontmatter here",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	var warnings []string
	_, err := Install(InstallOptions{
		Owner:    "owner",
		Repo:     "repo",
		Path:     "grill-me",
		Ref:      "abc123",
		Alias:    "interview-me",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
		Logf: func(format string, a ...any) {
			warnings = append(warnings, fmt.Sprintf(format, a...))
		},
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	// Should still install successfully
	if _, err := os.Stat(filepath.Join(installDir, "interview-me", "SKILL.md")); os.IsNotExist(err) {
		t.Error("skill was not installed")
	}

	// Should have warned about missing frontmatter name
	if len(warnings) == 0 {
		t.Fatal("expected a warning about missing frontmatter name field")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "name") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings %v don't mention frontmatter name", warnings)
	}
}

func TestInstall_WithAlias_BlocksIfNameExists(t *testing.T) {
	tarball := makeTarball(t, "owner-repo-abc123", map[string]string{
		"grill-me/SKILL.md": "---\nname: grill-me\n---\n# Grill Me",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(tarball)
	}))
	defer srv.Close()

	installDir := t.TempDir()
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	// Pre-install a skill named "interview-me"
	os.MkdirAll(filepath.Join(installDir, "interview-me"), 0o755)
	os.WriteFile(filepath.Join(installDir, "interview-me", "SKILL.md"), []byte("# Existing"), 0o644)

	_, err := Install(InstallOptions{
		Owner:    "owner",
		Repo:     "repo",
		Path:     "grill-me",
		Ref:      "abc123",
		Alias:    "interview-me",
		BaseURL:  srv.URL,
		Dirs:     []string{installDir},
		LockPath: lockPath,
	})
	if err == nil {
		t.Fatal("Install() should error when alias name already exists on disk")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", err)
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

	_, err := Install(InstallOptions{
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
