package lock

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Skill struct {
	Name   string `json:"name"`
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Ref    string `json:"ref"`
	Pinned bool   `json:"pinned"`
}

type File struct {
	Skills []Skill `json:"skills"`
}

func (f *File) Add(s Skill) {
	for i, existing := range f.Skills {
		if existing.Name == s.Name {
			f.Skills[i] = s
			return
		}
	}
	f.Skills = append(f.Skills, s)
}

func (f *File) Remove(name string) bool {
	for i, s := range f.Skills {
		if s.Name == name {
			f.Skills = append(f.Skills[:i], f.Skills[i+1:]...)
			return true
		}
	}
	return false
}

func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &File{}, nil
	}
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func Save(f *File, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
