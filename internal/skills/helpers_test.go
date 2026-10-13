package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/lock"
)

// makeTarball creates a .tar.gz in memory with the given files.
// GitHub tarballs have a top-level prefix dir like "owner-repo-sha/".
func makeTarball(t *testing.T, prefix string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		tw.WriteHeader(&tar.Header{Name: prefix + "/" + name, Size: int64(len(content)), Mode: 0o644, Typeflag: tar.TypeReg})
		tw.Write([]byte(content))
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

// upstream is a fake GitHub serving one tarball, or a per-ref one, and recording requested refs.
type upstream struct {
	tarball []byte
	byRef   map[string][]byte
	missing map[string]bool // repos answering 404
	refs    []string
}

func newUpstream(t *testing.T, prefix string, files map[string]string) (*upstream, string) {
	t.Helper()
	u := &upstream{tarball: makeTarball(t, prefix, files), byRef: map[string][]byte{}, missing: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// /repos/<owner>/<repo>/tarball/<ref>
		if u.missing[strings.Split(r.URL.Path, "/")[3]] {
			http.NotFound(w, r)
			return
		}
		ref := path.Base(r.URL.Path)
		u.refs = append(u.refs, ref)
		if tb, ok := u.byRef[ref]; ok {
			w.Write(tb)
			return
		}
		w.Write(u.tarball)
	}))
	t.Cleanup(srv.Close)
	return u, srv.URL
}

// newEnv points XDG at fresh dirs and writes a skl.yaml listing that many agent directories and no skills.
func newEnv(t *testing.T, baseURL string, dirs int) Env {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	env := Env{
		Paths: Paths{
			Store:   config.StoreDir(),
			Backups: config.BackupsDir(),
			Lock:    config.LockPath(),
		},
		BaseURL: baseURL,
	}
	agents := t.TempDir()
	for i := range dirs {
		env.Dirs = append(env.Dirs, filepath.Join(agents, string(rune('a'+i)), "skills"))
	}
	setSkills(t, env, "skills: {}\n")
	return env
}

// setSkills writes skl.yaml with env's dirs followed by skills, a raw `skills:` block.
func setSkills(t *testing.T, env Env, skills string) {
	t.Helper()
	writeFile(t, manifestPath(), dirsBlock(env)+skills)
}

func dirsBlock(env Env) string {
	out := "directories:\n"
	for _, d := range env.Dirs {
		out += "  - " + d + "\n"
	}
	return out
}

func manifestPath() string {
	return filepath.Join(config.Dir(), "skl.yaml")
}

func loadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("loading skl.yaml: %v", err)
	}
	return cfg
}

// installAs runs `skl install <source> --as name` the way the CLI does.
func installAs(t *testing.T, env Env, name, source string, resolve Resolver) ([]Result, error) {
	t.Helper()
	src, err := config.ParseSource(source)
	if err != nil {
		t.Fatal(err)
	}
	return Install(loadConfig(t), map[string]config.Source{name: src}, env, resolve)
}

// mustInstall installs and fails the test on any command or per-skill error.
func mustInstall(t *testing.T, env Env, name, source string) {
	t.Helper()
	results, err := installAs(t, env, name, source, skipAll)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Err != nil {
			t.Fatalf("installing %s: %v", r.Name, r.Err)
		}
	}
}

type recorder struct {
	resolution Resolution
	skills     []string
	calls      [][]Conflict
}

func (r *recorder) resolve(skill string, conflicts []Conflict) Resolution {
	r.skills = append(r.skills, skill)
	r.calls = append(r.calls, conflicts)
	return r.resolution
}

func assertLinked(t *testing.T, dir, name, store string) {
	t.Helper()
	target, err := os.Readlink(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("%s/%s is not a symlink: %v", dir, name, err)
	}
	if want := filepath.Join(store, name); target != want || !filepath.IsAbs(target) {
		t.Errorf("%s/%s -> %q, want absolute %q", dir, name, target, want)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func loadLock(t *testing.T, env Env) *lock.File {
	t.Helper()
	lf, err := lock.Load(env.Lock)
	if err != nil {
		t.Fatalf("loading lock: %v", err)
	}
	return lf
}
