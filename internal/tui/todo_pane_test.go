package tui

import (
	"reflect"
	"testing"
)

func TestWrapTodoText(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		maxLen int
		want   []string
	}{
		{
			name:   "short text fits on single line",
			text:   "Setup Reviewer",
			maxLen: 28,
			want:   []string{"Setup Reviewer"},
		},
		{
			name:   "long text wraps to multiple lines",
			text:   "Setup Implementation Reviewer Subagent Configuration",
			maxLen: 28,
			want: []string{
				"Setup Implementation",
				"Reviewer Subagent",
				"Configuration",
			},
		},
		{
			name:   "empty text",
			text:   "",
			maxLen: 28,
			want:   []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapTodoText(tt.text, tt.maxLen)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("wrapTodoText(%q, %d) = %q, want %q", tt.text, tt.maxLen, got, tt.want)
			}
		})
	}
}
