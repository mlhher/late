package orchestrator

import (
	"context"
	"late/internal/client"
	"late/internal/common"
	"late/internal/session"
	"os"
	"path/filepath"
	"testing"
)

func TestBaseOrchestrator_ResetContextIfCancelledPreservesConfiguration(t *testing.T) {
	o := NewBaseOrchestrator("test-orch", nil, nil, 10)

	ctx, cancel := context.WithCancel(context.WithValue(
		context.Background(), common.SkipConfirmationKey, true,
	))
	cancel()
	o.ctx = ctx

	o.resetContextIfCancelled()

	if err := o.ctx.Err(); err != nil {
		t.Fatalf("reset context is still cancelled: %v", err)
	}
	if skip, ok := o.ctx.Value(common.SkipConfirmationKey).(bool); !ok || !skip {
		t.Fatal("reset context lost SkipConfirmationKey")
	}
}

func TestBaseOrchestrator_Rewind(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "late-orchestrator-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	originalSessionDir := session.SessionDir
	session.SessionDir = func() (string, error) { return tmpDir, nil }
	t.Cleanup(func() { session.SessionDir = originalSessionDir })

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

func TestBaseOrchestrator_ResetStartsNewConversation(t *testing.T) {
	tmpDir := t.TempDir()
	originalSessionDir := session.SessionDir
	session.SessionDir = func() (string, error) { return tmpDir, nil }
	t.Cleanup(func() { session.SessionDir = originalSessionDir })
	originalPath := filepath.Join(tmpDir, "session-original.json")
	history := []client.ChatMessage{
		{Role: "user", Content: client.TextContent("keep me")},
		{Role: "assistant", Content: client.TextContent("preserved")},
	}

	if err := session.SaveHistory(originalPath, history); err != nil {
		t.Fatal(err)
	}

	sess := session.New(nil, originalPath, history, "", false)
	o := NewBaseOrchestrator("test-orch", sess, nil, 10)
	if err := o.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	preserved, err := session.LoadHistory(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(preserved) != 2 || preserved[0].Content.String() != "keep me" {
		t.Fatalf("original history was not preserved: %#v", preserved)
	}
	if len(o.History()) != 0 {
		t.Fatalf("new conversation history length = %d, want 0", len(o.History()))
	}
	if sess.HistoryPath == originalPath {
		t.Fatal("new conversation reused the original history path")
	}
	if filepath.Dir(sess.HistoryPath) != tmpDir {
		t.Fatalf("new history directory = %q, want %q", filepath.Dir(sess.HistoryPath), tmpDir)
	}
	newPath := sess.HistoryPath
	if err := sess.AddUserMessage("new chat"); err != nil {
		t.Fatalf("saving new conversation: %v", err)
	}
	newHistory, err := session.LoadHistory(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(newHistory) != 1 || newHistory[0].Content.String() != "new chat" {
		t.Fatalf("new history was not saved separately: %#v", newHistory)
	}
	preserved, err = session.LoadHistory(originalPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(preserved) != 2 {
		t.Fatalf("saving the new conversation changed original history: %#v", preserved)
	}
}
