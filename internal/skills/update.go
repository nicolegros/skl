package skills

import (
	"fmt"
	"strings"

	"github.com/nicolaslegros/skills/internal/lock"
)

type UpdateOptions struct {
	Name     string // empty = update all
	BaseURL  string
	Dirs     []string
	LockPath string
	Token    string
}

type UpdateResult struct {
	Updated  []string
	Warnings []string
}

// Update refreshes installed skills from their upstream repos.
func Update(opts UpdateOptions) (*UpdateResult, error) {
	lf, err := lock.Load(opts.LockPath)
	if err != nil {
		return nil, err
	}

	result := &UpdateResult{}

	for _, skill := range lf.Skills {
		if opts.Name != "" && skill.Name != opts.Name {
			continue
		}

		if skill.Pinned {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%q is pinned (was at %s), updating anyway", skill.Name, skill.Ref))
		}

		// Parse owner/repo from lock entry
		parts := strings.SplitN(skill.Repo, "/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid repo in lock: %s", skill.Repo)
		}

		err := Install(InstallOptions{
			Owner:    parts[0],
			Repo:     parts[1],
			Path:     skill.Path,
			Ref:      "", // latest
			Pinned:   skill.Pinned,
			BaseURL:  opts.BaseURL,
			Dirs:     opts.Dirs,
			LockPath: opts.LockPath,
			Token:    opts.Token,
		})
		if err != nil {
			return nil, fmt.Errorf("updating %s: %w", skill.Name, err)
		}

		result.Updated = append(result.Updated, skill.Name)
	}

	return result, nil
}
