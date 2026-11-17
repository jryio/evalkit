package workspace_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jryio/evalkit/internal/workspace"
)

func mustRun(tb testing.TB, dir, name string, args ...string) string {
	tb.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	// Fixture repos must not depend on the developer's git config (signing, tag rules).
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("%s %v in %s: %v\n%s", name, args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(tb testing.TB, path, content string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatal(err)
	}
}

func read(tb testing.TB, path string) string {
	tb.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// newRepo makes a git repository with two commits (v1 then v2 of a.txt), an
// untracked deps/ directory, and returns its path and the first commit.
func newRepo(tb testing.TB, dir string) (path, first string) {
	tb.Helper()
	write(tb, filepath.Join(dir, "a.txt"), "v1\n")
	mustRun(tb, dir, "git", "init", "-q", "-b", "main")
	mustRun(tb, dir, "git", "add", "a.txt")
	mustRun(tb, dir, "git", "commit", "-q", "-m", "one")
	first = mustRun(tb, dir, "git", "rev-parse", "HEAD")
	write(tb, filepath.Join(dir, "a.txt"), "v2\n")
	mustRun(tb, dir, "git", "commit", "-q", "-am", "two")
	mustRun(tb, dir, "git", "tag", "later")
	write(tb, filepath.Join(dir, "deps", "lib.txt"), "dep\n")
	write(tb, filepath.Join(dir, "scratch.txt"), "untracked\n")
	return dir, first
}

func newPool(tb testing.TB, cfg workspace.Config, suiteDir string, n int) *workspace.Pool {
	tb.Helper()
	p, err := workspace.NewPool(tb.Context(), cfg, suiteDir, filepath.Join(tb.TempDir(), "slots"), n)
	if err != nil {
		tb.Fatalf("NewPool: %v", err)
	}
	return p
}

func acquire(tb testing.TB, p *workspace.Pool, pins map[string]string) *workspace.Slot {
	tb.Helper()
	s, err := p.Acquire(tb.Context(), pins)
	if err != nil {
		tb.Fatalf("Acquire: %v", err)
	}
	return s
}

func TestPool_CloneSource(t *testing.T) {
	t.Parallel()
	suite := t.TempDir()
	_, first := newRepo(t, filepath.Join(suite, "repo"))
	p := newPool(t, workspace.Config{Sources: []workspace.Source{{Path: "repo", Include: []string{"deps"}}}}, suite, 1)
	s := acquire(t, p, nil)
	repo := s.Path("repo")

	if got := read(t, filepath.Join(repo, "a.txt")); got != "v2\n" {
		t.Errorf("a.txt = %q, want the pinned HEAD content", got)
	}
	if exists(filepath.Join(repo, "scratch.txt")) {
		t.Error("an untracked file of the source reached the slot")
	}
	if got := read(t, filepath.Join(repo, "deps", "lib.txt")); got != "dep\n" {
		t.Errorf("included deps/lib.txt = %q", got)
	}
	if refs := mustRun(t, repo, "git", "for-each-ref", "--format=%(refname)"); refs != "refs/heads/work" {
		t.Errorf("refs = %q, want only the work branch", refs)
	}
	if remotes := mustRun(t, repo, "git", "remote"); remotes != "" {
		t.Errorf("remotes = %q, want none", remotes)
	}

	// An agent edits, adds, commits, branches and stashes.
	write(t, filepath.Join(repo, "a.txt"), "hacked\n")
	write(t, filepath.Join(repo, "new.txt"), "new\n")
	mustRun(t, repo, "git", "add", "-A")
	mustRun(t, repo, "git", "commit", "-q", "-m", "agent")
	mustRun(t, repo, "git", "checkout", "-q", "-b", "agent-branch")
	write(t, filepath.Join(repo, "wip.txt"), "wip\n")
	write(t, filepath.Join(s.Dir, "stray.txt"), "stray\n")

	diffs, err := s.Diff(t.Context())
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	for _, want := range []string{"+hacked", "new.txt", "wip.txt"} {
		if !strings.Contains(diffs["repo"], want) {
			t.Errorf("diff lacks %q:\n%s", want, diffs["repo"])
		}
	}
	if strings.Contains(diffs["repo"], "deps/") {
		t.Errorf("diff lists the included deps:\n%s", diffs["repo"])
	}

	p.Release(s)
	s = acquire(t, p, map[string]string{"repo": first})
	if got := read(t, filepath.Join(repo, "a.txt")); got != "v1\n" {
		t.Errorf("a.txt after a pinned acquire = %q, want the first commit", got)
	}
	for _, gone := range []string{"new.txt", "wip.txt"} {
		if exists(filepath.Join(repo, gone)) {
			t.Errorf("%s survived the reset", gone)
		}
	}
	if exists(filepath.Join(s.Dir, "stray.txt")) {
		t.Error("a stray file at the slot root survived the reset")
	}
	if !exists(filepath.Join(repo, "deps", "lib.txt")) {
		t.Error("the reset removed the included deps")
	}
	if refs := mustRun(t, repo, "git", "for-each-ref", "--format=%(refname)"); refs != "refs/heads/work" {
		t.Errorf("refs after reset = %q, want only the work branch", refs)
	}
}

func TestPool_CopySourceAndKeep(t *testing.T) {
	t.Parallel()
	suite := t.TempDir()
	write(t, filepath.Join(suite, "data", "x.txt"), "x\n")
	write(t, filepath.Join(suite, "data", "sub", "y.txt"), "y\n")
	cfg := workspace.Config{
		Sources: []workspace.Source{{Path: "data", As: "d"}},
		Setup:   "mkdir -p d/cache && echo built > d/cache/out",
		Keep:    []string{"d/cache"},
	}
	p := newPool(t, cfg, suite, 1)
	s := acquire(t, p, nil)
	d := s.Path("d")
	if got := read(t, filepath.Join(d, "cache", "out")); got != "built\n" {
		t.Fatalf("setup output = %q", got)
	}
	os.Remove(filepath.Join(d, "x.txt"))
	write(t, filepath.Join(d, "sub", "y.txt"), "changed\n")
	write(t, filepath.Join(d, "extra.txt"), "extra\n")
	write(t, filepath.Join(d, "cache", "out"), "warm\n")
	p.Release(s)

	s = acquire(t, p, nil)
	if got := read(t, filepath.Join(d, "x.txt")); got != "x\n" {
		t.Errorf("x.txt = %q, want it restored", got)
	}
	if got := read(t, filepath.Join(d, "sub", "y.txt")); got != "y\n" {
		t.Errorf("sub/y.txt = %q, want it restored", got)
	}
	if exists(filepath.Join(d, "extra.txt")) {
		t.Error("extra.txt survived the reset")
	}
	if got := read(t, filepath.Join(d, "cache", "out")); got != "warm\n" {
		t.Errorf("kept cache = %q, want the attempt's content", got)
	}
}

func TestPool_SlotsAreSeparate(t *testing.T) {
	t.Parallel()
	suite := t.TempDir()
	newRepo(t, filepath.Join(suite, "a"))
	write(t, filepath.Join(suite, "b", "f.txt"), "b\n")
	cfg := workspace.Config{Sources: []workspace.Source{{Path: "a"}, {Path: "b"}}}
	p := newPool(t, cfg, suite, 3)
	var slots []*workspace.Slot
	for range 3 {
		slots = append(slots, acquire(t, p, nil))
	}
	for i, s := range slots {
		write(t, filepath.Join(s.Path("a"), "a.txt"), "slot\n")
		write(t, filepath.Join(s.Path("b"), "mine.txt"), string(rune('0'+i)))
	}
	for i, s := range slots {
		for j, other := range slots {
			if i != j && other.Dir == s.Dir {
				t.Fatalf("slots %d and %d share %s", i, j, s.Dir)
			}
		}
		if got := read(t, filepath.Join(s.Path("b"), "mine.txt")); got != string(rune('0'+i)) {
			t.Errorf("slot %d reads %q, another slot's file", i, got)
		}
	}
	if got := read(t, filepath.Join(suite, "a", "a.txt")); got != "v2\n" {
		t.Errorf("the source repo changed: a.txt = %q", got)
	}
	if exists(filepath.Join(suite, "b", "mine.txt")) {
		t.Error("a slot wrote into the source directory")
	}
}

func TestPool_ResetPolicies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		reset         string
		wantInRun     bool // a file left by one attempt is there for the next
		wantAcrossRun bool // and for the next run
	}{
		{"attempt", workspace.ResetAttempt, false, false},
		{"run", workspace.ResetRun, true, false},
		{"never", workspace.ResetNever, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			suite := t.TempDir()
			newRepo(t, filepath.Join(suite, "repo"))
			cfg := workspace.Config{Sources: []workspace.Source{{Path: "repo"}}, Reset: tc.reset}
			root := filepath.Join(t.TempDir(), "slots")
			open := func() *workspace.Pool {
				p, err := workspace.NewPool(t.Context(), cfg, suite, root, 1)
				if err != nil {
					t.Fatalf("NewPool: %v", err)
				}
				return p
			}
			p := open()
			s := acquire(t, p, nil)
			left := filepath.Join(s.Path("repo"), "left.txt")
			write(t, left, "left\n")
			p.Release(s)
			acquire(t, p, nil)
			if got := exists(left); got != tc.wantInRun {
				t.Errorf("left.txt present on the next attempt = %v, want %v", got, tc.wantInRun)
			}
			write(t, left, "left\n")
			acquire(t, open(), nil)
			if got := exists(left); got != tc.wantAcrossRun {
				t.Errorf("left.txt present in the next run = %v, want %v", got, tc.wantAcrossRun)
			}
		})
	}
}

func TestPool_ReusesSlotsUntilConfigChanges(t *testing.T) {
	t.Parallel()
	suite := t.TempDir()
	write(t, filepath.Join(suite, "data", "x.txt"), "x\n")
	root := filepath.Join(t.TempDir(), "slots")
	counter := filepath.Join(t.TempDir(), "setups")
	cfg := workspace.Config{Sources: []workspace.Source{{Path: "data"}}, Setup: "echo x >> " + counter}
	open := func(cfg workspace.Config) {
		p, err := workspace.NewPool(t.Context(), cfg, suite, root, 1)
		if err != nil {
			t.Fatalf("NewPool: %v", err)
		}
		acquire(t, p, nil)
	}
	open(cfg)
	open(cfg)
	if got := strings.Count(read(t, counter), "x"); got != 1 {
		t.Errorf("setup ran %d times for one config, want 1", got)
	}
	cfg.Keep = []string{"data/cache"}
	open(cfg)
	if got := strings.Count(read(t, counter), "x"); got != 2 {
		t.Errorf("setup ran %d times after the config changed, want 2", got)
	}
}

func TestPool_CommandOverrides(t *testing.T) {
	t.Parallel()
	suite := t.TempDir()
	cfg := workspace.Config{
		Provision: `echo made > "$EVALKIT_SLOT/made.txt"`,
		Wipe:      `rm -f "$EVALKIT_SLOT/dirty.txt" && echo wiped >> "$EVALKIT_SLOT/wipes.txt"`,
	}
	p := newPool(t, cfg, suite, 1)
	s := acquire(t, p, nil)
	if got := read(t, filepath.Join(s.Dir, "made.txt")); got != "made\n" {
		t.Fatalf("made.txt = %q", got)
	}
	write(t, filepath.Join(s.Dir, "dirty.txt"), "dirty\n")
	p.Release(s)
	s = acquire(t, p, nil)
	if exists(filepath.Join(s.Dir, "dirty.txt")) {
		t.Error("the wipe command did not run")
	}
	if got := read(t, filepath.Join(s.Dir, "wipes.txt")); got != "wiped\n" {
		t.Errorf("wipes.txt = %q, want one wipe", got)
	}
}

func TestPool_EmptyWorkspaceIsScratch(t *testing.T) {
	t.Parallel()
	p := newPool(t, workspace.Config{}, t.TempDir(), 1)
	s := acquire(t, p, nil)
	write(t, filepath.Join(s.Dir, "out.txt"), "out\n")
	p.Release(s)
	s = acquire(t, p, nil)
	if exists(filepath.Join(s.Dir, "out.txt")) {
		t.Error("a scratch slot kept the last attempt's file")
	}
}

func TestPool_Probe(t *testing.T) {
	t.Parallel()
	suite := t.TempDir()
	newRepo(t, filepath.Join(suite, "repo"))
	p := newPool(t, workspace.Config{Sources: []workspace.Source{{Path: "repo"}}}, suite, 1)
	e, err := p.Probe(t.Context())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if e.Provision <= 0 || e.SlotBytes <= 0 {
		t.Errorf("Probe = %+v, want a provision time and a slot size", e)
	}
}

func TestNewPool_Errors(t *testing.T) {
	t.Parallel()
	suite := t.TempDir()
	write(t, filepath.Join(suite, "plain", "x.txt"), "x\n")
	tests := []struct {
		name string
		cfg  workspace.Config
		want string
	}{
		{"missing source", workspace.Config{Sources: []workspace.Source{{Path: "nope"}}}, "not a directory"},
		{"clone of a plain directory", workspace.Config{Sources: []workspace.Source{{Path: "plain", Method: "clone"}}}, "needs the root of a git repository"},
		{"unknown method", workspace.Config{Sources: []workspace.Source{{Path: "plain", Method: "zip"}}}, "want auto, clone or copy"},
		{"unknown reset", workspace.Config{Reset: "daily"}, "want attempt, run or never"},
		{"two sources with one name", workspace.Config{Sources: []workspace.Source{{Path: "plain"}, {Path: "plain"}}}, "set as"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := workspace.NewPool(t.Context(), tc.cfg, suite, filepath.Join(t.TempDir(), "slots"), 1)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("NewPool error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}
