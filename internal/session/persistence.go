package session

import (
	"encoding/json"
	"fmt"
	"late/internal/client"
	"os"
	"path/filepath"
)

// SaveHistory atomically saves the chat history to the specified path.
func SaveHistory(path string, history []client.ChatMessage) error {
	if path == "" {
		return nil // Skip saving if no path provided
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}

	// Write to a temporary file first
	tmpFile, err := os.CreateTemp(dir, "history-*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name()) // Clean up if something goes wrong before rename

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpFile.Name(), path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// LoadHistory loads the chat history from the specified path.
func LoadHistory(path string) ([]client.ChatMessage, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []client.ChatMessage{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read history file: %w", err)
	}

	var history []client.ChatMessage
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("failed to unmarshal history: %w", err)
	}

	return history, nil
}
