package common

import (
	"late/internal/client"
	"strings"
)

// ReplacePlaceholders replaces all occurrences of placeholders with their values.
func ReplacePlaceholders(text string, placeholders map[string]string) string {
	for p, v := range placeholders {
		text = strings.ReplaceAll(text, p, v)
	}
	return text
}

// EstimateTokenCount estimates the number of tokens in text.
// Uses a simple approximation: 1 token ≈ 4 characters.
func EstimateTokenCount(text string) int {
	if text == "" {
		return 0
	}
	// A common rule of thumb for English and code: 4 characters per token.
	return (len(text) + 3) / 4
}

// EstimateMessageTokens estimates tokens for a full chat message including tool calls.
func EstimateMessageTokens(msg client.ChatMessage) int {
	tokens := EstimateTokenCount(msg.Content) + EstimateTokenCount(msg.ReasoningContent)
	for _, tc := range msg.ToolCalls {
		tokens += EstimateTokenCount(tc.Function.Name) + EstimateTokenCount(tc.Function.Arguments)
	}
	return tokens
}

// EstimateEventTokens estimates tokens for a content event, including streamed tool calls.
func EstimateEventTokens(event ContentEvent) int {
	tokens := EstimateTokenCount(event.Content) + EstimateTokenCount(event.ReasoningContent)
	for _, tc := range event.ToolCalls {
		tokens += EstimateTokenCount(tc.Function.Name) + EstimateTokenCount(tc.Function.Arguments)
	}
	return tokens
}
