//go:build unix

package workspace

import "syscall"

// freeBytes is the space left on the file system that holds dir.
func freeBytes(dir string) int64 {
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}
