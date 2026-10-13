package config

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// Source is where a skill comes from, written owner/repo[/path][@ref].
type Source struct {
	Repo string // owner/repo
	Path string // subdirectory, empty for the repo root
	Ref  string // branch, tag, or SHA; empty for the default branch
}

// ParseSource accepts owner/repo[/path][@ref] or a GitHub URL, including /tree/<ref>/<path> links.
func ParseSource(s string) (Source, error) {
	invalid := fmt.Errorf("invalid source %q (expected owner/repo[/path][@ref])", s)
	if strings.ContainsAny(s, " \t\r\n") {
		return Source{}, invalid
	}
	isURL := strings.Contains(s, "://")
	rest := s
	if isURL {
		u, err := url.Parse(s)
		if err != nil || (u.Host != "github.com" && u.Host != "www.github.com") {
			return Source{}, fmt.Errorf("not a GitHub URL: %s", s)
		}
		rest = strings.Trim(u.Path, "/")
	}

	rest, ref, hasRef := strings.Cut(rest, "@")
	if hasRef && ref == "" {
		return Source{}, invalid
	}
	if parts := strings.SplitN(rest, "/", 5); isURL && len(parts) >= 4 && parts[2] == "tree" {
		if hasRef {
			return Source{}, invalid
		}
		rest, ref = parts[0]+"/"+parts[1], parts[3]
		if len(parts) == 5 {
			rest += "/" + parts[4]
		}
	}

	segs := strings.Split(strings.TrimSuffix(rest, "/"), "/")
	if len(segs) < 2 {
		return Source{}, invalid
	}
	segs[1] = strings.TrimSuffix(segs[1], ".git")
	for _, seg := range segs {
		if seg == "" || seg == "." || seg == ".." {
			return Source{}, invalid
		}
	}
	return Source{Repo: segs[0] + "/" + segs[1], Path: strings.Join(segs[2:], "/"), Ref: ref}, nil
}

// String renders the normalized form written to skl.yaml.
func (s Source) String() string {
	out := s.Repo
	if s.Path != "" {
		out += "/" + s.Path
	}
	if s.Ref != "" {
		out += "@" + s.Ref
	}
	return out
}

// Name is the directory name the skill has upstream: the last path segment, or the repo name.
func (s Source) Name() string {
	if s.Path != "" {
		return path.Base(s.Path)
	}
	_, repo, _ := strings.Cut(s.Repo, "/")
	return repo
}
