//go:build windows

package archive

// processAlive on Windows cannot reliably check process liveness via signals
// (syscall.Signal is unsupported). Returning true treats any non-stale lock as
// held, which is safe: the StaleAfterSeconds mechanism handles recovery if the
// owner process has genuinely died.
func processAlive(_ int) bool {
	return true
}
