package skills

import (
	"fmt"
	"strings"

	"github.com/nicolegros/skl/internal/lock"
)

type UpdateOptions struct {
	Name     string // empty = update all
	BaseURL  string
	Dirs     []string
	LockPath string
	Token    string
	Force    bool
}

// SkillModification represents a skill that has local modifications in one or more directories.
type SkillModification struct {
	SkillName string
	Dirs      []Modification
}

type UpdateResult struct {
	Updated       []string
	Warnings      []string
	Modifications []SkillModification
}

// Update refreshes installed skills from their upstream repos.
func Update(opts UpdateOptions) (*UpdateResult, error) {
	lf, err := lock.Load(opts.LockPath)
	if err != nil {
		return nil, err
	}

	result := &UpdateResult{}

	for _, skill := range lf.Skills {
		if opts.Name != "" && skill.Name != opts.Name && skill.Alias != opts.Name {
			continue
		}

		if skill.Pinned {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%q is pinned (was at %s), updating anyway", skill.Name, skill.Ref))
		}

		// Check for local modifications before overwriting
		if !opts.Force && skill.Files != nil {
			mods := CheckModifications(skill.Name, opts.Dirs, skill.Files)
			if len(mods) > 0 {
				result.Modifications = append(result.Modifications, SkillModification{
					SkillName: skill.Name,
					Dirs:      mods,
				})
				continue
			}
		}

		// Parse owner/repo from lock entry
		parts := strings.SplitN(skill.Repo, "/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid repo in lock: %s", skill.Repo)
		}

		_, err := Install(InstallOptions{
			Owner:    parts[0],
			Repo:     parts[1],
			Path:     skill.Path,
			Ref:      "", // latest
			Pinned:   skill.Pinned,
			Alias:    skill.Alias,
			BaseURL:  opts.BaseURL,
			Dirs:     opts.Dirs,
			LockPath: opts.LockPath,
			Token:    opts.Token,
			Force:    true, // already checked modifications at this level
		})
		if err != nil {
			return nil, fmt.Errorf("updating %s: %w", skill.Name, err)
		}

		result.Updated = append(result.Updated, skill.Name)
	}

	return result, nil
}
