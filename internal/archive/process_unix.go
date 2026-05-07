//go:build !windows

package archive

import (
	"os"
	"syscall"
)

// processAlive returns true if the given pid appears to be running.
// Uses kill(pid, 0) which is reliable on Unix/macOS.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
