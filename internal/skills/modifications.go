package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Modification represents local changes detected in one directory.
type Modification struct {
	Dir           string   // parent directory (e.g., ~/.kiro/skills)
	SkillDir      string   // full path to the skill directory on disk
	ModifiedFiles []string
}

// computeChecksums walks a directory and returns a map of relative path → sha256 hex.
func computeChecksums(dir string) (map[string]string, error) {
	checksums := make(map[string]string)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		checksums[rel] = hex.EncodeToString(hash[:])
		return nil
	})
	return checksums, err
}

// fileChecksum returns the sha256 hex digest of a file's contents.
func fileChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// CheckModifications compares files on disk against stored checksums for a skill.
// Returns a Modification entry for each directory that has local changes.
func CheckModifications(skillName string, dirs []string, checksums map[string]string) []Modification {
	var mods []Modification

	for _, dir := range dirs {
		skillDir := filepath.Join(dir, skillName)
		if _, err := os.Stat(skillDir); os.IsNotExist(err) {
			continue
		}

		var modified []string

		// Check files that should exist (from checksums)
		for relPath, expectedHash := range checksums {
			filePath := filepath.Join(skillDir, relPath)
			actual, err := fileChecksum(filePath)
			if err != nil {
				// File was deleted or unreadable
				modified = append(modified, relPath)
				continue
			}
			if actual != expectedHash {
				modified = append(modified, relPath)
			}
		}

		// Check for added files (on disk but not in checksums)
		_ = filepath.Walk(skillDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(skillDir, path)
			if _, exists := checksums[rel]; !exists {
				modified = append(modified, rel)
			}
			return nil
		})

		if len(modified) > 0 {
			sort.Strings(modified)
			mods = append(mods, Modification{Dir: dir, SkillDir: skillDir, ModifiedFiles: modified})
		}
	}

	return mods
}

// Backup copies a skill directory to a .bak sibling directory.
// Returns the path to the backup directory.
func Backup(dir, skillName string) (string, error) {
	src := filepath.Join(dir, skillName)
	dst := filepath.Join(dir, skillName+".bak")

	// Remove existing backup
	os.RemoveAll(dst)

	if err := copyDir(src, dst); err != nil {
		return "", fmt.Errorf("creating backup: %w", err)
	}
	return dst, nil
}

// FetchOriginalFiles fetches the upstream version of a skill at a given ref and returns
// a map of relative file path → content for the specified files.
func FetchOriginalFiles(baseURL, owner, repo, path, ref, token string, files []string) (map[string]string, error) {
	extractedRoot, _, cleanup, err := fetchAndExtract(baseURL, owner, repo, ref, token)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	srcDir := extractedRoot
	if path != "" {
		srcDir = filepath.Join(extractedRoot, path)
	}

	contents := make(map[string]string)
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(srcDir, f))
		if err != nil {
			// File didn't exist in original (was added locally)
			continue
		}
		contents[f] = string(data)
	}
	return contents, nil
}
