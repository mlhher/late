package orchestrator

import (
	"late/internal/client"
	"late/internal/session"
	"os"
	"path/filepath"
	"testing"
)

func TestBaseOrchestrator_Rewind(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "late-orchestrator-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	historyPath := filepath.Join(tmpDir, "history.json")
	history := []client.ChatMessage{
		{Role: "user", Content: client.TextContent("Msg 1")},
		{Role: "assistant", Content: client.TextContent("Reply 1")},
		{Role: "user", Content: client.TextContent("Msg 2")},
		{Role: "assistant", Content: client.TextContent("Reply 2")},
	}

	c := client.NewClient(client.Config{BaseURL: "http://localhost:8080"})
	sess := session.New(c, historyPath, history, "", false)
	o := NewBaseOrchestrator("test-orch", sess, nil, 10)

	// Test invalid rewind index
	if err := o.Rewind(-1); err == nil {
		t.Error("Expected error for negative index, got nil")
	}
	if err := o.Rewind(5); err == nil {
		t.Error("Expected error for out-of-bounds index, got nil")
	}

	// Rewind to index 2 (Msg 2)
	// After rewinding to index 2, history should contain index 0 and 1: Msg 1 and Reply 1.
	if err := o.Rewind(2); err != nil {
		t.Fatalf("Failed to rewind: %v", err)
	}

	updatedHistory := o.History()
	if len(updatedHistory) != 2 {
		t.Fatalf("Expected history length 2, got %d", len(updatedHistory))
	}
	if updatedHistory[0].Content.String() != "Msg 1" {
		t.Errorf("Expected first message 'Msg 1', got %q", updatedHistory[0].Content.String())
	}
	if updatedHistory[1].Content.String() != "Reply 1" {
		t.Errorf("Expected second message 'Reply 1', got %q", updatedHistory[1].Content.String())
	}
}
