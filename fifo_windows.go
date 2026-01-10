//go:build windows

package main

import "fmt"

// createFIFO is not supported on Windows.
// Named pipes (FIFOs) are a Unix-specific feature.
func createFIFO(path string) error {
	return fmt.Errorf("FIFO creation is not supported on Windows")
}
