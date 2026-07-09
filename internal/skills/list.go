package skills

import (
	"fmt"

	"github.com/nicolegros/skl/internal/lock"
)

type ListEntry struct {
	DisplayName string
	Source      string
	Ref         string
	Pinned      bool
	Alias       string // non-empty if installed under different name
}

type ListOptions struct {
	LockPath string
}

// List returns information about installed skills from the lock file.
func List(opts ListOptions) ([]ListEntry, error) {
	lf, err := lock.Load(opts.LockPath)
	if err != nil {
		return nil, err
	}

	var entries []ListEntry
	for _, s := range lf.Skills {
		entry := ListEntry{
			DisplayName: s.Name,
			Source:      s.Repo,
			Ref:         s.Ref,
			Pinned:      s.Pinned,
		}
		if s.Alias != "" {
			entry.DisplayName = fmt.Sprintf("%s (%s @ %s)", s.Alias, s.Name, s.Repo)
			entry.Alias = s.Alias
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
