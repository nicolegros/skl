package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nicolegros/skl/internal/lock"
)

func TestList_MarksUnlinkedDirs(t *testing.T) {
	cfg, data, agents := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", data)
	claude, pi := filepath.Join(agents, "claude"), filepath.Join(agents, "pi")
	os.MkdirAll(filepath.Join(cfg, "skl"), 0o755)
	os.WriteFile(filepath.Join(cfg, "skl", "config.yaml"), []byte("directories:\n  - "+claude+"\n  - "+pi+"\n"), 0o644)
	lock.Save(&lock.File{Skills: []lock.Skill{
		{Name: "tdd", Repo: "owner/repo", Commit: "abc1234567"},
		{Name: "grill", Repo: "owner/repo", Commit: "def1234567"},
	}}, filepath.Join(cfg, "skl", "skl.lock"))
	for _, name := range []string{"tdd", "grill"} {
		os.MkdirAll(filepath.Join(data, "skl", "skills", name), 0o755)
		os.MkdirAll(claude, 0o755)
		os.Symlink(filepath.Join(data, "skl", "skills", name), filepath.Join(claude, name))
	}
	os.MkdirAll(pi, 0o755)
	os.Symlink(filepath.Join(data, "skl", "skills", "tdd"), filepath.Join(pi, "tdd"))

	root := NewRoot("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list error = %v", err)
	}

	want := "tdd     owner/repo@abc1234\ngrill   owner/repo@def1234 (unlinked: " + pi + ")\n"
	if out.String() != want {
		t.Errorf("list output =\n%s\nwant\n%s", out.String(), want)
	}
}
