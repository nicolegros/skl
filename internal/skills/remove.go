package skills

import (
	"fmt"
	"slices"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/lock"
)

// Remove drops name from skl.yaml and converges, which prunes it.
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
	delete(cfg.Skills, name)
	if err := cfg.Save(); err != nil {
		return nil, err
	}
	results, err := converge(cfg.Skills, never, env, resolve)
	if c := changes[i]; c.Kind == Add {
		// Never installed, so there is nothing to prune; dropping the line is the whole removal.
		results = append([]Result{{Name: name, Source: *c.Want, Action: Removed}}, results...)
	}
	return results, err
}
