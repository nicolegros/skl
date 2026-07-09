package skills

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nicolegros/skl/internal/lock"
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

	// Find the skill to determine the on-disk directory name
	dirName := opts.Name
	for _, s := range lf.Skills {
		if s.Name == opts.Name || s.Alias == opts.Name {
			if s.Alias != "" {
				dirName = s.Alias
			}
			break
		}
	}

	if !lf.Remove(opts.Name) {
		return fmt.Errorf("skill %q not found in lock file", opts.Name)
	}

	for _, dir := range opts.Dirs {
		os.RemoveAll(filepath.Join(dir, dirName))
	}

	return lock.Save(lf, opts.LockPath)
}
