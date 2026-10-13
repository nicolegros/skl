package skills

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nicolegros/skl/internal/lock"
)

type InstallOptions struct {
	Owner string
	Repo  string
	Path  string // subdirectory within repo, empty for root
	Ref   string // declared branch, tag, or SHA; empty for the default branch
	Alias string // install under a different name
}

// Result reports one skill a command acted on.
type Result struct {
	Name    string
	Skipped bool // the user skipped a store conflict, so the store was left as is
}

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

// Reconcile brings disk in line with the lock file: stores filled, links in place.
func Reconcile(env Env, resolve Resolver) error {
	lf, err := lock.Load(env.Lock)
	if err != nil {
		return err
	}
	if len(lf.Skills) == 0 {
		return fmt.Errorf("lock file is empty or missing")
	}
	return errors.Join(reconcile(lf, env, resolve, ""), lock.Save(lf, env.Lock))
}

// reconcile syncs every lock entry except the one named except.
// Only entries not yet migrated from copies get the interactive resolver; a conflict
// the user already skipped stays skipped and shows up in `skl list`.
func reconcile(lf *lock.File, env Env, resolve Resolver, except string) error {
	f := newFetcher(env)
	defer f.cleanup()

	for i, entry := range lf.Skills {
		name := entry.InstalledName()
		if name == except {
			continue
		}
		r := resolve
		if entry.Links != nil {
			r = skipAll
		}
		var src *fetched
		if !exists(filepath.Join(env.Store, name)) {
			env.logf("Fetching %s from %s@%s", name, entry.Repo, entry.Commit)
			dir, commit, err := f.fetch(entry.Repo, entry.Commit, entry.Path)
			if err != nil {
				return fmt.Errorf("fetching %s: %w", name, err)
			}
			src = &fetched{dir: dir, commit: commit}
		}
		updated, _, err := sync(entry, src, env, r)
		if err != nil {
			return fmt.Errorf("linking %s: %w", name, err)
		}
		lf.Skills[i] = updated
	}
	return nil
}

// Install fetches a skill from GitHub into the store and links it into every configured directory.
func Install(opts InstallOptions, env Env, resolve Resolver) (Result, error) {
	lf, err := lock.Load(env.Lock)
	if err != nil {
		return Result{}, err
	}
	if err := reconcile(lf, env, resolve, ""); err != nil {
		return Result{}, errors.Join(err, lock.Save(lf, env.Lock))
	}

	f := newFetcher(env)
	defer f.cleanup()
	srcDir, commit, err := f.fetch(opts.Owner+"/"+opts.Repo, opts.Ref, opts.Path)
	if err != nil {
		return Result{}, err
	}

	skillName := opts.Repo
	if opts.Path != "" {
		skillName = filepath.Base(opts.Path)
	}
	entry := lock.Skill{Name: skillName, Repo: opts.Owner + "/" + opts.Repo, Path: opts.Path, Ref: opts.Ref, Alias: opts.Alias}
	existing, found := lf.Get(entry.InstalledName())
	if opts.Alias != "" {
		ownedByOther := found && (existing.Name != entry.Name || existing.Repo != entry.Repo || existing.Path != entry.Path)
		if ownedByOther || (!found && exists(filepath.Join(env.Store, opts.Alias))) {
			return Result{}, fmt.Errorf("%q already exists; remove it first or choose a different name", opts.Alias)
		}
	}
	if found {
		entry.Commit, entry.Files, entry.Links = existing.Commit, existing.Files, existing.Links
	}

	updated, wrote, err := sync(entry, &fetched{dir: srcDir, commit: commit}, env, resolve)
	if err != nil {
		return Result{}, errors.Join(err, lock.Save(lf, env.Lock))
	}
	lf.Add(updated)
	return Result{Name: entry.InstalledName(), Skipped: !wrote}, lock.Save(lf, env.Lock)
}

// InstallAll installs every skill found in the repo.
func InstallAll(opts InstallOptions, env Env, resolve Resolver) ([]Result, error) {
	lf, err := lock.Load(env.Lock)
	if err != nil {
		return nil, err
	}
	if err := reconcile(lf, env, resolve, ""); err != nil {
		return nil, errors.Join(err, lock.Save(lf, env.Lock))
	}

	extractedRoot, commit, cleanup, err := fetchAndExtract(env.BaseURL, opts.Owner, opts.Repo, opts.Ref, env.Token)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	discovered, err := Discover(extractedRoot)
	if err != nil {
		return nil, err
	}
	if len(discovered) == 0 {
		return nil, fmt.Errorf("no skills found in %s/%s", opts.Owner, opts.Repo)
	}

	var results []Result
	for _, skill := range discovered {
		path := skill.Path
		if path == "." {
			path = ""
		}
		entry := lock.Skill{Name: skill.Name, Repo: opts.Owner + "/" + opts.Repo, Path: path, Ref: opts.Ref}
		if existing, ok := lf.Get(skill.Name); ok {
			entry.Commit, entry.Files, entry.Links = existing.Commit, existing.Files, existing.Links
		}
		src := &fetched{dir: filepath.Join(extractedRoot, skill.Path), commit: commit}
		updated, wrote, err := sync(entry, src, env, resolve)
		if err != nil {
			return results, errors.Join(fmt.Errorf("installing %s: %w", skill.Name, err), lock.Save(lf, env.Lock))
		}
		lf.Add(updated)
		results = append(results, Result{Name: skill.Name, Skipped: !wrote})
	}
	return results, lock.Save(lf, env.Lock)
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
