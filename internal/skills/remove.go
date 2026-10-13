package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/nicolegros/skl/internal/lock"
)

// Remove deletes a skill's links, its store copy, and its lock entry.
// Foreign symlinks are left alone; a real directory goes through resolve.
func Remove(name string, env Env, resolve Resolver) error {
	lf, err := lock.Load(env.Lock)
	if err != nil {
		return err
	}
	entry, ok := lf.Get(name)
	if !ok {
		return fmt.Errorf("skill %q not found in lock file", name)
	}
	if err := reconcile(lf, env, resolve, name); err != nil {
		return errors.Join(err, lock.Save(lf, env.Lock))
	}

	storePath := filepath.Join(env.Store, name)
	var conflicts []Conflict
	for _, dir := range entry.Links {
		if slices.Contains(env.Dirs, dir) {
			continue
		}
		if state, _ := linkState(dir, name, storePath); state == stateLinked {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				return err
			}
		}
	}
	for _, dir := range env.Dirs {
		path := filepath.Join(dir, name)
		switch state, _ := linkState(dir, name, storePath); state {
		case stateLinked:
			if err := os.Remove(path); err != nil {
				return err
			}
		case stateLocalDir:
			local, err := computeChecksums(path)
			if err != nil {
				return err
			}
			conflicts = append(conflicts, Conflict{Path: path, Kind: LocalDir, Files: diffChecksums(entry.Files, local)})
		}
	}

	if len(conflicts) > 0 {
		resolution := resolve(name, conflicts)
		now := time.Now()
		for _, c := range conflicts {
			if resolution == Skip {
				break
			}
			if resolution == BackupAndOverwrite {
				dst, err := backup(c, env.Backups, name, now)
				if err != nil {
					return fmt.Errorf("backing up %s: %w", c.Path, err)
				}
				env.logf("Backed up %s to %s", c.Path, dst)
			}
			if err := os.RemoveAll(c.Path); err != nil {
				return err
			}
		}
	}

	if err := os.RemoveAll(storePath); err != nil {
		return err
	}
	lf.Remove(name)
	return lock.Save(lf, env.Lock)
}
