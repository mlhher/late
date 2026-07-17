package session

import (
	"late/internal/client"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionMeta(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "late-session-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Mock SessionDir
	oldSessionDir := SessionDir
	SessionDir = func() (string, error) {
		return tmpDir, nil
	}
	defer func() { SessionDir = oldSessionDir }()

	historyPath := filepath.Join(tmpDir, "session-test.json")
	history := []client.ChatMessage{{Role: "user", Content: client.TextContent("Hello")}}

	s := New(nil, historyPath, history, "", false)
	meta := s.GenerateSessionMeta()

	if meta.ID != "session-test" {
		t.Errorf("Expected ID 'session-test', got %q", meta.ID)
	}

	if err := SaveSessionMeta(meta); err != nil {
		t.Errorf("Failed to save meta: %v", err)
	}

	// Test exact load
	loaded, err := LoadSessionMeta("session-test")
	if err != nil || loaded == nil {
		t.Fatalf("Failed to load meta exactly: %v", err)
	}
	if loaded.ID != "session-test" {
		t.Errorf("Expected loaded ID 'session-test', got %q", loaded.ID)
	}

	// Test prefix load
	loadedPrefix, err := LoadSessionMeta("session-")
	if err != nil || loadedPrefix == nil {
		t.Fatalf("Failed to load meta by prefix: %v", err)
	}
	if loadedPrefix.ID != "session-test" {
		t.Errorf("Expected loaded prefix ID 'session-test', got %q", loadedPrefix.ID)
	}

	// Test ambiguous prefix
	meta2 := meta
	meta2.ID = "session-other"
	SaveSessionMeta(meta2)

	_, err = LoadSessionMeta("session-")
	if err == nil {
		t.Error("Expected error for ambiguous prefix, got nil")
	}
}

func TestGetLatestSession(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "late-latest-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Mock SessionDir
	oldSessionDir := SessionDir
	SessionDir = func() (string, error) {
		return tmpDir, nil
	}
	defer func() { SessionDir = oldSessionDir }()

	// 1. Test when no sessions exist
	latest, err := GetLatestSession()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if latest != nil {
		t.Errorf("Expected nil latest session when none exist, got %v", latest)
	}

	// 2. Add one session
	meta1 := SessionMeta{
		ID:          "session-1",
		Title:       "First Session",
		LastUpdated: time.Now().Add(-1 * time.Hour),
	}
	if err := SaveSessionMeta(meta1); err != nil {
		t.Fatalf("Failed to save meta1: %v", err)
	}

	latest, err = GetLatestSession()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if latest == nil || latest.ID != "session-1" {
		t.Errorf("Expected latest session to be 'session-1', got %v", latest)
	}

	// 3. Add a second, newer session
	meta2 := SessionMeta{
		ID:          "session-2",
		Title:       "Second Session",
		LastUpdated: time.Now(),
	}
	if err := SaveSessionMeta(meta2); err != nil {
		t.Fatalf("Failed to save meta2: %v", err)
	}

	latest, err = GetLatestSession()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if latest == nil || latest.ID != "session-2" {
		t.Errorf("Expected latest session to be 'session-2', got %v", latest)
	}
}

