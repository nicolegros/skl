package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckModifications_DetectsModifiedFile(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "grill-me")
	os.MkdirAll(skillDir, 0o755)

	// Write files that match the checksums we'll provide
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Original"), 0o644)
	os.WriteFile(filepath.Join(skillDir, "prompt.md"), []byte("Original prompt"), 0o644)

	// Compute checksums for the "original" state
	checksums, _ := computeChecksums(skillDir)

	// Now modify one file
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Modified by user"), 0o644)

	mods := CheckModifications("grill-me", []string{dir}, checksums)

	if len(mods) != 1 {
		t.Fatalf("expected 1 modification, got %d", len(mods))
	}
	if mods[0].Dir != dir {
		t.Errorf("modification dir = %q, want %q", mods[0].Dir, dir)
	}
	if len(mods[0].ModifiedFiles) != 1 || mods[0].ModifiedFiles[0] != "SKILL.md" {
		t.Errorf("modified files = %v, want [SKILL.md]", mods[0].ModifiedFiles)
	}
}

func TestCheckModifications_DetectsAddedFile(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "grill-me")
	os.MkdirAll(skillDir, 0o755)

	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Original"), 0o644)
	checksums, _ := computeChecksums(skillDir)

	// Add a new file that wasn't in the original
	os.WriteFile(filepath.Join(skillDir, "notes.md"), []byte("my notes"), 0o644)

	mods := CheckModifications("grill-me", []string{dir}, checksums)

	if len(mods) != 1 {
		t.Fatalf("expected 1 modification, got %d", len(mods))
	}
	if len(mods[0].ModifiedFiles) != 1 || mods[0].ModifiedFiles[0] != "notes.md" {
		t.Errorf("modified files = %v, want [notes.md]", mods[0].ModifiedFiles)
	}
}

func TestCheckModifications_DetectsDeletedFile(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "grill-me")
	os.MkdirAll(skillDir, 0o755)

	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Original"), 0o644)
	os.WriteFile(filepath.Join(skillDir, "prompt.md"), []byte("prompt"), 0o644)
	checksums, _ := computeChecksums(skillDir)

	// Delete one file
	os.Remove(filepath.Join(skillDir, "prompt.md"))

	mods := CheckModifications("grill-me", []string{dir}, checksums)

	if len(mods) != 1 {
		t.Fatalf("expected 1 modification, got %d", len(mods))
	}
	if len(mods[0].ModifiedFiles) != 1 || mods[0].ModifiedFiles[0] != "prompt.md" {
		t.Errorf("modified files = %v, want [prompt.md]", mods[0].ModifiedFiles)
	}
}

func TestCheckModifications_NoModifications(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "grill-me")
	os.MkdirAll(skillDir, 0o755)

	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Original"), 0o644)
	checksums, _ := computeChecksums(skillDir)

	// No changes — should return no modifications
	mods := CheckModifications("grill-me", []string{dir}, checksums)

	if len(mods) != 0 {
		t.Fatalf("expected 0 modifications, got %d: %+v", len(mods), mods)
	}
}

func TestCheckModifications_ChecksEachDirIndependently(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	skillDir1 := filepath.Join(dir1, "grill-me")
	skillDir2 := filepath.Join(dir2, "grill-me")
	os.MkdirAll(skillDir1, 0o755)
	os.MkdirAll(skillDir2, 0o755)

	os.WriteFile(filepath.Join(skillDir1, "SKILL.md"), []byte("# Original"), 0o644)
	os.WriteFile(filepath.Join(skillDir2, "SKILL.md"), []byte("# Original"), 0o644)
	checksums, _ := computeChecksums(skillDir1)

	// Modify only in dir1
	os.WriteFile(filepath.Join(skillDir1, "SKILL.md"), []byte("# Modified"), 0o644)

	mods := CheckModifications("grill-me", []string{dir1, dir2}, checksums)

	if len(mods) != 1 {
		t.Fatalf("expected 1 modification (only dir1), got %d: %+v", len(mods), mods)
	}
	if mods[0].Dir != dir1 {
		t.Errorf("modification dir = %q, want %q", mods[0].Dir, dir1)
	}
}

func TestCheckModifications_SkipsNonExistentDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	checksums := map[string]string{"SKILL.md": "abc123"}

	mods := CheckModifications("grill-me", []string{dir}, checksums)

	if len(mods) != 0 {
		t.Fatalf("expected 0 modifications for missing dir, got %d", len(mods))
	}
}

func TestBackup_CreatesBakSiblingDirectory(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "grill-me")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My modified skill"), 0o644)
	os.WriteFile(filepath.Join(skillDir, "prompt.md"), []byte("custom prompt"), 0o644)

	bakPath, err := Backup(dir, "grill-me")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	expectedBak := filepath.Join(dir, "grill-me.bak")
	if bakPath != expectedBak {
		t.Errorf("backup path = %q, want %q", bakPath, expectedBak)
	}

	// Verify backup files exist
	data, err := os.ReadFile(filepath.Join(expectedBak, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading backed up SKILL.md: %v", err)
	}
	if string(data) != "# My modified skill" {
		t.Errorf("backup SKILL.md = %q, want %q", data, "# My modified skill")
	}

	data, err = os.ReadFile(filepath.Join(expectedBak, "prompt.md"))
	if err != nil {
		t.Fatalf("reading backed up prompt.md: %v", err)
	}
	if string(data) != "custom prompt" {
		t.Errorf("backup prompt.md = %q, want %q", data, "custom prompt")
	}
}

func TestBackup_OverwritesExistingBak(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "grill-me")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Version 2"), 0o644)

	// Create a pre-existing .bak
	bakDir := filepath.Join(dir, "grill-me.bak")
	os.MkdirAll(bakDir, 0o755)
	os.WriteFile(filepath.Join(bakDir, "SKILL.md"), []byte("# Version 1 backup"), 0o644)

	_, err := Backup(dir, "grill-me")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	// Should have the new content
	data, _ := os.ReadFile(filepath.Join(bakDir, "SKILL.md"))
	if string(data) != "# Version 2" {
		t.Errorf("backup = %q, want %q", data, "# Version 2")
	}
}
