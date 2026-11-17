package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jryio/evalkit/internal/proc"
)

// commandLimit is the longest any one workspace command may take: a clone, a
// copy, a setup. A command that hangs must fail the attempt, not the run.
const commandLimit = 30 * time.Minute

// limited bounds ctx by commandLimit. The timer is released when ctx ends.
func limited(ctx context.Context) context.Context {
	ctx, cancel := context.WithTimeout(ctx, commandLimit)
	context.AfterFunc(ctx, cancel)
	return ctx
}

// run runs a command and returns its standard output. A failure carries the
// end of what the command printed.
func run(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	out, err := proc.Run(limited(ctx), append([]string{name}, args...), nil, dir, env)
	if err != nil {
		tail := strings.TrimSpace(string(out.Stderr) + string(out.Stdout))
		if len(tail) > 600 {
			tail = "…" + tail[len(tail)-600:]
		}
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, tail)
	}
	return string(out.Stdout), nil
}

// gitEnv is the environment of every git command evalkit runs in a slot. The
// user's own git config stays out: no signing, hooks, credential helpers or
// background maintenance, so a slot's git behaves the same on every machine.
func gitEnv() []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=gc.auto", "GIT_CONFIG_VALUE_0=0",
		"GIT_CONFIG_KEY_1=maintenance.auto", "GIT_CONFIG_VALUE_1=false")
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := run(ctx, dir, gitEnv(), "git", args...)
	return strings.TrimSpace(out), err
}

func isGitRoot(ctx context.Context, dir string) bool {
	top, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return false
	}
	a, errA := filepath.EvalSymlinks(top)
	b, errB := filepath.EvalSymlinks(dir)
	return errA == nil && errB == nil && a == b
}

func revParse(ctx context.Context, dir, ref string) (string, error) {
	return git(ctx, dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
}

// cloneAt makes dst a clone of src that shares its objects, checked out at sha
// on a branch named work, with no remote, tags or other branches to reach
// later commits through.
func cloneAt(ctx context.Context, src, dst, sha string) error {
	if _, err := git(ctx, "", "clone", "-q", "--shared", "--no-checkout", src, dst); err != nil {
		return err
	}
	if _, err := git(ctx, dst, "checkout", "-q", "-B", "work", sha); err != nil {
		return err
	}
	if _, err := git(ctx, dst, "remote", "remove", "origin"); err != nil {
		return err
	}
	return dropRefs(ctx, dst)
}

// dropRefs deletes every ref but the work branch.
func dropRefs(ctx context.Context, dir string) error {
	refs, err := git(ctx, dir, "for-each-ref", "--format=%(refname)")
	if err != nil {
		return err
	}
	for ref := range strings.SplitSeq(refs, "\n") {
		if ref == "" || ref == "refs/heads/work" {
			continue
		}
		if _, err := git(ctx, dir, "update-ref", "-d", ref); err != nil {
			return err
		}
	}
	return nil
}

// resetClone puts a clone back on sha with a clean tree, leaving the keep paths.
func resetClone(ctx context.Context, dir, sha string, keep []string) error {
	if len(keep) > 0 {
		// An attempt may have committed a keep path; untracked, the checkout leaves it on disk.
		args := append([]string{"rm", "-r", "-q", "--cached", "--ignore-unmatch", "--"}, keep...)
		if _, err := git(ctx, dir, args...); err != nil {
			return err
		}
	}
	if _, err := git(ctx, dir, "checkout", "-q", "-f", "-B", "work", sha); err != nil {
		return err
	}
	if err := dropRefs(ctx, dir); err != nil {
		return err
	}
	args := []string{"clean", "-ffdxq"}
	for _, k := range keep {
		args = append(args, "-e", "/"+strings.Trim(filepath.ToSlash(k), "/"))
	}
	_, err := git(ctx, dir, args...)
	return err
}

// diffSince is the tree's difference from sha: commits, edits and new files.
// It only reads the repository: the index stays as the attempt left it, so
// the stages that run next see the agent's own state.
func diffSince(ctx context.Context, dir, sha string, keep []string) (string, error) {
	var paths []string
	for _, k := range keep {
		paths = append(paths, ":(exclude)"+strings.Trim(filepath.ToSlash(k), "/"))
	}
	scope := append([]string{"--", "."}, paths...)
	diff, err := run(ctx, dir, gitEnv(), "git", append([]string{"-c", "core.quotepath=false", "diff", "--no-color", "--no-ext-diff", sha}, scope...)...)
	if err != nil {
		return "", err
	}
	// Files git does not track yet are not in that diff: each is added as a new file.
	untracked, err := run(ctx, dir, gitEnv(), "git", append([]string{"ls-files", "--others", "--exclude-standard", "-z"}, scope...)...)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(diff)
	for file := range strings.SplitSeq(untracked, "\x00") {
		if file == "" {
			continue
		}
		// diff --no-index exits 1 when the two sides differ, which they always do here.
		out, err := proc.Run(limited(ctx), []string{"git", "-c", "core.quotepath=false", "diff", "--no-color", "--no-ext-diff", "--no-index", "--", os.DevNull, file}, nil, dir, gitEnv())
		if exit, ok := errors.AsType[*exec.ExitError](err); err != nil && (!ok || exit.ExitCode() != 1) {
			return "", fmt.Errorf("git diff --no-index %s: %w: %s", file, err, strings.TrimSpace(string(out.Stderr)))
		}
		b.Write(out.Stdout)
	}
	return b.String(), nil
}

// copyTree copies src to dst, which must not exist, with copy-on-write where
// the file system has it.
func copyTree(ctx context.Context, src, dst string) error {
	var attempts [][]string
	switch runtime.GOOS {
	case "darwin":
		attempts = [][]string{{"-cRp"}, {"-Rp"}}
	case "linux":
		attempts = [][]string{{"-a", "--reflink=auto"}, {"-a"}}
	default:
		return os.CopyFS(dst, os.DirFS(src))
	}
	var err error
	for _, flags := range attempts {
		if _, err = run(ctx, "", os.Environ(), "cp", append(flags, src, dst)...); err == nil {
			return nil
		}
		os.RemoveAll(dst)
	}
	return err
}

// syncTree makes dst match src again, leaving the keep paths.
func syncTree(ctx context.Context, src, dst string, keep []string) error {
	if _, err := exec.LookPath("rsync"); err != nil {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		return copyTree(ctx, src, dst)
	}
	args := []string{"-a", "--delete"}
	for _, k := range keep {
		args = append(args, "--exclude=/"+strings.Trim(filepath.ToSlash(k), "/"))
	}
	_, err := run(ctx, "", os.Environ(), "rsync", append(args, src+"/", dst+"/")...)
	return err
}

// diskUsage is the size of dir in bytes, or 0 when du cannot say.
func diskUsage(ctx context.Context, dir string) int64 {
	out, err := run(ctx, "", os.Environ(), "du", "-sk", dir)
	if err != nil {
		return 0
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return 0
	}
	kb, _ := strconv.ParseInt(fields[0], 10, 64)
	return kb * 1024
}
