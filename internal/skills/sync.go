package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"github.com/nicolegros/skl/internal/lock"
)

// Paths locates everything skl reads and writes on disk.
type Paths struct {
	Store   string   // one directory per installed skill
	Backups string   // copies made before overwriting
	Dirs    []string // configured agent directories, expanded
	Lock    string
}

// Env is the context shared by every operation.
type Env struct {
	Paths
	BaseURL string // GitHub API base
	Token   string
	Logf    func(string, ...any)
}

func (e Env) logf(format string, a ...any) {
	if e.Logf != nil {
		e.Logf(format, a...)
	}
}

// LinkState is what sits at <dir>/<name> in an agent directory.
type LinkState int

const (
	stateMissing LinkState = iota
	stateLinked
	stateForeignLink
	stateLocalDir
)

func linkState(dir, name, storePath string) (LinkState, string) {
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil {
		return stateMissing, ""
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return stateLocalDir, ""
	}
	target, _ := os.Readlink(path)
	if target == storePath {
		return stateLinked, target
	}
	return stateForeignLink, target
}

type ConflictKind int

const (
	StoreEdited ConflictKind = iota // store contents differ from the lock checksums
	ForeignLink                     // a symlink pointing somewhere else
	LocalDir                        // a real directory with content differing from the store
)

// Conflict is something skl would destroy to apply a skill.
type Conflict struct {
	Path   string // store skill dir, or <dir>/<name>
	Kind   ConflictKind
	Files  []string // changed, added, or deleted relative paths (StoreEdited, LocalDir)
	Target string   // ForeignLink target
}

type Resolution int

const (
	Skip Resolution = iota
	Overwrite
	BackupAndOverwrite
)

// Resolver decides once for all conflicts of a skill.
type Resolver func(skill string, conflicts []Conflict) Resolution

func skipAll(string, []Conflict) Resolution { return Skip }

// fetched is a freshly downloaded skill directory and the ref it came from.
type fetched struct {
	dir    string
	ref    string
	pinned bool
}

// sync writes src (if non-nil) into the store, then links every configured dir.
// Conflicts are gathered first, resolved once, then applied.
// Returns the updated entry and whether the store was written.
func sync(entry lock.Skill, src *fetched, env Env, resolve Resolver) (lock.Skill, bool, error) {
	name := entry.InstalledName()
	storePath := filepath.Join(env.Store, name)
	var conflicts []Conflict
	storeConflict := false

	var staged string
	var reference map[string]string
	if src != nil {
		var err error
		staged, err = stage(entry, src.dir, env)
		if err != nil {
			return entry, false, err
		}
		defer os.RemoveAll(staged)
		if reference, err = computeChecksums(staged); err != nil {
			return entry, false, err
		}
		if exists(storePath) {
			current, err := computeChecksums(storePath)
			if err != nil {
				return entry, false, err
			}
			if changed := diffChecksums(entry.Files, current); len(changed) > 0 {
				conflicts = append(conflicts, Conflict{Path: storePath, Kind: StoreEdited, Files: changed})
				storeConflict = true
			}
		}
	} else {
		if !exists(storePath) {
			return entry, false, fmt.Errorf("%s is missing from the store", name)
		}
		var err error
		if reference, err = computeChecksums(storePath); err != nil {
			return entry, false, err
		}
	}

	type dirPlan struct {
		dir      string
		state    LinkState
		conflict bool
	}
	var plans []dirPlan
	for _, dir := range env.Dirs {
		state, target := linkState(dir, name, storePath)
		path := filepath.Join(dir, name)
		switch state {
		case stateMissing:
			plans = append(plans, dirPlan{dir: dir, state: state})
		case stateForeignLink:
			conflicts = append(conflicts, Conflict{Path: path, Kind: ForeignLink, Target: target})
			plans = append(plans, dirPlan{dir: dir, state: state, conflict: true})
		case stateLocalDir:
			local, err := computeChecksums(path)
			if err != nil {
				return entry, false, err
			}
			changed := diffChecksums(reference, local)
			if len(changed) > 0 {
				conflicts = append(conflicts, Conflict{Path: path, Kind: LocalDir, Files: changed})
			}
			plans = append(plans, dirPlan{dir: dir, state: state, conflict: len(changed) > 0})
		}
	}

	resolution := Skip
	if len(conflicts) > 0 {
		resolution = resolve(name, conflicts)
	}
	overwrite := resolution != Skip

	if resolution == BackupAndOverwrite {
		now := time.Now()
		for _, c := range conflicts {
			dst, err := backup(c, env.Backups, name, now)
			if err != nil {
				return entry, false, fmt.Errorf("backing up %s: %w", c.Path, err)
			}
			if dst == "" {
				env.logf("Nothing to back up for %s (dangling symlink)", c.Path)
			} else {
				env.logf("Backed up %s to %s", c.Path, dst)
			}
		}
	}

	wrote := false
	if src != nil && (!storeConflict || overwrite) {
		if err := replaceDir(staged, storePath); err != nil {
			return entry, false, fmt.Errorf("writing store: %w", err)
		}
		entry.Ref = src.ref
		entry.Pinned = src.pinned
		entry.Files = reference
		wrote = true
	}

	for _, p := range plans {
		if p.conflict && !overwrite {
			continue
		}
		path := filepath.Join(p.dir, name)
		if p.state != stateMissing {
			if err := os.RemoveAll(path); err != nil {
				return entry, wrote, err
			}
		}
		if err := os.MkdirAll(p.dir, 0o755); err != nil {
			return entry, wrote, err
		}
		if err := os.Symlink(storePath, path); err != nil {
			return entry, wrote, err
		}
	}

	for _, dir := range entry.Links {
		if slices.Contains(env.Dirs, dir) {
			continue
		}
		if state, _ := linkState(dir, name, storePath); state == stateLinked {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				return entry, wrote, err
			}
		}
	}

	entry.Links = []string{}
	for _, dir := range env.Dirs {
		if state, _ := linkState(dir, name, storePath); state == stateLinked {
			entry.Links = append(entry.Links, dir)
		}
	}
	return entry, wrote, nil
}

// stage copies src next to the store entry and applies --as patching to the copy.
func stage(entry lock.Skill, src string, env Env) (string, error) {
	name := entry.InstalledName()
	if err := os.MkdirAll(env.Store, 0o755); err != nil {
		return "", err
	}
	staged, err := os.MkdirTemp(env.Store, "."+name+"-*")
	if err != nil {
		return "", err
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		os.RemoveAll(staged)
		return "", err
	}
	if err := copyDir(src, staged); err != nil {
		os.RemoveAll(staged)
		return "", err
	}
	if entry.Alias != "" {
		if !patchFrontmatterName(staged, entry.Name, entry.Alias) {
			env.logf("warning: SKILL.md has no frontmatter name field to patch")
		}
		replacePathRefs(staged, entry.Name, entry.Alias)
	}
	return staged, nil
}

// replaceDir swaps staged into dst with renames so links never see a half-written dir.
func replaceDir(staged, dst string) error {
	if !exists(dst) {
		return os.Rename(staged, dst)
	}
	old, err := os.MkdirTemp(filepath.Dir(dst), "."+filepath.Base(dst)+"-old-*")
	if err != nil {
		return err
	}
	if err := os.Remove(old); err != nil {
		return err
	}
	if err := os.Rename(dst, old); err != nil {
		return err
	}
	if err := os.Rename(staged, dst); err != nil {
		_ = os.Rename(old, dst)
		return err
	}
	return os.RemoveAll(old)
}

// backup copies a conflict's contents to <root>/<name>/<timestamp>/<conflict path>.
// Returns "" when there is nothing to copy (dangling symlink).
func backup(c Conflict, root, name string, now time.Time) (string, error) {
	src := c.Path
	if c.Kind == ForeignLink {
		resolved, err := filepath.EvalSymlinks(c.Path)
		if os.IsNotExist(err) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		src = resolved
	}
	dst := filepath.Join(root, name, now.Format("2006-01-02T15-04-05"), c.Path)
	if err := copyDir(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// computeChecksums walks a directory and returns a map of relative path to sha256 hex.
func computeChecksums(dir string) (map[string]string, error) {
	checksums := make(map[string]string)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		checksums[rel] = hex.EncodeToString(hash[:])
		return nil
	})
	return checksums, err
}

// diffChecksums returns the sorted paths whose checksum differs or that exist on one side only.
func diffChecksums(want, got map[string]string) []string {
	var changed []string
	for path, sum := range want {
		if got[path] != sum {
			changed = append(changed, path)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
