package mcp

import (
	"context"
	"reflect"
	"testing"
)

func TestIsAllowedMCPCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "allows python", command: "python", want: true},
		{name: "allows uppercase bare command", command: "Python", want: true},
		{name: "allows absolute python path", command: "/usr/bin/python3", want: true},
		{name: "allows windows executable suffix", command: "python.exe", want: true},
		{name: "allows windows script suffix", command: "npx.cmd", want: true},
		{name: "allows node", command: "node", want: true},
		{name: "rejects relative path with separator", command: "./node", want: false},
		{name: "rejects nested relative path", command: "bin/python", want: false},
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

func TestServerEnvToList(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{name: "nil map", env: nil, want: nil},
		{name: "empty map", env: map[string]string{}, want: nil},
		{
			name: "sorted key value pairs",
			env: map[string]string{
				"Z_KEY": "three",
				"A_KEY": "one",
				"M_KEY": "two",
			},
			want: []string{"A_KEY=one", "M_KEY=two", "Z_KEY=three"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serverEnvToList(tt.env)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("serverEnvToList()=%v, want %v", got, tt.want)
			}
		})
	}
}
