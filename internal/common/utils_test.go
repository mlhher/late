package common

import (
	"late/internal/client"
	"testing"
)

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
	tests := []struct {
		text     string
		expected int
	}{
		{"", 0},
		{"a", 1},    // (1+3)/4 = 1
		{"abcd", 1}, // (4+3)/4 = 1
		{"abcde", 2}, // (5+3)/4 = 2 (rounding up)
		{"12345678", 2}, // (8+3)/4 = 2
		{"123456789", 3}, // (9+3)/4 = 3
	}

	for _, tt := range tests {
		result := EstimateTokenCount(tt.text)
		if result != tt.expected {
			t.Errorf("EstimateTokenCount(%q) = %d; want %d", tt.text, result, tt.expected)
		}
	}
}

func TestEstimateMessageTokens(t *testing.T) {
	msg := client.ChatMessage{
		Role:             "assistant",
		Content:          "Hello",
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

	// "Hello" = 5 chars -> 2 tokens
	// "Thinking..." = 11 chars -> 3 tokens
	// "test_tool" = 9 chars -> 3 tokens
	// `{"arg1": "val1"}` = 16 chars -> 4 tokens
	// Total = 2 + 3 + 3 + 4 = 12 tokens
	expected := 12
	result := EstimateMessageTokens(msg)
	if result != expected {
		t.Errorf("EstimateMessageTokens() = %d; want %d", result, expected)
	}
}

func TestEstimateEventTokens(t *testing.T) {
	event := ContentEvent{
		Content:          "Part1",
		ReasoningContent: "Reason",
		ToolCalls: []client.ToolCall{
			{
				Function: client.FunctionCall{
					Name:      "tool",
					Arguments: "{}",
				},
			},
		},
	}

	// "Part1" = 5 chars -> 2 tokens
	// "Reason" = 6 chars -> 2 tokens
	// "tool" = 4 chars -> 1 token
	// "{}" = 2 chars -> 1 token
	// Total = 2 + 2 + 1 + 1 = 6 tokens
	expected := 6
	result := EstimateEventTokens(event)
	if result != expected {
		t.Errorf("EstimateEventTokens() = %d; want %d", result, expected)
	}
}
