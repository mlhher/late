package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SubagentHistoryDir returns the directory holding a session's subagent
// histories: <sessionsDir>/<sessionID>/subagents. It is created lazily by
// SaveHistory (MkdirAll 0700), so this helper does not create it.
func SubagentHistoryDir(sessionID string) (string, error) {
	sessionsDir, err := SessionDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(sessionsDir, sessionID, "subagents"), nil
}

// SubagentHistoryPath returns the full path of a subagent history file:
// <sessionsDir>/<parentSessionID>/subagents/<childID>.json.
func SubagentHistoryPath(parentSessionID, childID string) (string, error) {
	dir, err := SubagentHistoryDir(parentSessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, childID+".json"), nil
}

// RemoveSessionFolder deletes a session's folder (containing its subagent
// histories) if it exists. Flat files <sessionID>.json / .meta.json are
// distinct names and are never touched. Returns nil when the folder does
// not exist (legacy sessions) or for IDs containing path separators.
func RemoveSessionFolder(sessionID string) error {
	if sessionID == "" || strings.ContainsAny(sessionID, "/\\") {
		return nil
	}
	sessionsDir, err := SessionDir()
	if err != nil {
		return fmt.Errorf("failed to get session directory: %w", err)
	}
	return os.RemoveAll(filepath.Join(sessionsDir, sessionID))
}
