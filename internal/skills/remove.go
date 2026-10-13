package skills

import (
	"fmt"
	"maps"
	"slices"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/lock"
)

// Remove converges without name, which prunes it, then drops its line unless the prune was skipped or failed.
func Remove(cfg *config.Config, name string, env Env, resolve Resolver) ([]Result, error) {
	lf, err := lock.Load(env.Lock)
	if err != nil {
		return nil, err
	}
	changes := plan(cfg.Skills, lf)
	i := slices.IndexFunc(changes, func(c change) bool { return c.Name == name })
	if i < 0 {
		return nil, fmt.Errorf("skill %q is not in skl.yaml or the lock file", name)
	}
	manifest := maps.Clone(cfg.Skills)
	delete(manifest, name)
	results, err := converge(manifest, never, env, resolve)
	if err != nil {
		return results, err
	}
	if c := changes[i]; c.Kind == Add {
		// Never installed, so there is nothing to prune; dropping the line is the whole removal.
		results = append([]Result{{Name: name, Source: *c.Want, Action: Removed}}, results...)
	}
	if slices.ContainsFunc(results, func(r Result) bool { return r.Name == name && (r.Err != nil || r.Skipped) }) {
		return results, nil
	}
	delete(cfg.Skills, name)
	return results, cfg.Save()
}
