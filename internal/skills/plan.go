package skills

import (
	"slices"
	"strings"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/lock"
)

// ChangeKind is how a manifest entry relates to its lock entry.
type ChangeKind int

const (
	Unchanged ChangeKind = iota // in both, same source
	Add                         // in the manifest only
	Changed                     // in both, repo, path, or declared ref differ
	Prune                       // in the lock only
)

// change pairs what skl.yaml wants with what the lock has under one installed name.
type change struct {
	Name string
	Kind ChangeKind
	Want *config.Source // nil for Prune
	Have *lock.Skill    // nil for Add
}

// plan diffs the manifest against the lock, sorted by name.
func plan(manifest map[string]config.Source, lf *lock.File) []change {
	var changes []change
	for name, src := range manifest {
		c := change{Name: name, Kind: Add, Want: &src}
		if have, ok := lf.Get(name); ok {
			c.Have = &have
			c.Kind = Changed
			if sourceOf(have) == src {
				c.Kind = Unchanged
			}
		}
		changes = append(changes, c)
	}
	for _, have := range lf.Skills {
		if _, ok := manifest[have.InstalledName()]; !ok {
			changes = append(changes, change{Name: have.InstalledName(), Kind: Prune, Have: &have})
		}
	}
	slices.SortFunc(changes, func(a, b change) int { return strings.Compare(a.Name, b.Name) })
	return changes
}

func sourceOf(s lock.Skill) config.Source {
	return config.Source{Repo: s.Repo, Path: s.Path, Ref: s.Ref}
}

// entryFor is a fresh lock entry for installing src under name, aliased when name differs upstream.
func entryFor(name string, src config.Source) lock.Skill {
	s := lock.Skill{Name: src.Name(), Repo: src.Repo, Path: src.Path, Ref: src.Ref}
	if name != s.Name {
		s.Alias = name
	}
	return s
}
