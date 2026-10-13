package skills

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestList_ReportsUnlinkedDirs(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Skill"})
	env := newEnv(t, url, 2)
	mustInstall(t, env, "repo", "owner/repo")

	entries, err := List(loadConfig(t), env.Paths)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "repo" || entries[0].Commit != "abc123" || entries[0].Unlinked != nil {
		t.Fatalf("fully linked List() = %+v, want repo with no unlinked dirs", entries)
	}

	os.Remove(filepath.Join(env.Dirs[1], "repo"))
	entries, _ = List(loadConfig(t), env.Paths)
	if !slices.Equal(entries[0].Unlinked, []string{env.Dirs[1]}) {
		t.Errorf("Unlinked = %v, want [%s]", entries[0].Unlinked, env.Dirs[1])
	}
}
