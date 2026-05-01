package mcp

import (
	"context"
	"testing"
)

func TestIsAllowedMCPCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "allows python", command: "python", want: true},
		{name: "allows absolute python path", command: "/usr/bin/python3", want: true},
		{name: "allows node", command: "node", want: true},
		{name: "rejects shell", command: "sh", want: false},
		{name: "rejects empty", command: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAllowedMCPCommand(tt.command)
			if got != tt.want {
				t.Fatalf("isAllowedMCPCommand(%q)=%v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

func TestNewStdioTransportRejectsDisallowedCommand(t *testing.T) {
	_, err := NewStdioTransport(context.Background(), "sh", []string{"-c", "echo unsafe"}, nil)
	if err == nil {
		t.Fatal("expected error for disallowed command")
	}
}
