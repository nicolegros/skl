package skills

import (
	"fmt"
	"path/filepath"

	"github.com/nicolegros/skl/internal/lock"
)

type ListEntry struct {
	DisplayName string
	Source      string
	Ref         string
	Pinned      bool
	Alias       string   // non-empty if installed under different name
	Unlinked    []string // configured dirs without our link
}

// List returns information about installed skills from the lock file.
func List(p Paths) ([]ListEntry, error) {
	lf, err := lock.Load(p.Lock)
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
		storePath := filepath.Join(p.Store, s.InstalledName())
		for _, dir := range p.Dirs {
			if state, _ := linkState(dir, s.InstalledName(), storePath); state != stateLinked {
				entry.Unlinked = append(entry.Unlinked, dir)
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
