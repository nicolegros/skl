package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nicolegros/skl/internal/lock"
)

func TestList_ShowsManifestRowsWithDriftFlags(t *testing.T) {
	cfg, data, agents := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", data)
	claude, pi := filepath.Join(agents, "claude"), filepath.Join(agents, "pi")
	os.MkdirAll(filepath.Join(cfg, "skl"), 0o755)
	os.WriteFile(filepath.Join(cfg, "skl", "skl.yaml"), []byte(`directories:
  - `+claude+`
  - `+pi+`
skills:
  grill: owner/repo/grill@v2
  new: owner/new
  pinned: owner/repo/pinned@v1.3.1
  tdd: owner/repo/tdd
`), 0o644)
	lock.Save(&lock.File{Skills: []lock.Skill{
		{Name: "tdd", Repo: "owner/repo", Path: "tdd", Commit: "abc1234567"},
		{Name: "grill", Repo: "owner/repo", Path: "grill", Commit: "def1234567"},
		{Name: "old", Repo: "owner/old", Ref: "v1", Commit: "0009999999"},
		{Name: "pinned", Repo: "owner/repo", Path: "pinned", Ref: "v1.3.1", Commit: "v1.3.1"},
	}}, filepath.Join(cfg, "skl", "skl.lock"))
	for _, name := range []string{"tdd", "grill", "old", "pinned"} {
		for _, dir := range []string{claude, pi} {
			if name == "tdd" && dir == pi {
				continue
			}
			os.MkdirAll(dir, 0o755)
			os.Symlink(filepath.Join(data, "skl", "skills", name), filepath.Join(dir, name))
		}
	}

	root := NewRoot("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("list error = %v", err)
	}

	want := `grill    owner/repo/grill@v2 def1234 (changed)
new      owner/new (not installed)
old      owner/old@v1 0009999 (not in skl.yaml, removed on next install)
pinned   owner/repo/pinned@v1.3.1
tdd      owner/repo/tdd abc1234 (unlinked: ` + pi + `)
`
	if out.String() != want {
		t.Errorf("list output =\n%s\nwant\n%s", out.String(), want)
	}
}
