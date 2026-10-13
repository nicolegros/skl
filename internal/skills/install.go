package skills

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/nicolegros/skl/internal/config"
)

// fetchAndExtract downloads a tarball and extracts it to a temp directory.
// Returns the extracted root path, the commit it resolved to, and a cleanup function.
func fetchAndExtract(baseURL, owner, repo, ref, token string) (extractedRoot, commit string, cleanup func(), err error) {
	url := fmt.Sprintf("%s/repos/%s/%s/tarball/%s", baseURL, owner, repo, ref)
	req, _ := http.NewRequest("GET", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", nil, fmt.Errorf("fetching tarball: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound && token == "" {
			return "", "", nil, fmt.Errorf("GitHub returned 404: repository not found (if private, set GITHUB_TOKEN environment variable)")
		}
		return "", "", nil, fmt.Errorf("GitHub returned %d", resp.StatusCode)
	}

	tmpDir, err := os.MkdirTemp("", "skills-install-*")
	if err != nil {
		return "", "", nil, err
	}
	cleanup = func() { os.RemoveAll(tmpDir) }

	if err := extractTarball(resp.Body, tmpDir); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("extracting tarball: %w", err)
	}

	entries, _ := os.ReadDir(tmpDir)
	if len(entries) == 0 {
		cleanup()
		return "", "", nil, fmt.Errorf("empty tarball")
	}

	extractedRoot = filepath.Join(tmpDir, entries[0].Name())

	// The tarball prefix is owner-repo-SHA whatever ref was asked for.
	parts := strings.Split(entries[0].Name(), "-")
	if len(parts) < 3 {
		cleanup()
		return "", "", nil, fmt.Errorf("unexpected tarball prefix %q", entries[0].Name())
	}
	return extractedRoot, parts[len(parts)-1], cleanup, nil
}

// fetcher downloads each repo@ref once per command.
type fetcher struct {
	env      Env
	roots    map[string]fetchedRoot
	cleanups []func()
}

type fetchedRoot struct {
	dir    string
	commit string
}

func newFetcher(env Env) *fetcher {
	return &fetcher{env: env, roots: map[string]fetchedRoot{}}
}

// fetch returns the skill directory at path within repo@ref and the commit ref resolved to.
func (f *fetcher) fetch(repo, ref, path string) (string, string, error) {
	key := repo + "@" + ref
	root, ok := f.roots[key]
	if !ok {
		owner, name, found := strings.Cut(repo, "/")
		if !found {
			return "", "", fmt.Errorf("invalid repo in lock: %s", repo)
		}
		dir, commit, cleanup, err := fetchAndExtract(f.env.BaseURL, owner, name, ref, f.env.Token)
		if err != nil {
			return "", "", err
		}
		f.cleanups = append(f.cleanups, cleanup)
		root = fetchedRoot{dir: dir, commit: commit}
		f.roots[key] = root
	}
	dir := filepath.Join(root.dir, path)
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		return "", "", fmt.Errorf("no SKILL.md found in %s", path)
	}
	return dir, root.commit, nil
}

func (f *fetcher) cleanup() {
	for _, c := range f.cleanups {
		c()
	}
}

// Install adds wanted to skl.yaml and converges.
// A name already listed from another repo or path is refused before anything changes.
// Each line is written to skl.yaml only once its skill is in place.
func Install(cfg *config.Config, wanted map[string]config.Source, env Env, resolve Resolver) ([]Result, error) {
	for name, src := range wanted {
		if have, ok := cfg.Skills[name]; ok && (have.Repo != src.Repo || have.Path != src.Path) {
			return nil, fmt.Errorf("%s is already installed from %s. Use --as <name>, or edit skl.yaml to switch source", name, have)
		}
	}

	manifest := maps.Clone(cfg.Skills)
	maps.Copy(manifest, wanted)
	results, err := converge(manifest, never, env, resolve)
	if err != nil {
		return results, err
	}
	for name, src := range wanted {
		if !slices.ContainsFunc(results, func(r Result) bool { return r.Name == name && (r.Err != nil || r.Skipped) }) {
			cfg.Skills[name] = src
		}
	}
	return results, cfg.Save()
}

// DiscoverAll returns a source for every skill under src, keyed by name.
func DiscoverAll(src config.Source, env Env) (map[string]config.Source, error) {
	owner, repo, _ := strings.Cut(src.Repo, "/")
	root, _, cleanup, err := fetchAndExtract(env.BaseURL, owner, repo, src.Ref, env.Token)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	found, err := Discover(filepath.Join(root, src.Path))
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("no skills found in %s", src)
	}
	wanted := map[string]config.Source{}
	for _, d := range found {
		s := src
		if d.Path != "." {
			s.Path = path.Join(src.Path, filepath.ToSlash(d.Path))
		}
		wanted[s.Name()] = s
	}
	return wanted, nil
}

// FetchOriginalFiles fetches the upstream version of a skill at a given ref and returns
// a map of relative file path to content for the specified files.
func FetchOriginalFiles(env Env, repo, path, ref string, files []string) (map[string]string, error) {
	f := newFetcher(env)
	defer f.cleanup()
	srcDir, _, err := f.fetch(repo, ref, path)
	if err != nil {
		return nil, err
	}

	contents := make(map[string]string)
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(srcDir, file))
		if err != nil {
			continue // added locally
		}
		contents[file] = string(data)
	}
	return contents, nil
}

func extractTarball(r io.Reader, dest string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

// replacePathRefs replaces /<oldName>/ and /<oldName> (at segment boundaries)
// with /<newName> in all files under dir. Only matches whole path segments to
// avoid corrupting longer names like /<oldName>-extended.
func replacePathRefs(dir, oldName, newName string) {
	// Match /<oldName> followed by /, whitespace, quote, end-of-line, or end-of-string
	pattern := regexp.MustCompile(`/` + regexp.QuoteMeta(oldName) + `([/\s"'` + "`" + `\])}\n]|$)`)
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		if pattern.MatchString(content) {
			content = pattern.ReplaceAllString(content, "/"+newName+"${1}")
			_ = os.WriteFile(path, []byte(content), info.Mode())
		}
		return nil
	})
}

// patchFrontmatterName replaces the name: field in SKILL.md frontmatter.
// Returns true if a name field was found and patched.
func patchFrontmatterName(dir, oldName, newName string) bool {
	skillMd := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(skillMd)
	if err != nil {
		return false
	}
	content := string(data)

	// Replace name: <oldName> in frontmatter (between --- delimiters)
	lines := strings.Split(content, "\n")
	inFrontmatter := false
	found := false
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break // end of frontmatter
		}
		if inFrontmatter && strings.HasPrefix(strings.TrimSpace(line), "name:") {
			lines[i] = "name: " + newName
			found = true
		}
	}

	if found {
		_ = os.WriteFile(skillMd, []byte(strings.Join(lines, "\n")), 0o644)
	}
	return found
}
