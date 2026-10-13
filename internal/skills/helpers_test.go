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
	"testing"

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

// upstream is a fake GitHub serving one tarball and recording requested refs.
type upstream struct {
	tarball []byte
	refs    []string
}

func newUpstream(t *testing.T, prefix string, files map[string]string) (*upstream, string) {
	t.Helper()
	u := &upstream{tarball: makeTarball(t, prefix, files)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.refs = append(u.refs, path.Base(r.URL.Path))
		w.Write(u.tarball)
	}))
	t.Cleanup(srv.Close)
	return u, srv.URL
}

func newEnv(t *testing.T, baseURL string, dirs int) Env {
	t.Helper()
	data := t.TempDir()
	env := Env{
		Paths: Paths{
			Store:   filepath.Join(data, "skills"),
			Backups: filepath.Join(data, "backups"),
			Lock:    filepath.Join(t.TempDir(), "skl.lock"),
		},
		BaseURL: baseURL,
	}
	agents := t.TempDir()
	for i := range dirs {
		env.Dirs = append(env.Dirs, filepath.Join(agents, string(rune('a'+i)), "skills"))
	}
	return env
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
