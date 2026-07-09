package cmd

import (
	"bufio"
	"fmt"
	"os"
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
			localData, localErr := os.ReadFile(localPath)
			original := originals[file]

			if localErr != nil {
				// File deleted locally
				fmt.Fprintf(os.Stderr, "  --- a/%s\n  +++ /dev/null\n", file)
				for _, line := range strings.Split(original, "\n") {
					fmt.Fprintf(os.Stderr, "  -%s\n", line)
				}
			} else if original == "" {
				// File added locally (not in original)
				fmt.Fprintf(os.Stderr, "  --- /dev/null\n  +++ b/%s\n", file)
				for _, line := range strings.Split(string(localData), "\n") {
					fmt.Fprintf(os.Stderr, "  +%s\n", line)
				}
			} else {
				// File modified — show unified diff
				fmt.Fprintf(os.Stderr, "  --- a/%s (upstream @ %s)\n  +++ b/%s (local)\n", file, skill.Ref[:7], file)
				printUnifiedDiff(original, string(localData))
			}
		}
	}
}

// printUnifiedDiff prints a simple line-by-line diff between two strings.
func printUnifiedDiff(original, local string) {
	origLines := strings.Split(original, "\n")
	localLines := strings.Split(local, "\n")

	// Simple LCS-based diff
	diff := computeDiff(origLines, localLines)
	for _, d := range diff {
		switch d.op {
		case diffEqual:
			fmt.Fprintf(os.Stderr, "   %s\n", d.line)
		case diffDelete:
			fmt.Fprintf(os.Stderr, "  -%s\n", d.line)
		case diffInsert:
			fmt.Fprintf(os.Stderr, "  +%s\n", d.line)
		}
	}
}

type diffOp int

const (
	diffEqual  diffOp = iota
	diffDelete
	diffInsert
)

type diffLine struct {
	op   diffOp
	line string
}

// computeDiff produces a minimal diff between two slices of lines using Myers' algorithm (simplified).
func computeDiff(a, b []string) []diffLine {
	// Build LCS table
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// Backtrack to produce diff
	var result []diffLine
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && a[i-1] == b[j-1] {
			result = append(result, diffLine{diffEqual, a[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			result = append(result, diffLine{diffInsert, b[j-1]})
			j--
		} else {
			result = append(result, diffLine{diffDelete, a[i-1]})
			i--
		}
	}

	// Reverse
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}
