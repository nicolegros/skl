package skills

import (
	"path/filepath"
	"testing"

	"github.com/nicolegros/skl/internal/lock"
)

func TestList_ShowsAliasRelationship(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "skl.lock")

	lf := &lock.File{Skills: []lock.Skill{
		{Name: "tdd", Repo: "owner/repo", Path: "tdd", Ref: "abc123", Pinned: false},
		{Name: "grill-me", Repo: "owner/repo", Path: "grill-me", Ref: "def456", Alias: "interview-me"},
	}}
	lock.Save(lf, lockPath)

	entries, err := List(ListOptions{LockPath: lockPath})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("List() returned %d entries, want 2", len(entries))
	}

	// First entry: no alias
	if entries[0].DisplayName != "tdd" {
		t.Errorf("entries[0].DisplayName = %q, want %q", entries[0].DisplayName, "tdd")
	}
	if entries[0].Alias != "" {
		t.Errorf("entries[0].Alias = %q, want empty", entries[0].Alias)
	}

	// Second entry: has alias, should show relationship
	if entries[1].DisplayName != "interview-me (grill-me @ owner/repo)" {
		t.Errorf("entries[1].DisplayName = %q, want %q", entries[1].DisplayName, "interview-me (grill-me @ owner/repo)")
	}
	if entries[1].Alias != "interview-me" {
		t.Errorf("entries[1].Alias = %q, want %q", entries[1].Alias, "interview-me")
	}
	if entries[1].Source != "owner/repo" {
		t.Errorf("entries[1].Source = %q, want %q", entries[1].Source, "owner/repo")
	}
}
