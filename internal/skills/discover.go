package skills

import (
	"fmt"
	"os"
	"path/filepath"
)

// DiscoveredSkill represents a skill found during directory scanning.
type DiscoveredSkill struct {
	Name string // leaf directory name
	Path string // relative path from root
}

// Discover recursively scans root for directories containing SKILL.md.
// If root itself contains SKILL.md, it returns just that as a single skill.
// Errors on name collisions among leaf directory names.
func Discover(root string) ([]DiscoveredSkill, error) {
	// Check if root itself is a skill
	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err == nil {
		return []DiscoveredSkill{{Name: filepath.Base(root), Path: "."}}, nil
	}

	var found []DiscoveredSkill
	seen := map[string]string{} // name -> first path

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() != "SKILL.md" || info.IsDir() {
			return nil
		}
		dir := filepath.Dir(path)
		name := filepath.Base(dir)
		rel, _ := filepath.Rel(root, dir)

		if prev, exists := seen[name]; exists {
			return fmt.Errorf("name collision: %q found at both %q and %q", name, prev, rel)
		}
		seen[name] = rel
		found = append(found, DiscoveredSkill{Name: name, Path: rel})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}
