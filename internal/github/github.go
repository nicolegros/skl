package github

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseRepo accepts "owner/repo" or a full GitHub URL and returns owner and repo.
func ParseRepo(input string) (string, string, error) {
	if strings.Contains(input, "://") {
		u, err := url.Parse(input)
		if err != nil || (u.Host != "github.com" && u.Host != "www.github.com") {
			return "", "", fmt.Errorf("not a GitHub URL: %s", input)
		}
		input = strings.TrimPrefix(u.Path, "/")
	}

	input = strings.TrimSuffix(input, ".git")
	input = strings.TrimSuffix(input, "/")

	parts := strings.Split(input, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo format: %q (expected owner/repo)", input)
	}
	return parts[0], parts[1], nil
}
