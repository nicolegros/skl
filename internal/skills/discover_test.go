package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscover_FindsSkillsRecursively(t *testing.T) {
	root := t.TempDir()

	// Create skill at root level
	os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("root skill"), 0o644)

	// Create nested skills
	os.MkdirAll(filepath.Join(root, "category", "nested-skill"), 0o755)
	os.WriteFile(filepath.Join(root, "category", "nested-skill", "SKILL.md"), []byte("nested"), 0o644)

	skills, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Root SKILL.md means the root itself is a skill — should return just that
	// (same as single-skill repo behavior)
	if len(skills) != 1 {
		t.Fatalf("Discover() found %d skills, want 1 (root skill takes precedence)", len(skills))
	}
	if skills[0].Name != filepath.Base(root) {
		t.Errorf("got name %q, want %q", skills[0].Name, filepath.Base(root))
	}
}

func TestDiscover_FindsMultipleSkillsWithoutRootSkill(t *testing.T) {
	root := t.TempDir()

	os.MkdirAll(filepath.Join(root, "tdd"), 0o755)
	os.WriteFile(filepath.Join(root, "tdd", "SKILL.md"), []byte("tdd skill"), 0o644)

	os.MkdirAll(filepath.Join(root, "grill-me"), 0o755)
	os.WriteFile(filepath.Join(root, "grill-me", "SKILL.md"), []byte("grill skill"), 0o644)

	os.MkdirAll(filepath.Join(root, "deep", "nested"), 0o755)
	os.WriteFile(filepath.Join(root, "deep", "nested", "SKILL.md"), []byte("deep skill"), 0o644)

	skills, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(skills) != 3 {
		t.Fatalf("Discover() found %d skills, want 3", len(skills))
	}

	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	for _, want := range []string{"tdd", "grill-me", "nested"} {
		if !names[want] {
			t.Errorf("missing skill %q in results", want)
		}
	}
}

func TestDiscover_ErrorsOnNameCollision(t *testing.T) {
	root := t.TempDir()

	os.MkdirAll(filepath.Join(root, "a", "dupe"), 0o755)
	os.WriteFile(filepath.Join(root, "a", "dupe", "SKILL.md"), []byte("one"), 0o644)

	os.MkdirAll(filepath.Join(root, "b", "dupe"), 0o755)
	os.WriteFile(filepath.Join(root, "b", "dupe", "SKILL.md"), []byte("two"), 0o644)

	_, err := Discover(root)
	if err == nil {
		t.Fatal("Discover() should error on name collision")
	}
}
