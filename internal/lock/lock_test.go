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
		{Name: "grill-me", Repo: "owner/repo", Path: "grill-me", Ref: "abc123", Pinned: false},
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
	if s.Name != "grill-me" || s.Repo != "owner/repo" || s.Ref != "abc123" || s.Pinned != false {
		t.Errorf("got %+v", s)
	}

	// Verify file exists on disk
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("lock file was not written to disk")
	}
}

func TestFile_Add(t *testing.T) {
	f := &File{}

	f.Add(Skill{Name: "tdd", Repo: "owner/repo", Path: "tdd", Ref: "abc", Pinned: false})

	if len(f.Skills) != 1 || f.Skills[0].Name != "tdd" {
		t.Errorf("Add() skills = %v", f.Skills)
	}

	// Adding same name replaces
	f.Add(Skill{Name: "tdd", Repo: "owner/repo", Path: "tdd", Ref: "def", Pinned: true})

	if len(f.Skills) != 1 || f.Skills[0].Ref != "def" || !f.Skills[0].Pinned {
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
