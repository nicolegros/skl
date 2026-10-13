package skills

import (
	"fmt"

	"github.com/nicolegros/skl/internal/config"
)

// Update converges, re-resolving the declared ref of the named skill, or of every skill when name is empty.
// It writes the lock, never skl.yaml.
func Update(cfg *config.Config, name string, env Env, resolve Resolver) ([]Result, error) {
	if _, ok := cfg.Skills[name]; name != "" && !ok {
		return nil, fmt.Errorf("skill %q not found in skl.yaml", name)
	}
	return converge(cfg.Skills, func(n string) bool { return name == "" || n == name }, env, resolve)
}
