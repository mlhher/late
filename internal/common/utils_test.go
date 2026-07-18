package common

import (
	"late/internal/client"
	"testing"

	"github.com/pkoukk/tiktoken-go"
)

// canonicalTokens returns the true cl100k_base token count for text, used as
// the ground truth the package's EstimateTokenCount must match.
func canonicalTokens(t *testing.T, text string) int {
	t.Helper()
	if text == "" {
		return 0
	}
	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		t.Fatalf("failed to load canonical encoder: %v", err)
	}
	return len(enc.Encode(text, nil, nil))
}

func TestReplacePlaceholders(t *testing.T) {
	tests := []struct {
		text         string
		placeholders map[string]string
		expected     string
	}{
		{
			text:         "Hello ${{CWD}}",
			placeholders: map[string]string{"${{CWD}}": "/tmp"},
			expected:     "Hello /tmp",
		},
		{
			text:         "No placeholder here",
			placeholders: map[string]string{"${{CWD}}": "/tmp"},
			expected:     "No placeholder here",
		},
		{
			text:         "Multiple ${{CWD}} in ${{CWD}}",
			placeholders: map[string]string{"${{CWD}}": "/home"},
			expected:     "Multiple /home in /home",
		},
	}

	for _, tt := range tests {
		result := ReplacePlaceholders(tt.text, tt.placeholders)
		if result != tt.expected {
			t.Errorf("ReplacePlaceholders(%q, %v) = %q; want %q", tt.text, tt.placeholders, result, tt.expected)
		}
	}
}

func TestEstimateTokenCount(t *testing.T) {
	tests := []string{
		"",
		"a",
		"abcd",
		"12345678",
		"this is a test",
		"Hello, world!",
		"def main():\n    return 42",
		"The quick brown fox jumps over the lazy dog.",
	}

	for _, tt := range tests {
		got := EstimateTokenCount(tt)
		want := canonicalTokens(t, tt)
		if got != want {
			t.Errorf("EstimateTokenCount(%q) = %d; want %d", tt, got, want)
		}
	}
}

func TestEstimateMessageTokens(t *testing.T) {
	msg := client.ChatMessage{
		Role:             "assistant",
		Content:          client.TextContent("Hello"),
		ReasoningContent: "Thinking...",
		ToolCalls: []client.ToolCall{
			{
				Function: client.FunctionCall{
					Name:      "test_tool",
					Arguments: `{"arg1": "val1"}`,
				},
			},
		},
	}

	// Expected = sum of real BPE counts for each text field + 4 msg overhead.
	want := EstimateTokenCount("Hello") +
		EstimateTokenCount("Thinking...") +
		EstimateTokenCount("test_tool") +
		EstimateTokenCount(`{"arg1": "val1"}`) + 4

	if got := EstimateMessageTokens(msg); got != want {
		t.Errorf("EstimateMessageTokens() = %d; want %d", got, want)
	}
}

func TestEstimateEventTokens(t *testing.T) {
	event := ContentEvent{
		Content:          "Part1",
		ReasoningContent: "Reason",
	}

	want := EstimateTokenCount("Part1") + EstimateTokenCount("Reason")
	if got := EstimateEventTokens(event); got != want {
		t.Errorf("EstimateEventTokens() = %d; want %d", got, want)
	}
}

func TestCalculateHistoryTokens(t *testing.T) {
	tests := []struct {
		name         string
		history      []client.ChatMessage
		systemPrompt string
		tools        []client.ToolDefinition
		want         int
	}{
		{
			name:         "empty history with system prompt",
			history:      []client.ChatMessage{},
			systemPrompt: "You are an assistant",
			tools:        nil,
			want:         EstimateTokenCount("You are an assistant") + 10,
		},
		{
			name: "single user message",
			history: []client.ChatMessage{
				{
					Role:    "user",
					Content: client.TextContent("Hello"),
				},
			},
			systemPrompt: "",
			tools:        nil,
			want:         10 + EstimateMessageTokens(client.ChatMessage{Role: "user", Content: client.TextContent("Hello")}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateHistoryTokens(tt.history, tt.systemPrompt, tt.tools)
			if got != tt.want {
				t.Errorf("CalculateHistoryTokens() = %d; want %d", got, tt.want)
			}
		})
	}
}
