package skills

import (
	"archive/tar"
	"compress/gzip"
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
	Owner    string
	Repo     string
	Path     string // subdirectory within repo, empty for root
	Ref      string
	Pinned   bool
	Alias    string // install under a different name
	BaseURL  string // override for testing (GitHub API base)
	Dirs     []string
	LockPath string
	Token    string
	Logf     func(string, ...any) // optional logger for warnings
	Force    bool
}

// fetchAndExtract downloads a tarball and extracts it to a temp directory.
// Returns the extracted root path, resolved ref, and a cleanup function.
func fetchAndExtract(baseURL, owner, repo, ref, token string) (extractedRoot, resolvedRef string, cleanup func(), err error) {
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

	// Resolve ref from tarball prefix if not specified (format: owner-repo-SHA)
	resolvedRef = ref
	if resolvedRef == "" {
		parts := strings.Split(entries[0].Name(), "-")
		if len(parts) >= 3 {
			resolvedRef = parts[len(parts)-1]
		}
	}

	return extractedRoot, resolvedRef, cleanup, nil
}

// InstallResult contains the outcome of an install operation.
type InstallResult struct {
	Name          string
	Modifications []Modification // non-nil when local modifications detected and Force=false
}

// Install fetches a skill from GitHub and installs it into all configured directories.
// Returns the installed skill name.
func Install(opts InstallOptions) (*InstallResult, error) {
	extractedRoot, resolvedRef, cleanup, err := fetchAndExtract(opts.BaseURL, opts.Owner, opts.Repo, opts.Ref, opts.Token)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	srcDir := extractedRoot
	skillName := opts.Repo
	if opts.Path != "" {
		srcDir = filepath.Join(extractedRoot, opts.Path)
		skillName = filepath.Base(opts.Path)
	}

	if _, err := os.Stat(filepath.Join(srcDir, "SKILL.md")); os.IsNotExist(err) {
		return nil, fmt.Errorf("no SKILL.md found in %s", opts.Path)
	}

	// Check for local modifications if not forcing
	if !opts.Force {
		lf, err := lock.Load(opts.LockPath)
		if err != nil {
			return nil, err
		}
		for _, s := range lf.Skills {
			if s.Name == skillName && s.Files != nil {
				mods := CheckModifications(skillName, opts.Dirs, s.Files)
				if len(mods) > 0 {
					return &InstallResult{Modifications: mods}, nil
				}
				break
			}
		}
	}

	// Determine the installed directory name
	installedName := skillName
	if opts.Alias != "" {
		installedName = opts.Alias
		// Block if alias name already exists on disk, unless it's our own skill being updated
		if skillExists(installedName, opts.Dirs) {
			lf, err := lock.Load(opts.LockPath)
			if err != nil {
				return nil, err
			}
			ownedByUs := false
			for _, s := range lf.Skills {
				if s.Alias == opts.Alias && s.Name == skillName {
					ownedByUs = true
					break
				}
			}
			if !ownedByUs {
				return nil, fmt.Errorf("%q already exists; remove it first or choose a different name", installedName)
			}
		}
	}

	for _, dir := range opts.Dirs {
		dest := filepath.Join(dir, installedName)
		os.RemoveAll(dest)
		if err := copyDir(srcDir, dest); err != nil {
			return nil, fmt.Errorf("copying to %s: %w", dir, err)
		}
		if opts.Alias != "" {
			if !patchFrontmatterName(dest, skillName, opts.Alias) {
				if opts.Logf != nil {
					opts.Logf("warning: SKILL.md has no frontmatter name field to patch")
				}
			}
			replacePathRefs(dest, skillName, opts.Alias)
		}
	}

	checksums, err := computeChecksums(srcDir)
	if err != nil {
		return nil, fmt.Errorf("computing checksums: %w", err)
	}

	lf, err := lock.Load(opts.LockPath)
	if err != nil {
		return nil, err
	}
	entry := lock.Skill{
		Name:   skillName,
		Repo:   opts.Owner + "/" + opts.Repo,
		Path:   opts.Path,
		Ref:    resolvedRef,
		Pinned: opts.Pinned,
		Files:  checksums,
	}
	if opts.Alias != "" {
		entry.Alias = opts.Alias
	}
	lf.Add(entry)
	return &InstallResult{Name: installedName}, lock.Save(lf, opts.LockPath)
}

// InstallAllResult contains the outcome of an install-all operation.
type InstallAllResult struct {
	Installed []string
	Skipped   []InstallResult // skills skipped due to local modifications
}

// InstallAll fetches all skills from a repo using --all flag.
// Returns the list of installed skill names and any skipped skills with modifications.
func InstallAll(opts InstallOptions) (*InstallAllResult, error) {
	extractedRoot, resolvedRef, cleanup, err := fetchAndExtract(opts.BaseURL, opts.Owner, opts.Repo, opts.Ref, opts.Token)
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

	lf, err := lock.Load(opts.LockPath)
	if err != nil {
		return nil, err
	}

	result := &InstallAllResult{}
	for _, skill := range discovered {
		srcDir := filepath.Join(extractedRoot, skill.Path)

		if !opts.Force {
			for _, s := range lf.Skills {
				if s.Name == skill.Name && s.Files != nil {
					mods := CheckModifications(skill.Name, opts.Dirs, s.Files)
					if len(mods) > 0 {
						result.Skipped = append(result.Skipped, InstallResult{
							Name:          skill.Name,
							Modifications: mods,
						})
						goto nextSkill
					}
					break
				}
			}
		}

		for _, dir := range opts.Dirs {
			dest := filepath.Join(dir, skill.Name)
			os.RemoveAll(dest)
			if err := copyDir(srcDir, dest); err != nil {
				return nil, fmt.Errorf("copying %s: %w", skill.Name, err)
			}
		}
		{
			checksums, err := computeChecksums(srcDir)
			if err != nil {
				return nil, fmt.Errorf("computing checksums for %s: %w", skill.Name, err)
			}
			path := skill.Path
			if path == "." {
				path = ""
			}
			lf.Add(lock.Skill{
				Name:   skill.Name,
				Repo:   opts.Owner + "/" + opts.Repo,
				Path:   path,
				Ref:    resolvedRef,
				Pinned: opts.Pinned,
				Files:  checksums,
			})
			result.Installed = append(result.Installed, skill.Name)
		}
	nextSkill:
	}

	return result, lock.Save(lf, opts.LockPath)
}

// InstallFromLock installs all skills from the lock file that are missing from disk.
func InstallFromLock(lockPath, baseURL, token string, dirs []string, logf func(string, ...any)) error {
	lf, err := lock.Load(lockPath)
	if err != nil {
		return err
	}
	if len(lf.Skills) == 0 {
		return fmt.Errorf("lock file is empty or missing")
	}

	for _, s := range lf.Skills {
		checkName := s.Name
		if s.Alias != "" {
			checkName = s.Alias
		}
		if skillExists(checkName, dirs) {
			logf("Skipping %s (already installed)", checkName)
			continue
		}
		logf("Installing %s from %s@%s", checkName, s.Repo, s.Ref)
		parts := strings.SplitN(s.Repo, "/", 2)
		_, err := Install(InstallOptions{
			Owner:    parts[0],
			Repo:     parts[1],
			Path:     s.Path,
			Ref:      s.Ref,
			Pinned:   s.Pinned,
			Alias:    s.Alias,
			BaseURL:  baseURL,
			Dirs:     dirs,
			LockPath: lockPath,
			Token:    token,
		})
		if err != nil {
			return fmt.Errorf("installing %s: %w", checkName, err)
		}
	}
	return nil
}

func skillExists(name string, dirs []string) bool {
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, name, "SKILL.md")); err == nil {
			return true
		}
	}
	return false
}

// ExpandPath resolves ~ to home directory.
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
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

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
