package skills

import (
	"errors"
	"fmt"

	"github.com/nicolegros/skl/internal/lock"
)

type UpdateResult struct {
	Results  []Result
	Warnings []string
}

// Update refreshes installed skills from their upstream repos. An empty name updates all.
func Update(name string, env Env, resolve Resolver) (*UpdateResult, error) {
	lf, err := lock.Load(env.Lock)
	if err != nil {
		return nil, err
	}
	if name != "" {
		if _, ok := lf.Get(name); !ok {
			return nil, fmt.Errorf("skill %q not found in lock file", name)
		}
	}
	if err := reconcile(lf, env, resolve, ""); err != nil {
		return nil, errors.Join(err, lock.Save(lf, env.Lock))
	}

	f := newFetcher(env)
	defer f.cleanup()

	result := &UpdateResult{}
	for i, entry := range lf.Skills {
		installed := entry.InstalledName()
		if name != "" && installed != name {
			continue
		}
		if entry.Pinned {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%q is pinned (was at %s), updating anyway", installed, entry.Ref))
		}

		dir, ref, err := f.fetch(entry.Repo, "", entry.Path)
		if err != nil {
			return result, errors.Join(fmt.Errorf("updating %s: %w", installed, err), lock.Save(lf, env.Lock))
		}
		updated, wrote, err := sync(entry, &fetched{dir: dir, ref: ref, pinned: entry.Pinned}, env, resolve)
		if err != nil {
			return result, errors.Join(fmt.Errorf("updating %s: %w", installed, err), lock.Save(lf, env.Lock))
		}
		lf.Skills[i] = updated
		result.Results = append(result.Results, Result{Name: installed, Skipped: !wrote})
	}
	return result, lock.Save(lf, env.Lock)
}
