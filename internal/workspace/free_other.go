//go:build !unix

package workspace

func freeBytes(string) int64 { return -1 }
