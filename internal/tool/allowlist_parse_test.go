package tool

import (
	"testing"
)

func TestParseCommandsForAllowList(t *testing.T) {
	tests := []struct {
		command string
		want    map[string][]string
	}{
		{
			"go mod tidy && go test -v ./...",
			map[string][]string{
				"go mod":  {"tidy"},
				"go test": {"-v"},
			},
		},
		{
			"git log --oneline --output=test.txt | grep foo",
			map[string][]string{
				"git log": {"--oneline", "--output"},
				"grep":    {},
			},
		},
	}

	for _, tc := range tests {
		got := ParseCommandsForAllowList(tc.command)
		if len(got) != len(tc.want) {
			t.Errorf("ParseCommandsForAllowList(%q): length mismatch: got %d, want %d", tc.command, len(got), len(tc.want))
			continue
		}
		for key, wantFlags := range tc.want {
			gotFlags, ok := got[key]
			if !ok {
				t.Errorf("ParseCommandsForAllowList(%q): missing key %q", tc.command, key)
				continue
			}
			if len(gotFlags) != len(wantFlags) {
				t.Errorf("ParseCommandsForAllowList(%q): key %q: flags length mismatch: got %d, want %d", tc.command, key, len(gotFlags), len(wantFlags))
				continue
			}
			for i, f := range wantFlags {
				if gotFlags[i] != f {
					t.Errorf("ParseCommandsForAllowList(%q): key %q: flag mismatch at %d: got %q, want %q", tc.command, key, i, gotFlags[i], f)
				}
			}
		}
	}
}
