package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/nicolegros/skl/internal/skills"
)

// promptForModifications handles the interactive flow when local modifications are detected.
// Returns true if the user chose to proceed with overwrite (after optional backup).
func promptForModifications(skillName string, mods []skills.Modification) bool {
	fmt.Fprintf(os.Stderr, "\n⚠️  Skill %q has local modifications:\n", skillName)
	for _, m := range mods {
		fmt.Fprintf(os.Stderr, "  %s:\n", m.Dir)
		for _, f := range m.ModifiedFiles {
			fmt.Fprintf(os.Stderr, "    - %s\n", f)
		}
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(os.Stderr, "\n  [d]iff  [b]ackup & overwrite  [o]verwrite  [s]kip: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "d", "diff":
			showDiffs(skillName, mods)
		case "b", "backup":
			for _, m := range mods {
				bakPath, err := skills.Backup(m.Dir, skillName)
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

// showDiffs prints the contents of locally modified files.
func showDiffs(skillName string, mods []skills.Modification) {
	for _, m := range mods {
		skillDir := fmt.Sprintf("%s/%s", m.Dir, skillName)
		fmt.Fprintf(os.Stderr, "\n  Directory: %s\n", m.Dir)
		for _, file := range m.ModifiedFiles {
			filePath := fmt.Sprintf("%s/%s", skillDir, file)
			data, err := os.ReadFile(filePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\n  --- %s (deleted from disk)\n", file)
				continue
			}
			fmt.Fprintf(os.Stderr, "\n  --- %s (local version) ---\n", file)
			for _, line := range strings.Split(string(data), "\n") {
				fmt.Fprintf(os.Stderr, "  | %s\n", line)
			}
		}
	}
}
