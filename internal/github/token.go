package github

import (
	"os"
	"os/exec"
	"strings"
)

// Token returns a GitHub token by checking GITHUB_TOKEN env var first,
// then falling back to `gh auth token` if gh is installed.
func Token() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
