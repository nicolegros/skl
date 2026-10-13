package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/lock"
)

// Action is what a command did to one skill.
type Action int

const (
	Installed Action = iota
	Updated
	Removed
)

// Result reports one skill a command acted on.
type Result struct {
	Name    string
	Source  config.Source
	Action  Action
	Skipped bool  // the user skipped a conflict, so the skill was left as is
	Err     error // this skill failed; the others still ran
}

// Converge makes the store, links, and lock match skl.yaml.
func Converge(cfg *config.Config, env Env, resolve Resolver) ([]Result, error) {
	return converge(cfg.Skills, never, env, resolve)
}

func never(string) bool { return false }

// converge applies the plan one entry at a time, saving the lock after each, so a failure or crash
// leaves every other entry consistent and the next run picks up where this one stopped.
// refresh names the entries whose declared ref is re-resolved instead of reusing the locked commit.
func converge(manifest map[string]config.Source, refresh func(string) bool, env Env, resolve Resolver) ([]Result, error) {
	lf, err := lock.Load(env.Lock)
	if err != nil {
		return nil, err
	}
	f := newFetcher(env)
	defer f.cleanup()

	var results []Result
	for _, c := range plan(manifest, lf) {
		if r := apply(c, refresh(c.Name), f, lf, env, resolve); r != nil {
			results = append(results, *r)
		}
		if err := lock.Save(lf, env.Lock); err != nil {
			return results, err
		}
	}
	return results, nil
}

// apply converges one entry and returns what to report, or nil when nothing happened.
func apply(c change, refresh bool, f *fetcher, lf *lock.File, env Env, resolve Resolver) *Result {
	switch c.Kind {
	case Prune:
		r := &Result{Name: c.Name, Source: sourceOf(*c.Have), Action: Removed}
		r.Skipped, r.Err = prune(*c.Have, lf, env, resolve)
		return r
	case Add, Changed:
		r := &Result{Name: c.Name, Source: *c.Want, Action: Installed}
		r.Skipped, r.Err = install(c, f, lf, env, resolve)
		return r
	default:
		r := &Result{Name: c.Name, Source: *c.Want, Action: Updated}
		var moved bool
		moved, r.Skipped, r.Err = relink(*c.Have, refresh, f, lf, env, resolve)
		if !moved && r.Err == nil {
			return nil
		}
		return r
	}
}

// install fetches the wanted source and puts it in place of whatever the lock had under that name.
// Skipping a store conflict keeps the previous lock entry, so the change is retried next run.
func install(c change, f *fetcher, lf *lock.File, env Env, resolve Resolver) (bool, error) {
	entry := entryFor(c.Name, *c.Want)
	if c.Have != nil {
		entry.Files, entry.Links = c.Have.Files, c.Have.Links
	}
	dir, commit, err := f.fetch(c.Want.Repo, c.Want.Ref, c.Want.Path)
	if err != nil {
		return false, err
	}
	updated, wrote, err := sync(entry, &fetched{dir: dir, commit: commit}, env, resolve)
	if err != nil {
		return false, err
	}
	switch {
	case wrote:
		lf.Add(updated)
	case c.Have != nil:
		kept := *c.Have
		kept.Links = updated.Links
		lf.Add(kept)
	}
	return !wrote, nil
}

// relink keeps an unchanged entry at its locked commit, refetching only a missing store copy, and fixes its links.
// With refresh it re-resolves the declared ref first and reports whether the commit moved.
func relink(entry lock.Skill, refresh bool, f *fetcher, lf *lock.File, env Env, resolve Resolver) (moved, skipped bool, err error) {
	name := entry.InstalledName()
	storeMissing := !exists(filepath.Join(env.Store, name))
	var src *fetched
	switch {
	case refresh:
		dir, commit, err := f.fetch(entry.Repo, entry.Ref, entry.Path)
		if err != nil {
			return false, false, err
		}
		moved = commit != entry.Commit
		if moved || storeMissing {
			src = &fetched{dir: dir, commit: commit}
		}
	case storeMissing:
		env.logf("Fetching %s from %s@%s", name, entry.Repo, entry.Commit)
		dir, commit, err := f.fetch(entry.Repo, entry.Commit, entry.Path)
		if err != nil {
			return false, false, err
		}
		src = &fetched{dir: dir, commit: commit}
	}

	// Only a new commit or an entry not yet migrated from copies gets the interactive resolver;
	// a link conflict the user already skipped stays skipped and shows up in `skl list`.
	if !moved && entry.Links != nil {
		resolve = skipAll
	}
	updated, wrote, err := sync(entry, src, env, resolve)
	if err != nil {
		return moved, false, err
	}
	lf.Add(updated)
	return moved, moved && !wrote, nil
}

// prune deletes a skill's links, store copy, and lock entry, and reports whether the user chose to keep it.
// An edited store copy goes through resolve first, and skipping keeps everything.
// Copies left by pre-symlink installs are deleted only if the user agrees.
func prune(entry lock.Skill, lf *lock.File, env Env, resolve Resolver) (bool, error) {
	name := entry.InstalledName()
	storePath := filepath.Join(env.Store, name)
	var conflicts []Conflict
	storeEdited := false
	if exists(storePath) {
		current, err := computeChecksums(storePath)
		if err != nil {
			return false, err
		}
		if changed := diffChecksums(entry.Files, current); len(changed) > 0 {
			conflicts = append(conflicts, Conflict{Path: storePath, Kind: StoreEdited, Files: changed})
			storeEdited = true
		}
	}

	dirs := slices.Concat(entry.Links, env.Dirs)
	slices.Sort(dirs)
	var links []string
	for _, dir := range slices.Compact(dirs) {
		path := filepath.Join(dir, name)
		switch state, _ := linkState(dir, name, storePath); state {
		case stateLinked:
			links = append(links, path)
		case stateLocalDir:
			// Only a legacy entry owns its real directories; after migration they belong to the user.
			if entry.Links != nil || !slices.Contains(env.Dirs, dir) {
				continue
			}
			local, err := computeChecksums(path)
			if err != nil {
				return false, err
			}
			conflicts = append(conflicts, Conflict{Path: path, Kind: LocalDir, Files: diffChecksums(entry.Files, local)})
		}
	}

	resolution := Skip
	if len(conflicts) > 0 {
		resolution = resolve(name, conflicts)
	}
	if storeEdited && resolution == Skip {
		return true, nil
	}
	if resolution == BackupAndOverwrite {
		now := time.Now()
		for _, c := range conflicts {
			dst, err := backup(c, env.Backups, name, now)
			if err != nil {
				return false, fmt.Errorf("backing up %s: %w", c.Path, err)
			}
			env.logf("Backed up %s to %s", c.Path, dst)
		}
	}

	for _, path := range links {
		if err := os.Remove(path); err != nil {
			return false, err
		}
	}
	for _, c := range conflicts {
		if c.Kind == LocalDir && resolution != Skip {
			if err := os.RemoveAll(c.Path); err != nil {
				return false, err
			}
		}
	}
	if err := os.RemoveAll(storePath); err != nil {
		return false, err
	}
	lf.Remove(name)
	return false, nil
}
