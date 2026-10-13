package skills

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nicolegros/skl/internal/lock"
)

func TestList_ShowsAliasRelationship(t *testing.T) {
	env := newEnv(t, "", 1)
	lock.Save(&lock.File{Skills: []lock.Skill{
		{Name: "tdd", Repo: "owner/repo", Path: "tdd", Ref: "abc123"},
		{Name: "grill-me", Repo: "owner/repo", Path: "grill-me", Ref: "def456", Alias: "interview-me"},
	}}, env.Lock)

	entries, err := List(env.Paths)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("List() returned %d entries, want 2", len(entries))
	}
	if entries[0].DisplayName != "tdd" || entries[0].Alias != "" {
		t.Errorf("entries[0] = %+v", entries[0])
	}
	if entries[1].DisplayName != "interview-me (grill-me @ owner/repo)" || entries[1].Alias != "interview-me" || entries[1].Source != "owner/repo" {
		t.Errorf("entries[1] = %+v", entries[1])
	}
}

func TestList_ReportsUnlinkedDirs(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Skill"})
	env := newEnv(t, url, 2)
	if _, err := Install(InstallOptions{Owner: "owner", Repo: "repo"}, env, skipAll); err != nil {
		t.Fatal(err)
	}

	entries, err := List(env.Paths)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].DisplayName != "repo" || entries[0].Unlinked != nil {
		t.Fatalf("fully linked List() = %+v, want repo with no unlinked dirs", entries)
	}

	os.Remove(filepath.Join(env.Dirs[1], "repo"))
	entries, _ = List(env.Paths)
	if !slices.Equal(entries[0].Unlinked, []string{env.Dirs[1]}) {
		t.Errorf("Unlinked = %v, want [%s]", entries[0].Unlinked, env.Dirs[1])
	}
}
