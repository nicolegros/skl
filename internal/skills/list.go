package skills

import (
	"path/filepath"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/lock"
)

type ListEntry struct {
	Name     string
	Source   config.Source // as listed in skl.yaml, or as locked when it is not listed
	Commit   string        // empty when not installed
	Status   ChangeKind
	Unlinked []string // configured dirs without our link
}

// List returns every skill in skl.yaml or the lock, sorted by name.
func List(cfg *config.Config, p Paths) ([]ListEntry, error) {
	lf, err := lock.Load(p.Lock)
	if err != nil {
		return nil, err
	}

	var entries []ListEntry
	for _, c := range plan(cfg.Skills, lf) {
		e := ListEntry{Name: c.Name, Status: c.Kind}
		if c.Want != nil {
			e.Source = *c.Want
		} else {
			e.Source = sourceOf(*c.Have)
		}
		if c.Have != nil {
			e.Commit = c.Have.Commit
			storePath := filepath.Join(p.Store, c.Name)
			for _, dir := range p.Dirs {
				if state, _ := linkState(dir, c.Name, storePath); state != stateLinked {
					e.Unlinked = append(e.Unlinked, dir)
				}
			}
		}
		entries = append(entries, e)
	}
	return entries, nil
}
