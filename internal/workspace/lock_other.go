//go:build !unix

package workspace

import "os"

// tryLock has no cross-process lock here: one run per suite at a time.
func tryLock(path string) (*os.File, bool) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	return f, err == nil
}
