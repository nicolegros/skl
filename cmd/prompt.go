package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nicolegros/skl/internal/lock"
	"github.com/nicolegros/skl/internal/skills"
)

// promptContext carries the info needed to fetch originals for diffing.
type promptContext struct {
	BaseURL  string
	Token    string
	LockPath string
}

// promptForModifications handles the interactive flow when local modifications are detected.
// Returns true if the user chose to proceed with overwrite (after optional backup).
func promptForModifications(skillName string, mods []skills.Modification, ctx promptContext) bool {
	fmt.Fprintf(os.Stderr, "\n⚠️  Skill %q has local modifications:\n", skillName)
	for _, m := range mods {
		fmt.Fprintf(os.Stderr, "  %s:\n", m.SkillDir)
		for _, f := range m.ModifiedFiles {
			fmt.Fprintf(os.Stderr, "    - %s\n", f)
		}
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(os.Stderr, "\n  [d]iff  [b]ackup & overwrite  [o]verwrite  [s]kip: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n")
			return false
		}
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "d", "diff":
			showDiffs(skillName, mods, ctx)
		case "b", "backup":
			for _, m := range mods {
				dirName := filepath.Base(m.SkillDir)
				bakPath, err := skills.Backup(m.Dir, dirName)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Error backing up: %v\n", err)
					return false
				}
				fmt.Fprintf(os.Stderr, "  Backed up to %s\n", bakPath)
			}
			return true
		case "o", "overwrite":
			return true
		case "s", "skip":
			return false
		default:
			fmt.Fprintf(os.Stderr, "  Unknown option %q. Choose d/b/o/s.\n", input)
		}
	}
}

// showDiffs fetches the original version and shows a unified diff against local.
func showDiffs(skillName string, mods []skills.Modification, ctx promptContext) {
	// Look up the skill in the lock to get repo/ref/path
	lf, err := lock.Load(ctx.LockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error loading lock: %v\n", err)
		return
	}

	var skill lock.Skill
	found := false
	for _, s := range lf.Skills {
		if s.Name == skillName || s.Alias == skillName {
			skill = s
			found = true
			break
		}
	}
	if !found {
		fmt.Fprintf(os.Stderr, "  Could not find skill in lock file\n")
		return
	}

	// Collect all modified files across all dirs
	var allFiles []string
	for _, m := range mods {
		allFiles = append(allFiles, m.ModifiedFiles...)
	}

	// Fetch original files from upstream at the locked ref
	parts := strings.SplitN(skill.Repo, "/", 2)
	originals, err := skills.FetchOriginalFiles(ctx.BaseURL, parts[0], parts[1], skill.Path, skill.Ref, ctx.Token, allFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error fetching original: %v\n", err)
		return
	}

	for _, m := range mods {
		fmt.Fprintf(os.Stderr, "\n")
		for _, file := range m.ModifiedFiles {
			localPath := filepath.Join(m.SkillDir, file)

			original := originals[file]
			localData, localErr := os.ReadFile(localPath)

			if localErr != nil && original == "" {
				continue
			}

			// Write original to a temp file for diff
			tmpOrig, err := os.CreateTemp("", "skl-orig-*")
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
				continue
			}
			_, _ = tmpOrig.WriteString(original)
			tmpOrig.Close()

			// Write local to a temp file (or /dev/null if deleted)
			tmpLocal, err := os.CreateTemp("", "skl-local-*")
			if err != nil {
				os.Remove(tmpOrig.Name())
				fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
				continue
			}
			if localErr == nil {
				_, _ = tmpLocal.Write(localData)
			}
			tmpLocal.Close()

			labelA := fmt.Sprintf("a/%s (upstream @ %s)", file, skill.Ref[:minInt(7, len(skill.Ref))])
			labelB := fmt.Sprintf("b/%s (local)", file)

			cmd := exec.Command("diff", "-u", "--label", labelA, "--label", labelB, tmpOrig.Name(), tmpLocal.Name())
			output, _ := cmd.Output() // diff exits 1 when files differ, that's fine
			if len(output) > 0 {
				fmt.Fprintf(os.Stderr, "%s", output)
			}

			os.Remove(tmpOrig.Name())
			os.Remove(tmpLocal.Name())
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
