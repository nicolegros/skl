package skills

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nicolegros/skl/internal/config"
	"github.com/nicolegros/skl/internal/lock"
)

// legacyInstall writes a pre-symlink install: a lock entry without links and real copies in every dir.
func legacyInstall(t *testing.T, env Env, content string) {
	t.Helper()
	lock.Save(&lock.File{Skills: []lock.Skill{
		{Name: "repo", Repo: "owner/repo", Commit: "abc123", Files: map[string]string{"SKILL.md": sha(content)}},
	}}, env.Lock)
	setSkills(t, env, "skills:\n  repo: owner/repo\n")
	for _, dir := range env.Dirs {
		writeFile(t, filepath.Join(dir, "repo", "SKILL.md"), content)
	}
}

func runConverge(t *testing.T, env Env, resolve Resolver) []Result {
	t.Helper()
	results, err := Converge(loadConfig(t), env, resolve)
	if err != nil {
		t.Fatalf("Converge() error = %v", err)
	}
	return results
}

func TestConverge_InstallsNewEntriesAtDeclaredRef(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-abc123", map[string]string{"tdd/SKILL.md": "# TDD"})
	env := newEnv(t, url, 1)
	setSkills(t, env, "skills:\n  tdd: owner/repo/tdd@v1\n")

	results := runConverge(t, env, skipAll)

	want := []Result{{Name: "tdd", Source: config.Source{Repo: "owner/repo", Path: "tdd", Ref: "v1"}, Action: Installed}}
	if !slices.Equal(results, want) {
		t.Errorf("Converge() = %+v, want %+v", results, want)
	}
	if !slices.Equal(up.refs, []string{"v1"}) {
		t.Errorf("fetched refs = %v, want [v1]", up.refs)
	}
	assertLinked(t, env.Dirs[0], "tdd", env.Store)
	if s := loadLock(t, env).Skills[0]; s.Ref != "v1" || s.Commit != "abc123" {
		t.Errorf("lock entry = %+v", s)
	}
}

func TestConverge_SecondRunIsSilentAndFetchesNothing(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"tdd/SKILL.md":   "# TDD",
		"grill/SKILL.md": "# Grill",
	})
	env := newEnv(t, url, 2)
	setSkills(t, env, "skills:\n  grill: owner/repo/grill\n  tdd: owner/repo/tdd\n")
	runConverge(t, env, skipAll)
	before := snapshot(t, env)
	fetches := len(up.refs)
	r := &recorder{}

	if results := runConverge(t, env, r.resolve); len(results) != 0 {
		t.Errorf("second Converge() = %+v, want nothing to report", results)
	}
	if len(up.refs) != fetches {
		t.Errorf("second run fetched %v", up.refs[fetches:])
	}
	if len(r.calls) != 0 {
		t.Errorf("second run called the resolver: %+v", r.calls)
	}
	if after := snapshot(t, env); !slices.Equal(before, after) {
		t.Errorf("second run changed disk:\nbefore %v\nafter  %v", before, after)
	}
}

func TestConverge_RefetchesMissingStoreAtLockedCommit(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# v1"})
	env := newEnv(t, url, 1)
	mustInstall(t, env, "repo", "owner/repo")
	os.RemoveAll(filepath.Join(env.Store, "repo"))
	up.tarball = makeTarball(t, "owner-repo-def456", map[string]string{"SKILL.md": "# v2"})
	up.byRef["abc123"] = makeTarball(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# v1"})

	if results := runConverge(t, env, skipAll); len(results) != 0 {
		t.Errorf("Converge() = %+v, want a silent repair", results)
	}
	if got := readFile(t, filepath.Join(env.Dirs[0], "repo", "SKILL.md")); got != "# v1" {
		t.Errorf("SKILL.md = %q, want the locked # v1", got)
	}
	if got := up.refs[len(up.refs)-1]; got != "abc123" {
		t.Errorf("fetched ref %q, want the locked commit", got)
	}
}

func TestConverge_ChangedSourceReinstalls(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"grill/SKILL.md": "---\nname: grill\n---\n",
		"quiz/SKILL.md":  "---\nname: quiz\n---\n",
	})
	env := newEnv(t, url, 1)
	mustInstall(t, env, "ask-me", "owner/repo/grill")
	writeFile(t, filepath.Join(env.Store, "ask-me", "SKILL.md"), "# edited")
	setSkills(t, env, "skills:\n  ask-me: owner/repo/quiz\n")
	fetches := len(up.refs)

	t.Run("skip keeps the old install", func(t *testing.T) {
		r := &recorder{resolution: Skip}
		results := runConverge(t, env, r.resolve)

		if len(results) != 1 || !results[0].Skipped {
			t.Errorf("Converge() = %+v, want ask-me skipped", results)
		}
		if len(r.calls) != 1 || r.calls[0][0].Kind != StoreEdited {
			t.Errorf("resolver calls = %+v, want one StoreEdited conflict", r.calls)
		}
		if s := loadLock(t, env).Skills[0]; s.Path != "grill" {
			t.Errorf("lock entry = %+v, want the old grill source kept", s)
		}
	})

	t.Run("overwrite installs the new source", func(t *testing.T) {
		results := runConverge(t, env, func(string, []Conflict) Resolution { return Overwrite })

		want := []Result{{Name: "ask-me", Source: config.Source{Repo: "owner/repo", Path: "quiz"}, Action: Installed}}
		if !slices.Equal(results, want) {
			t.Errorf("Converge() = %+v, want %+v", results, want)
		}
		if got := readFile(t, filepath.Join(env.Dirs[0], "ask-me", "SKILL.md")); got != "---\nname: ask-me\n---\n" {
			t.Errorf("SKILL.md = %q, want quiz patched to ask-me", got)
		}
		if s := loadLock(t, env).Skills[0]; s.Name != "quiz" || s.Alias != "ask-me" || s.Path != "quiz" {
			t.Errorf("lock entry = %+v", s)
		}
	})
	if len(up.refs) == fetches {
		t.Error("changed source was not fetched")
	}
}

func TestConverge_PrunesEntriesNotInManifest(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"tdd/SKILL.md":   "# TDD",
		"grill/SKILL.md": "# Grill",
	})
	env := newEnv(t, url, 2)
	mustInstall(t, env, "tdd", "owner/repo/tdd")
	mustInstall(t, env, "grill", "owner/repo/grill")
	setSkills(t, env, "skills:\n  tdd: owner/repo/tdd\n")

	results := runConverge(t, env, skipAll)

	want := []Result{{Name: "grill", Source: config.Source{Repo: "owner/repo", Path: "grill"}, Action: Removed}}
	if !slices.Equal(results, want) {
		t.Errorf("Converge() = %+v, want %+v", results, want)
	}
	for _, dir := range env.Dirs {
		if _, err := os.Lstat(filepath.Join(dir, "grill")); !os.IsNotExist(err) {
			t.Errorf("%s/grill still exists", dir)
		}
		assertLinked(t, dir, "tdd", env.Store)
	}
	if _, err := os.Stat(filepath.Join(env.Store, "grill")); !os.IsNotExist(err) {
		t.Error("grill store copy still exists")
	}
	if lf := loadLock(t, env); len(lf.Skills) != 1 || lf.Skills[0].Name != "tdd" {
		t.Errorf("lock = %+v, want only tdd", lf.Skills)
	}
}

func TestConverge_PruneOfEditedStoreGoesThroughResolver(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Upstream"})

	setup := func(t *testing.T) Env {
		env := newEnv(t, url, 1)
		mustInstall(t, env, "repo", "owner/repo")
		writeFile(t, filepath.Join(env.Dirs[0], "repo", "SKILL.md"), "# edited through the link")
		setSkills(t, env, "skills: {}\n")
		return env
	}

	t.Run("skip keeps everything", func(t *testing.T) {
		env := setup(t)
		r := &recorder{resolution: Skip}

		results := runConverge(t, env, r.resolve)

		if len(results) != 1 || results[0].Action != Removed || !results[0].Skipped {
			t.Errorf("Converge() = %+v, want repo skipped", results)
		}
		store := filepath.Join(env.Store, "repo")
		if len(r.calls) != 1 || len(r.calls[0]) != 1 || r.calls[0][0].Kind != StoreEdited || r.calls[0][0].Path != store {
			t.Fatalf("resolver calls = %+v, want one StoreEdited conflict at %s", r.calls, store)
		}
		if got := readFile(t, filepath.Join(env.Dirs[0], "repo", "SKILL.md")); got != "# edited through the link" {
			t.Errorf("SKILL.md through the link = %q, want the edit kept", got)
		}
		if lf := loadLock(t, env); len(lf.Skills) != 1 {
			t.Errorf("lock = %+v, want the entry kept", lf.Skills)
		}
	})

	t.Run("backup and overwrite removes it", func(t *testing.T) {
		env := setup(t)

		results := runConverge(t, env, func(string, []Conflict) Resolution { return BackupAndOverwrite })

		if len(results) != 1 || results[0].Skipped || results[0].Err != nil {
			t.Errorf("Converge() = %+v, want repo removed", results)
		}
		if _, err := os.Lstat(filepath.Join(env.Dirs[0], "repo")); !os.IsNotExist(err) {
			t.Error("link still exists")
		}
		if _, err := os.Stat(filepath.Join(env.Store, "repo")); !os.IsNotExist(err) {
			t.Error("store copy still exists")
		}
		matches, _ := filepath.Glob(filepath.Join(env.Backups, "repo", "*", env.Store, "repo", "SKILL.md"))
		if len(matches) != 1 || readFile(t, matches[0]) != "# edited through the link" {
			t.Errorf("backups = %v, want the edited store copy", matches)
		}
		if lf := loadLock(t, env); len(lf.Skills) != 0 {
			t.Errorf("lock = %+v, want empty", lf.Skills)
		}
	})
}

func TestConverge_OneFailingEntryDoesNotBlockOthers(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Skill"})
	up.missing["gone"] = true
	env := newEnv(t, url, 1)
	setSkills(t, env, "skills:\n  aaa: owner/gone\n  zzz: owner/repo\n")

	results := runConverge(t, env, skipAll)

	if len(results) != 2 || results[0].Name != "aaa" || results[0].Err == nil || results[1].Name != "zzz" || results[1].Err != nil {
		t.Fatalf("Converge() = %+v, want aaa failed and zzz installed", results)
	}
	if got := results[0].Err.Error(); got != "GitHub returned 404: repository not found (if private, set GITHUB_TOKEN environment variable)" {
		t.Errorf("aaa error = %q", got)
	}
	assertLinked(t, env.Dirs[0], "zzz", env.Store)
	if lf := loadLock(t, env); len(lf.Skills) != 1 || lf.Skills[0].InstalledName() != "zzz" {
		t.Errorf("lock = %+v, want only zzz so aaa is retried", lf.Skills)
	}

	delete(up.missing, "gone")
	if results := runConverge(t, env, skipAll); len(results) != 1 || results[0].Name != "aaa" || results[0].Err != nil {
		t.Errorf("retry = %+v, want aaa installed", results)
	}
}

func TestConverge_ResumesAfterStoreWrittenButLockNotSaved(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Skill"})
	env := newEnv(t, url, 1)
	mustInstall(t, env, "repo", "owner/repo")
	os.Remove(env.Lock)
	r := &recorder{}

	results := runConverge(t, env, r.resolve)

	if len(results) != 1 || results[0].Action != Installed || results[0].Skipped {
		t.Errorf("Converge() = %+v, want repo installed", results)
	}
	if len(r.calls) != 0 {
		t.Errorf("resolver called with %+v for a store already holding upstream content", r.calls)
	}
	if s := loadLock(t, env).Skills[0]; s.Commit != "abc123" || !slices.Equal(s.Links, env.Dirs) {
		t.Errorf("lock entry = %+v", s)
	}
}

func TestConverge_MigratesIdenticalCopiesWithoutPrompting(t *testing.T) {
	up, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Upstream"})
	env := newEnv(t, url, 2)
	legacyInstall(t, env, "# Upstream")
	r := &recorder{}

	runConverge(t, env, r.resolve)

	if !slices.Equal(up.refs, []string{"abc123"}) {
		t.Errorf("fetched refs = %v, want the locked commit only", up.refs)
	}
	if len(r.calls) != 0 {
		t.Errorf("resolver called with %+v", r.calls)
	}
	for _, dir := range env.Dirs {
		assertLinked(t, dir, "repo", env.Store)
	}
	if s := loadLock(t, env).Skills[0]; !slices.Equal(s.Links, env.Dirs) || s.Commit != "abc123" {
		t.Errorf("lock entry = %+v", s)
	}
}

func TestConverge_MigrationWithEditedCopy(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Upstream"})
	env := newEnv(t, url, 2)
	legacyInstall(t, env, "# Upstream")
	edited := filepath.Join(env.Dirs[0], "repo")
	writeFile(t, filepath.Join(edited, "SKILL.md"), "# Edited")
	r := &recorder{resolution: Skip}

	runConverge(t, env, r.resolve)

	if len(r.calls) != 1 || len(r.calls[0]) != 1 || r.calls[0][0].Kind != LocalDir || r.calls[0][0].Path != edited {
		t.Fatalf("resolver calls = %+v, want one LocalDir conflict at %s", r.calls, edited)
	}
	assertLinked(t, env.Dirs[1], "repo", env.Store)
	if links := loadLock(t, env).Skills[0].Links; !slices.Equal(links, []string{env.Dirs[1]}) {
		t.Errorf("lock links = %v", links)
	}

	second := &recorder{resolution: Overwrite}
	runConverge(t, env, second.resolve)
	if len(second.calls) != 0 {
		t.Errorf("second converge prompted again: %+v", second.calls)
	}
	if got := readFile(t, filepath.Join(edited, "SKILL.md")); got != "# Edited" {
		t.Errorf("skipped dir changed to %q", got)
	}
}

func TestConverge_MigrationSkippedEverywhereStillMarksMigrated(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Upstream"})
	env := newEnv(t, url, 1)
	legacyInstall(t, env, "# Edited")

	runConverge(t, env, skipAll)

	if links := loadLock(t, env).Skills[0].Links; links == nil || len(links) != 0 {
		t.Errorf("lock links = %#v, want empty non-nil", links)
	}
}

func TestConverge_RemovesOurLinksFromUnconfiguredDirs(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{
		"tdd/SKILL.md":   "# TDD",
		"grill/SKILL.md": "# Grill",
	})
	env := newEnv(t, url, 2)
	mustInstall(t, env, "tdd", "owner/repo/tdd")
	mustInstall(t, env, "grill", "owner/repo/grill")
	dropped := env.Dirs[1]
	foreign := filepath.Join(dropped, "grill")
	os.Remove(foreign)
	os.Symlink(t.TempDir(), foreign)

	env.Dirs = env.Dirs[:1]
	runConverge(t, env, skipAll)

	if _, err := os.Lstat(filepath.Join(dropped, "tdd")); !os.IsNotExist(err) {
		t.Error("our link in the dropped dir was kept")
	}
	if info, err := os.Lstat(foreign); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("foreign link in the dropped dir was removed")
	}
	assertLinked(t, env.Dirs[0], "tdd", env.Store)
	for _, s := range loadLock(t, env).Skills {
		if !slices.Equal(s.Links, env.Dirs) {
			t.Errorf("%s links = %v, want %v", s.Name, s.Links, env.Dirs)
		}
	}
}

func TestConverge_IsIdempotentAfterMigration(t *testing.T) {
	_, url := newUpstream(t, "owner-repo-abc123", map[string]string{"SKILL.md": "# Upstream"})
	env := newEnv(t, url, 2)
	legacyInstall(t, env, "# Upstream")
	writeFile(t, filepath.Join(env.Dirs[0], "repo", "SKILL.md"), "# Edited")

	runConverge(t, env, func(string, []Conflict) Resolution { return Overwrite })
	before := snapshot(t, env)

	r := &recorder{}
	runConverge(t, env, r.resolve)
	if len(r.calls) != 0 {
		t.Errorf("second converge called the resolver: %+v", r.calls)
	}
	if after := snapshot(t, env); !slices.Equal(before, after) {
		t.Errorf("second converge changed disk:\nbefore %v\nafter  %v", before, after)
	}
}

// snapshot lists every path under the store and agent dirs with its link target or content, plus the lock and skl.yaml.
func snapshot(t *testing.T, env Env) []string {
	t.Helper()
	var out []string
	for _, root := range append([]string{env.Store}, env.Dirs...) {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			entry := path
			if d.Type()&fs.ModeSymlink != 0 {
				target, _ := os.Readlink(path)
				entry += " -> " + target
			} else if !d.IsDir() {
				entry += " = " + readFile(t, path)
			}
			out = append(out, entry)
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
	}
	return append(out, readFile(t, env.Lock), readFile(t, manifestPath()))
}
