//go:build unix

package workspace

import (
	"os"
	"syscall"
	"time"
)

// tryLock takes an exclusive lock on path for as long as the returned file
// stays open. ok is false when another process holds it.
//
// A lock is shared for an instant with every child this process is starting,
// from fork until the child's exec closes its copy. A lock that was just
// released can therefore look held, so a refusal is retried a few times.
func tryLock(path string) (f *os.File, ok bool) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, false
	}
	for try := 0; ; try++ {
		if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
			return f, true
		}
		if try == 3 {
			f.Close()
			return nil, false
		}
		time.Sleep(10 * time.Millisecond)
	}
}
