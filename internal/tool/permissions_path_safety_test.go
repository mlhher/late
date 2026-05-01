package tool

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsSafePathRejectsRelativeSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation can require elevated privileges on Windows")
	}

	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(origCwd)
	}()

	projectRoot := t.TempDir()
	outsideRoot := t.TempDir()

	if err := os.MkdirAll(filepath.Join(projectRoot, "safe"), 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(outsideRoot, "escape"), 0755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}

	if err := os.Symlink(filepath.Join(outsideRoot, "escape"), filepath.Join(projectRoot, "safe", "link")); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("failed to switch cwd: %v", err)
	}

	if IsSafePath(filepath.Join("safe", "link", "secret.txt")) {
		t.Fatalf("expected relative symlink traversal path to be unsafe")
	}
}

func TestIsSafePathAllowsNormalProjectRelativePath(t *testing.T) {
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(origCwd)
	}()

	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, "safe"), 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf("failed to switch cwd: %v", err)
	}

	if !IsSafePath(filepath.Join("safe", "file.txt")) {
		t.Fatalf("expected normal in-project relative path to be safe")
	}
}
