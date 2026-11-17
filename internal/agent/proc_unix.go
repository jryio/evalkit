//go:build unix

package agent

import (
	"os/exec"
	"syscall"
)

// ownGroup starts cmd in its own process group, so everything it spawns can
// be killed with it.
func ownGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
