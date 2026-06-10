package skills

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nicolaslegros/skills/internal/lock"
)

type RemoveOptions struct {
	Name     string
	Dirs     []string
	LockPath string
}

// Remove deletes a skill from all configured directories and the lock file.
func Remove(opts RemoveOptions) error {
	lf, err := lock.Load(opts.LockPath)
	if err != nil {
		return err
	}

	if !lf.Remove(opts.Name) {
		return fmt.Errorf("skill %q not found in lock file", opts.Name)
	}

	for _, dir := range opts.Dirs {
		os.RemoveAll(filepath.Join(dir, opts.Name))
	}

	return lock.Save(lf, opts.LockPath)
}
