//go:build unix

package proc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func sh(script string) []string { return []string{"sh", "-c", script} }

func TestRun_Outcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		argv       []string
		stdin      string
		env        []string
		wantStdout string
		wantStderr string
		wantExit   int // -1: no exit error expected
		wantStart  bool
	}{
		{"exit 0 with output", sh("echo out; echo err >&2"), "", nil, "out\n", "err\n", -1, false},
		{"stdin reaches the command", sh("cat"), "typed", nil, "typed", "", -1, false},
		{"the environment is the caller's", sh(`printf %s "$ONLY"`), "", []string{"ONLY=this", "PATH=" + os.Getenv("PATH")}, "this", "", -1, false},
		{"a non-zero exit is an exit error", sh("echo partial; exit 3"), "", nil, "partial\n", "", 3, false},
		{"a missing program does not start", []string{"/no/such/program"}, "", nil, "", "", -1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := Run(t.Context(), tc.argv, []byte(tc.stdin), t.TempDir(), tc.env)
			if string(out.Stdout) != tc.wantStdout || string(out.Stderr) != tc.wantStderr {
				t.Errorf("stdout %q stderr %q, want %q and %q", out.Stdout, out.Stderr, tc.wantStdout, tc.wantStderr)
			}
			exit, isExit := errors.AsType[*exec.ExitError](err)
			switch {
			case tc.wantStart:
				if err == nil || isExit {
					t.Errorf("error = %v, want a start error", err)
				}
			case tc.wantExit >= 0:
				if !isExit || exit.ExitCode() != tc.wantExit {
					t.Errorf("error = %v, want exit %d", err, tc.wantExit)
				}
			default:
				if err != nil {
					t.Errorf("error = %v, want none", err)
				}
			}
		})
	}
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func readPID(tb testing.TB, path string) int {
	tb.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if b, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				return pid
			}
		}
	}
	tb.Fatalf("no pid in %s", path)
	return 0
}

func dies(pid int) bool {
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if !alive(pid) {
			return true
		}
	}
	syscall.Kill(pid, syscall.SIGKILL)
	return false
}

func TestRun_DeadlineKillsTheWholeGroup(t *testing.T) {
	t.Parallel()
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Run(ctx, sh("sleep 60 & echo $! > "+pidFile+"; wait"), nil, "", nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("error = %v, want ErrTimeout", err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("Run returned after %v, want soon after the 300ms deadline", took)
	}
	if !dies(readPID(t, pidFile)) {
		t.Error("the command's child outlived the deadline")
	}
}

func TestRun_CancelIsNotATimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(200*time.Millisecond, cancel)
	if _, err := Run(ctx, sh("sleep 60"), nil, "", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

// A command that exits and leaves a child holding its output must neither hang
// the caller nor leave the child running.
func TestRun_LeftoverChildWithOpenPipes(t *testing.T) {
	t.Parallel()
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	start := time.Now()
	out, err := Run(t.Context(), sh("sleep 60 & echo $! > "+pidFile+"; echo done"), nil, "", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(string(out.Stdout)) != "done" {
		t.Errorf("stdout = %q, want done", out.Stdout)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("Run returned after %v, want it back as soon as the command exited", took)
	}
	if !dies(readPID(t, pidFile)) {
		t.Error("the leftover child is still running")
	}
}
