package tool

import (
	"encoding/json"
	"testing"
)

func TestAnalyzeBashCommand(t *testing.T) {
	st := &ShellTool{}

	tests := []struct {
		desc              string
		command           string
		expectBlocked     bool
		expectConfirm     bool
	}{
		{"Simple ls", "ls", false, false},
		{"Simple grep", "grep foo bar", false, false},
		{"Echo quoted (auto-approve)", "echo \"hello world\"", false, false},
		{"Date (auto-approve)", "date", false, false},
		{"Echo with expansion (confirm)", "echo \"hello $USER\"", false, true},
		{"Blocked cd", "cd /tmp", true, true},
		{"Blocked redirect", "ls > out.txt", true, true},
		{"Blocked append", "echo foo >> bar.txt", true, true},
		{"Blocked redirect with &", "ls &> out.txt", true, true},
		{"Complex pipe (needs confirm)", "ls | grep foo", false, true},
		{"Nested subshell (needs confirm)", "(ls)", false, true},
		{"Command subst (needs confirm)", "echo $(ls)", false, true},
		{"Whitelisted list", "ls; pwd", false, false},
		{"Non-whitelisted", "mkdir foo", false, true},
		{"Combined cd & ls (blocked)", "cd /tmp; ls", true, true},
		{"Quoted cd (blocked)", "'cd' /tmp", true, true},
		{"Variable expansion (needs confirm)", "echo $HOME", false, true},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			blocked, _, confirm := st.analyzeBashCommand(tc.command)
			if blocked != tc.expectBlocked {
				t.Errorf("blocked mismatch: got %v, want %v", blocked, tc.expectBlocked)
			}
			if confirm != tc.expectConfirm {
				t.Errorf("confirm mismatch: got %v, want %v", confirm, tc.expectConfirm)
			}
			
			// Also test RequiresConfirmation with marshaled args
			args, _ := json.Marshal(map[string]string{"command": tc.command})
			if st.RequiresConfirmation(args) != tc.expectConfirm {
				t.Errorf("RequiresConfirmation mismatch: got %v, want %v", st.RequiresConfirmation(args), tc.expectConfirm)
			}
		})
	}
}
