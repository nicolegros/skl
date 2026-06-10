package skills

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nicolegros/skl/internal/lock"
)

type InstallOptions struct {
	Owner    string
	Repo     string
	Path     string // subdirectory within repo, empty for root
	Ref      string
	Pinned   bool
	BaseURL  string // override for testing (GitHub API base)
	Dirs     []string
	LockPath string
	Token    string
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

// Install fetches a skill from GitHub and installs it into all configured directories.
func Install(opts InstallOptions) error {
	extractedRoot, resolvedRef, cleanup, err := fetchAndExtract(opts.BaseURL, opts.Owner, opts.Repo, opts.Ref, opts.Token)
	if err != nil {
		return err
	}
	defer cleanup()

	srcDir := extractedRoot
	skillName := opts.Repo
	if opts.Path != "" {
		srcDir = filepath.Join(extractedRoot, opts.Path)
		skillName = filepath.Base(opts.Path)
	}

	if _, err := os.Stat(filepath.Join(srcDir, "SKILL.md")); os.IsNotExist(err) {
		return fmt.Errorf("no SKILL.md found in %s", opts.Path)
	}

	for _, dir := range opts.Dirs {
		dest := filepath.Join(dir, skillName)
		os.RemoveAll(dest)
		if err := copyDir(srcDir, dest); err != nil {
			return fmt.Errorf("copying to %s: %w", dir, err)
		}
	}

	lf, err := lock.Load(opts.LockPath)
	if err != nil {
		return err
	}
	lf.Add(lock.Skill{
		Name:   skillName,
		Repo:   opts.Owner + "/" + opts.Repo,
		Path:   opts.Path,
		Ref:    resolvedRef,
		Pinned: opts.Pinned,
	})
	return lock.Save(lf, opts.LockPath)
}

// InstallAll fetches all skills from a repo using --all flag.
func InstallAll(opts InstallOptions) error {
	extractedRoot, resolvedRef, cleanup, err := fetchAndExtract(opts.BaseURL, opts.Owner, opts.Repo, opts.Ref, opts.Token)
	if err != nil {
		return err
	}
	defer cleanup()

	discovered, err := Discover(extractedRoot)
	if err != nil {
		return err
	}
	if len(discovered) == 0 {
		return fmt.Errorf("no skills found in %s/%s", opts.Owner, opts.Repo)
	}

	lf, err := lock.Load(opts.LockPath)
	if err != nil {
		return err
	}

	for _, skill := range discovered {
		srcDir := filepath.Join(extractedRoot, skill.Path)
		for _, dir := range opts.Dirs {
			dest := filepath.Join(dir, skill.Name)
			os.RemoveAll(dest)
			if err := copyDir(srcDir, dest); err != nil {
				return fmt.Errorf("copying %s: %w", skill.Name, err)
			}
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
		})
	}

	return lock.Save(lf, opts.LockPath)
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
		if skillExists(s.Name, dirs) {
			logf("Skipping %s (already installed)", s.Name)
			continue
		}
		logf("Installing %s from %s@%s", s.Name, s.Repo, s.Ref)
		parts := strings.SplitN(s.Repo, "/", 2)
		err := Install(InstallOptions{
			Owner:    parts[0],
			Repo:     parts[1],
			Path:     s.Path,
			Ref:      s.Ref,
			Pinned:   s.Pinned,
			BaseURL:  baseURL,
			Dirs:     dirs,
			LockPath: lockPath,
			Token:    token,
		})
		if err != nil {
			return fmt.Errorf("installing %s: %w", s.Name, err)
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
