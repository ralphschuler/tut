//go:build unix

package main

import (
	"os"
	"syscall"
)

// createFIFO creates a named pipe (FIFO) at the specified path.
// It removes any existing file at that path first.
func createFIFO(path string) error {
	_ = os.Remove(path)
	return syscall.Mkfifo(path, 0o600)
}
