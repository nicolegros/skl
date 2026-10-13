package lock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ReturnsEmptyWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skl.lock")

	lf, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(lf.Skills) != 0 {
		t.Errorf("Load() skills = %v, want empty", lf.Skills)
	}
}

func TestSave_WritesAndLoadsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skl.lock")

	lf := &File{Skills: []Skill{
		{Name: "grill-me", Repo: "owner/repo", Path: "grill-me", Ref: "v1", Commit: "abc123"},
	}}

	if err := Save(lf, path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(loaded.Skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(loaded.Skills))
	}
	s := loaded.Skills[0]
	if s.Name != "grill-me" || s.Repo != "owner/repo" || s.Ref != "v1" || s.Commit != "abc123" {
		t.Errorf("got %+v", s)
	}

	// Verify file exists on disk
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("lock file was not written to disk")
	}
}

func TestFile_Add(t *testing.T) {
	f := &File{}

	f.Add(Skill{Name: "tdd", Repo: "owner/repo", Path: "tdd", Commit: "abc"})

	if len(f.Skills) != 1 || f.Skills[0].Name != "tdd" {
		t.Errorf("Add() skills = %v", f.Skills)
	}

	// Adding same name replaces
	f.Add(Skill{Name: "tdd", Repo: "owner/repo", Path: "tdd", Commit: "def"})

	if len(f.Skills) != 1 || f.Skills[0].Commit != "def" {
		t.Errorf("Add() did not replace: %+v", f.Skills[0])
	}
}

func TestFile_Remove(t *testing.T) {
	f := &File{Skills: []Skill{
		{Name: "tdd", Repo: "owner/repo", Path: "tdd", Ref: "abc"},
		{Name: "grill", Repo: "owner/repo", Path: "grill", Ref: "def"},
	}}

	ok := f.Remove("tdd")
	if !ok {
		t.Error("Remove() returned false for existing skill")
	}
	if len(f.Skills) != 1 || f.Skills[0].Name != "grill" {
		t.Errorf("Remove() skills = %v", f.Skills)
	}

	ok = f.Remove("nonexistent")
	if ok {
		t.Error("Remove() returned true for missing skill")
	}
}

func TestFile_Add_KeysByInstalledName(t *testing.T) {
	f := &File{}

	f.Add(Skill{Name: "grill-me", Repo: "owner/repo", Alias: "interview-me", Ref: "abc"})
	f.Add(Skill{Name: "grill-me", Repo: "owner/repo", Alias: "quiz-me", Ref: "abc"})
	f.Add(Skill{Name: "grill-me", Repo: "owner/repo", Alias: "quiz-me", Ref: "def"})

	if len(f.Skills) != 2 {
		t.Fatalf("Add() skills = %+v, want 2 entries", f.Skills)
	}
	got, ok := f.Get("quiz-me")
	if !ok || got.Ref != "def" {
		t.Errorf("Get(quiz-me) = %+v, %v; want ref def", got, ok)
	}
	if _, ok := f.Get("grill-me"); ok {
		t.Error("Get(grill-me) found an entry, but both are installed under aliases")
	}
}

func TestFile_Remove_ByInstalledName(t *testing.T) {
	f := &File{Skills: []Skill{
		{Name: "grill-me", Alias: "interview-me"},
		{Name: "grill-me", Alias: "quiz-me"},
	}}

	if !f.Remove("interview-me") {
		t.Fatal("Remove(interview-me) returned false")
	}
	if len(f.Skills) != 1 || f.Skills[0].Alias != "quiz-me" {
		t.Errorf("Remove() skills = %+v, want only quiz-me", f.Skills)
	}
}

func TestSave_DistinguishesEmptyLinksFromLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skl.lock")
	Save(&File{Skills: []Skill{
		{Name: "migrated", Links: []string{}},
		{Name: "linked", Links: []string{"/a/skills"}},
		{Name: "legacy"},
	}}, path)

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if l := loaded.Skills[0].Links; l == nil || len(l) != 0 {
		t.Errorf("migrated Links = %#v, want empty non-nil", l)
	}
	if l := loaded.Skills[1].Links; len(l) != 1 || l[0] != "/a/skills" {
		t.Errorf("linked Links = %#v, want [/a/skills]", l)
	}
	if l := loaded.Skills[2].Links; l != nil {
		t.Errorf("legacy Links = %#v, want nil", l)
	}
}

func TestLoad_ReadsLegacyPinnedField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skl.lock")
	os.WriteFile(path, []byte(`{"skills": [
  {"name": "tdd", "repo": "owner/repo", "path": "tdd", "ref": "abc1234", "pinned": false},
  {"name": "grill", "repo": "owner/repo", "path": "grill", "ref": "def5678", "pinned": true}
]}`), 0o644)

	lf, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := lf.Skills[0]; got.Ref != "" || got.Commit != "abc1234" {
		t.Errorf("unpinned entry = %+v, want default branch at abc1234", got)
	}
	if got := lf.Skills[1]; got.Ref != "def5678" || got.Commit != "def5678" {
		t.Errorf("pinned entry = %+v, want declared and resolved def5678", got)
	}
}
