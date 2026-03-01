package tool

import (
	"os"
	"path/filepath"
	"strings"
)

// IsSafePath checks if a path is within the current working directory.
func IsSafePath(path string) bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Normalize both paths to prevent symlink traversal
	absCwd, _ := filepath.Abs(cwd)
	return strings.HasPrefix(absPath, absCwd)
}
