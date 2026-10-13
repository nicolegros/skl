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

// One reader for the whole process so piped answers are not lost between prompts.
var stdin = bufio.NewReader(os.Stdin)

func resolver(force bool, env skills.Env) skills.Resolver {
	if force {
		return func(string, []skills.Conflict) skills.Resolution { return skills.Overwrite }
	}
	return func(skill string, conflicts []skills.Conflict) skills.Resolution {
		return prompt(skill, conflicts, env)
	}
}

func prompt(skill string, conflicts []skills.Conflict, env skills.Env) skills.Resolution {
	fmt.Fprintf(os.Stderr, "\n⚠️  Skill %q has local changes skl would overwrite:\n", skill)
	for _, c := range conflicts {
		switch c.Kind {
		case skills.StoreEdited:
			fmt.Fprintf(os.Stderr, "  %s (edited in store):\n", c.Path)
		case skills.LocalDir:
			fmt.Fprintf(os.Stderr, "  %s (local directory):\n", c.Path)
		case skills.ForeignLink:
			fmt.Fprintf(os.Stderr, "  %s (symlink -> %s)\n", c.Path, c.Target)
		}
		for _, f := range c.Files {
			fmt.Fprintf(os.Stderr, "    - %s\n", f)
		}
	}

	for {
		fmt.Fprintf(os.Stderr, "\n  [d]iff  [b]ackup & overwrite  [o]verwrite  [s]kip: ")
		input, err := stdin.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n")
			return skills.Skip
		}
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "d", "diff":
			showDiffs(skill, conflicts, env)
		case "b", "backup":
			return skills.BackupAndOverwrite
		case "o", "overwrite":
			return skills.Overwrite
		case "s", "skip":
			return skills.Skip
		default:
			fmt.Fprintf(os.Stderr, "  Unknown option %q. Choose d/b/o/s.\n", input)
		}
	}
}

// showDiffs prints a unified diff of each conflict against upstream at the locked ref.
func showDiffs(skill string, conflicts []skills.Conflict, env skills.Env) {
	var files []string
	for _, c := range conflicts {
		files = append(files, c.Files...)
	}

	var entry lock.Skill
	var originals map[string]string
	if len(files) > 0 {
		lf, err := lock.Load(env.Lock)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error loading lock: %v\n", err)
			return
		}
		var ok bool
		if entry, ok = lf.Get(skill); !ok {
			fmt.Fprintf(os.Stderr, "  Could not find skill in lock file\n")
			return
		}
		originals, err = skills.FetchOriginalFiles(env, entry.Repo, entry.Path, entry.Commit, files)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error fetching original: %v\n", err)
			return
		}
	}

	for _, c := range conflicts {
		fmt.Fprintf(os.Stderr, "\n")
		if c.Kind == skills.ForeignLink {
			fmt.Fprintf(os.Stderr, "%s: symlink -> %s\n", c.Path, c.Target)
			continue
		}
		for _, file := range c.Files {
			printDiff(file, originals[file], filepath.Join(c.Path, file), entry.Commit)
		}
	}
}

func printDiff(file, original, localPath, ref string) {
	localData, localErr := os.ReadFile(localPath)
	if localErr != nil && original == "" {
		return
	}

	tmpOrig, err := os.CreateTemp("", "skl-orig-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		return
	}
	defer os.Remove(tmpOrig.Name())
	_, _ = tmpOrig.WriteString(original)
	tmpOrig.Close()

	tmpLocal, err := os.CreateTemp("", "skl-local-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		return
	}
	defer os.Remove(tmpLocal.Name())
	if localErr == nil {
		_, _ = tmpLocal.Write(localData)
	}
	tmpLocal.Close()

	labelA := fmt.Sprintf("a/%s (upstream @ %s)", file, ref[:min(7, len(ref))])
	labelB := fmt.Sprintf("b/%s (local)", file)

	output, _ := exec.Command("diff", "-u", "--label", labelA, "--label", labelB, tmpOrig.Name(), tmpLocal.Name()).Output() // exits 1 when files differ
	if len(output) > 0 {
		fmt.Fprintf(os.Stderr, "%s", output)
	}
}
