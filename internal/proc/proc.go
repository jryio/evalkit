// Package proc runs child processes for a benchmark: each in its own process
// group, killed whole when its context ends, and never waited on forever.
package proc

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ErrTimeout reports that the context's deadline ended the command.
var ErrTimeout = errors.New("timed out")

// Output is what a command printed and how long it ran.
type Output struct {
	Stdout  []byte
	Stderr  []byte
	Seconds float64
}

// Run runs argv to its end or to ctx's end, whichever is first, and then kills
// whatever the command left running. The error is nil on exit 0, ErrTimeout
// when the deadline passed, ctx's error when it was cancelled, and an
// *exec.ExitError or a start error otherwise. A nil env is this process's.
//
// The command reads and writes plain files, not pipes: a child that outlives
// it and keeps its output open then cannot hold Run back, so Run returns as
// soon as the command itself exits.
func Run(ctx context.Context, argv []string, stdin []byte, dir string, env []string) (Output, error) {
	scratch, err := os.MkdirTemp("", "evalkit-proc-*")
	if err != nil {
		return Output{}, err
	}
	defer os.RemoveAll(scratch)
	stdout, err := os.Create(filepath.Join(scratch, "stdout"))
	if err != nil {
		return Output{}, err
	}
	defer stdout.Close()
	stderr, err := os.Create(filepath.Join(scratch, "stderr"))
	if err != nil {
		return Output{}, err
	}
	defer stderr.Close()

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if len(stdin) > 0 {
		in := filepath.Join(scratch, "stdin")
		if err := os.WriteFile(in, stdin, 0o600); err != nil {
			return Output{}, err
		}
		f, err := os.Open(in)
		if err != nil {
			return Output{}, err
		}
		defer f.Close()
		cmd.Stdin = f
	}
	ownGroup(cmd)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return Output{}, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		killGroup(cmd)
		<-done
		err = ctx.Err()
	}
	killGroup(cmd) // children left running (a dev server, a watcher) end with the command
	out := Output{Seconds: time.Since(start).Seconds()}
	out.Stdout, _ = os.ReadFile(stdout.Name())
	out.Stderr, _ = os.ReadFile(stderr.Name())
	if errors.Is(err, context.DeadlineExceeded) {
		return out, ErrTimeout
	}
	return out, err
}
