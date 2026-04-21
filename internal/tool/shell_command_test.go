package tool

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewShellCommand(t *testing.T) {
	cmd := newShellCommand(context.Background(), "echo test")

	if runtime.GOOS == "windows" {
		if !strings.EqualFold(filepath.Base(cmd.Path), "cmd.exe") {
			t.Fatalf("expected cmd.Path to end with cmd.exe, got %q", cmd.Path)
		}
		if len(cmd.Args) < 3 {
			t.Fatalf("expected at least 3 args for windows shell command, got %v", cmd.Args)
		}
		if cmd.Args[1] != "/C" {
			t.Fatalf("expected cmd.Args[1] to be /C, got %q", cmd.Args[1])
		}
		if cmd.Args[2] != "echo test" {
			t.Fatalf("expected cmd.Args[2] to be original command, got %q", cmd.Args[2])
		}
		return
	}

	expectedShell := getUnixShellPath()
	if cmd.Path != expectedShell {
		t.Fatalf("expected cmd.Path %q, got %q", expectedShell, cmd.Path)
	}
	if len(cmd.Args) < 3 {
		t.Fatalf("expected at least 3 args for unix shell command, got %v", cmd.Args)
	}
	if cmd.Args[1] != "-c" {
		t.Fatalf("expected cmd.Args[1] to be -c, got %q", cmd.Args[1])
	}
	if cmd.Args[2] != "echo test" {
		t.Fatalf("expected cmd.Args[2] to be original command, got %q", cmd.Args[2])
	}
}
