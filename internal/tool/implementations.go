package tool

import (
	"context"
	"encoding/json" // used for hash generation
	"fmt"
	"late/internal/common"
	"os"
	"os/exec"
	"strings"
	"time"
)

// WeatherTool returns simulated weather.
type WeatherTool struct{}

func (t WeatherTool) Name() string        { return "get_weather" }
func (t WeatherTool) Description() string { return "Get the current weather for a location" }
func (t WeatherTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"location": { "type": "string", "description": "The city and state, e.g. San Francisco, CA" }
		},
		"required": ["location"]
	}`)
}
func (t WeatherTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Location string `json:"location"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}
	return fmt.Sprintf("The weather in %s is currently 72°F and sunny.", params.Location), nil
}
func (t WeatherTool) RequiresConfirmation(args json.RawMessage) bool { return false }

// ReadFileTool reads content from a file.
type ReadFileTool struct {
	LastReads map[string]ReadState
}

type ReadState struct {
	ModTime    time.Time
	Size       int64
	LastParams string
}

func NewReadFileTool() *ReadFileTool {
	return &ReadFileTool{
		LastReads: make(map[string]ReadState),
	}
}

func (t *ReadFileTool) Name() string        { return "read_file" }
func (t *ReadFileTool) Description() string { return "Read the content of a file" }
func (t *ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Path to the file to read" },
			"start_line": { "type": "integer", "description": "Optional: Start reading from this line number (1-indexed)" },
			"end_line": { "type": "integer", "description": "Optional: Stop reading at this line number (inclusive)" }
		},
		"required": ["path"]
	}`)
}
func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	// info, err := os.Stat(params.Path)
	// if err != nil {
	// 	return "", err
	// }

	// Check for unchanged key - DISABLED per user request
	// paramsJson, _ := json.Marshal(params)
	// paramsStr := string(paramsJson)

	// if state, ok := t.LastReads[params.Path]; ok {
	// 	if state.ModTime.Equal(info.ModTime()) && state.Size == info.Size() && state.LastParams == paramsStr {
	// 		return "File has not changed since last read with these parameters.", nil
	// 	}
	// }

	// Update state - DISABLED
	// t.LastReads[params.Path] = ReadState{
	// 	ModTime:    info.ModTime(),
	// 	Size:       info.Size(),
	// 	LastParams: paramsStr,
	// }

	data, err := os.ReadFile(params.Path)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	totalLines := len(lines)

	type lineInfo struct {
		lineNum int
		content string
	}
	fileLines := make([]lineInfo, totalLines)
	for i, line := range lines {
		fileLines[i] = lineInfo{
			lineNum: i + 1,
			content: line,
		}
	}

	start := 1
	end := totalLines

	if params.StartLine > 0 {
		start = params.StartLine
	}
	if params.EndLine > 0 {
		end = params.EndLine
	}

	if start < 1 {
		start = 1
	}
	if end > totalLines {
		end = totalLines
	}
	if start > end {
		return fmt.Sprintf("Invalid range: start_line %d > end_line %d (total: %d)", start, end, totalLines), nil
	}

	result := fileLines[start-1 : end]

	var sb strings.Builder
	for _, l := range result {
		sb.WriteString(fmt.Sprintf("%d | %s\n", l.lineNum, l.content))
	}

	return sb.String(), nil
}
func (t *ReadFileTool) RequiresConfirmation(args json.RawMessage) bool { return false }

func (t *ReadFileTool) CallString(args json.RawMessage) string {
	path := getToolParam(args, "path")
	if cwd, err := os.Getwd(); err == nil {
		path = strings.Replace(path, cwd, ".", 1)
	}
	return fmt.Sprintf("Reading file %s", truncate(path, 50))
}

// UpdateFileTool updates a file using a Search/Replace strategy.
type UpdateFileTool struct{}

func (t UpdateFileTool) Name() string        { return "update_file" }
func (t UpdateFileTool) Description() string { return "Update a file by replacing text blocks" }
func (t UpdateFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Path to the file to update" },
			"edits": { 
				"type": "string", 
				"description": "A string containing text replacement blocks. Use this format:\n<<<<\n[Exact content to find]\n====\n[New content to insert]\n>>>>\n\nYou can provide multiple blocks. The 'content to find' must EXACTLY match the existing file content (including whitespace/indentation) and be unique." 
			}
		},
		"required": ["path", "edits"]
	}`)
}

func (t UpdateFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path  string `json:"path"`
		Edits string `json:"edits"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	data, err := os.ReadFile(params.Path)
	if err != nil {
		return "", err
	}
	content := string(data)

	// Simple parser for custom block format
	// <<<<
	// SEARCH
	// ====
	// REPLACE
	// >>>>

	lines := strings.Split(params.Edits, "\n")
	var searchBuilder, replaceBuilder strings.Builder
	inSearch := false
	inReplace := false

	// We will collect edits and apply them.
	// To avoid overlapping issues or complex state, applying them sequentially to the 'content' string
	// is the most robust way to handle multiple patches in one go.

	updates := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "<<<<" && !inSearch && !inReplace {
			inSearch = true
			searchBuilder.Reset()
			continue
		}

		if trimmed == "====" && inSearch {
			inSearch = false
			inReplace = true
			replaceBuilder.Reset()
			continue
		}

		if trimmed == ">>>>" && inReplace {
			inReplace = false

			// Process the block
			searchStr := searchBuilder.String()
			replaceStr := replaceBuilder.String()

			// Remove the trailing newline added by the loop
			if len(searchStr) > 0 && searchStr[len(searchStr)-1] == '\n' {
				searchStr = searchStr[:len(searchStr)-1]
			}
			if len(replaceStr) > 0 && replaceStr[len(replaceStr)-1] == '\n' {
				replaceStr = replaceStr[:len(replaceStr)-1]
			}

			// Validate uniqueness
			count := strings.Count(content, searchStr)
			if count == 0 {
				return "", fmt.Errorf("search block not found in file:\n%s", searchStr)
			}
			if count > 1 {
				return "", fmt.Errorf("search block matches %d times, must be unique:\n%s", count, searchStr)
			}

			// Apply replacement
			content = strings.Replace(content, searchStr, replaceStr, 1)
			updates++
			continue
		}

		if inSearch {
			searchBuilder.WriteString(line + "\n")
		} else if inReplace {
			replaceBuilder.WriteString(line + "\n")
		}
		// Text outside blocks is ignored (allows for comments/explanations if the model adds them)
	}

	if inSearch || inReplace {
		return "", fmt.Errorf("incomplete block detected")
	}

	if err := os.WriteFile(params.Path, []byte(content), 0644); err != nil {
		return "", err
	}

	return fmt.Sprintf("Successfully applied %d updates to %s", updates, params.Path), nil
}
func (t UpdateFileTool) RequiresConfirmation(args json.RawMessage) bool {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return true
	}
	return !IsSafePath(params.Path)
}

func (t UpdateFileTool) CallString(args json.RawMessage) string {
	path := getToolParam(args, "path")
	if cwd, err := os.Getwd(); err == nil {
		path = strings.Replace(path, cwd, ".", 1)
	}
	return fmt.Sprintf("Updating file %s", truncate(path, 50))
}

// WriteFileTool writes content to a file.
type WriteFileTool struct{}

func (t WriteFileTool) Name() string { return "write_file" }
func (t WriteFileTool) Description() string {
	return "Write content to a file. Requires confirmation if writing outside CWD."
}
func (t WriteFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Path to the file to write" },
			"content": { "type": "string", "description": "Content to write to the file" }
		},
		"required": ["path", "content"]
	}`)
}
func (t WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	if params.Content == "" {
		return "", fmt.Errorf("Your edit to %s failed: content cannot be empty", params.Path)
	}
	if err := os.WriteFile(params.Path, []byte(params.Content), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully wrote to %s", params.Path), nil
}
func (t WriteFileTool) RequiresConfirmation(args json.RawMessage) bool {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return true // Default to safe if we can't parse
	}
	return !IsSafePath(params.Path)
}

func (t WriteFileTool) CallString(args json.RawMessage) string {
	path := getToolParam(args, "path")
	if cwd, err := os.Getwd(); err == nil {
		path = strings.Replace(path, cwd, ".", 1)
	}
	return fmt.Sprintf("Writing to file %s", truncate(path, 50))
}

// ListDirTool lists contents of a directory.
type ListDirTool struct{}

func (t ListDirTool) Name() string        { return "list_dir" }
func (t ListDirTool) Description() string { return "List the contents of a directory" }
func (t ListDirTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Path to the directory to list" }
		},
		"required": ["path"]
	}`)
}
func (t ListDirTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	entries, err := os.ReadDir(params.Path)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Contents of %s:\n", params.Path))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			sb.WriteString(fmt.Sprintf("- %s (error getting info)\n", entry.Name()))
			continue
		}
		typeStr := "file"
		if info.IsDir() {
			typeStr = "dir"
		}
		sb.WriteString(fmt.Sprintf("- %-20s [%s] %d bytes\n", entry.Name(), typeStr, info.Size()))
	}
	return sb.String(), nil
}
func (t ListDirTool) RequiresConfirmation(args json.RawMessage) bool { return false }

func (t ListDirTool) CallString(args json.RawMessage) string {
	path := getToolParam(args, "path")
	if cwd, err := os.Getwd(); err == nil {
		path = strings.Replace(path, cwd, ".", 1)
	}
	return fmt.Sprintf("Listing directory %s", truncate(path, 50))
}

// MkdirTool creates a new directory.
type MkdirTool struct{}

func (t MkdirTool) Name() string { return "mkdir" }
func (t MkdirTool) Description() string {
	return "Create a new directory (including parent directories)"
}
func (t MkdirTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": { "type": "string", "description": "Path to the directory to create" }
		},
		"required": ["path"]
	}`)
}
func (t MkdirTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}
	if err := os.MkdirAll(params.Path, 0755); err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully created directory %s", params.Path), nil
}
func (t MkdirTool) RequiresConfirmation(args json.RawMessage) bool {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return true
	}
	return !IsSafePath(params.Path)
}

func (t MkdirTool) CallString(args json.RawMessage) string {
	path := getToolParam(args, "path")
	if cwd, err := os.Getwd(); err == nil {
		path = strings.Replace(path, cwd, ".", 1)
	}
	return fmt.Sprintf("Creating directory %s", truncate(path, 50))
}

// Allowed commands whitelist for BashTool
var allowedCommands = map[string]bool{
	"grep":   true,
	"find":   true,
	"ls":     true,
	"cat":    true,
	"head":   true,
	"tail":   true,
	"echo":   true,
	"pwd":    true,
	"date":   true,
	"whoami": true,
	"mkdir":  true,
	"touch":  true,
	"seq":    true,
}

// Maximum number of output lines to prevent memory exhaustion
const maxBashOutputLines = 1024

// BashTool executes a bash command with security restrictions.
type BashTool struct{}

func (t BashTool) Name() string        { return "bash" }
func (t BashTool) Description() string { return "Execute a safe command with whitelist protection." }
func (t BashTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": { "type": "string", "description": "The base command name only (e.g. 'grep', 'find', 'ls'). Do NOT include arguments here, use the 'args' parameter instead." },
			"args": { "type": "array", "items": { "type": "string" }, "description": "Arguments to pass to the command (optional)" },
			"cwd": { "type": "string", "description": "Working directory for execution, must be within CWD (optional)" }
		},
		"required": ["command"]
	}`)
}
func (t BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Cwd     string   `json:"cwd"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	// Fallback: if command contains spaces, the agent likely put the full command
	// string in the command field. Split it: first token = command, rest = args.
	if strings.Contains(params.Command, " ") {
		parts := strings.Fields(params.Command)
		params.Command = parts[0]
		params.Args = append(parts[1:], params.Args...)
	}

	// Validate that the command is in the whitelist
	if !allowedCommands[params.Command] {
		return "", fmt.Errorf("command '%s' is not in the allowed whitelist", params.Command)
	}

	// Validate and set working directory
	if params.Cwd != "" {
		if !IsSafePath(params.Cwd) {
			return "", fmt.Errorf("cwd '%s' is outside the allowed directory", params.Cwd)
		}
	} else {
		// Default to current directory
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current working directory: %w", err)
		}
		params.Cwd = cwd
	}

	// Validate arguments to prevent command injection
	for _, arg := range params.Args {
		if strings.Contains(arg, "..") || strings.Contains(arg, ";") ||
			strings.Contains(arg, "|") || strings.Contains(arg, "&") {
			return "", fmt.Errorf("argument '%s' contains dangerous characters", arg)
		}
		if strings.HasPrefix(arg, "/") && !IsSafePath(arg) {
			return "", fmt.Errorf("argument path '%s' is outside the allowed directory", arg)
		}
	}

	// Execute the command using exec.CommandContext with args
	cmd := exec.CommandContext(ctx, params.Command, params.Args...)
	cmd.Dir = params.Cwd

	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Sprintf("Command failed with exit code %d\n%s", exitErr.ExitCode(), string(output)), nil
		}
		return fmt.Sprintf("Error executing command: %v\n%s", err, string(output)), nil
	}

	// Limit output to prevent memory exhaustion
	lines := strings.Split(string(output), "\n")
	if len(lines) > maxBashOutputLines {
		lines = lines[:maxBashOutputLines]
		lines = append(lines, "... (output truncated)")
	}

	return strings.Join(lines, "\n"), nil
}
func (t BashTool) RequiresConfirmation(args json.RawMessage) bool {
	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return true // Default to requiring confirmation if we can't parse
	}
	// If agent put full command string, extract base command
	cmd := params.Command
	if i := strings.IndexByte(cmd, ' '); i >= 0 {
		cmd = cmd[:i]
	}
	return !allowedCommands[cmd]
}

func (t BashTool) CallString(args json.RawMessage) string {
	var params struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Cwd     string   `json:"cwd"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "Executing: (invalid args)"
	}

	// Build the display string
	result := fmt.Sprintf("Executing: %s", params.Command)
	if len(params.Args) > 0 {
		result += " " + strings.Join(params.Args, " ")
	}
	if params.Cwd != "" {
		result += " in dir: " + params.Cwd
	}
	return result
}

// AskTool asks the user a question.
type AskTool struct{}

func (t AskTool) Name() string { return "ask" }
func (t AskTool) Description() string {
	return "Ask the user a question. Supports single-choice (free text) and multi-choice (dropdown) modes."
}
func (t AskTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": { "type": "string", "description": "The question to ask the user" },
			"options": { "type": "array", "items": { "type": "string" }, "description": "If provided, user must choose one of these options" }
		},
		"required": ["prompt"]
	}`)
}
func (t AskTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Prompt  string   `json:"prompt"`
		Options []string `json:"options"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	provider := common.GetInputProvider(ctx)
	if provider == nil {
		return "", fmt.Errorf("ask tool: no input provider found in context")
	}

	// Create a generic prompt request
	schema := json.RawMessage(`{"type": "string"}`)
	if len(params.Options) > 0 {
		optionsArr, _ := json.Marshal(params.Options)
		schema = json.RawMessage(fmt.Sprintf(`{"type": "string", "enum": %s}`, string(optionsArr)))
	}

	req := common.PromptRequest{
		Title:       "User Input Required",
		Description: params.Prompt,
		Schema:      schema,
	}

	result, err := provider.Prompt(ctx, req)
	if err != nil {
		return "", fmt.Errorf("ask tool prompt failed: %w", err)
	}

	var userInput string
	if err := json.Unmarshal(result, &userInput); err != nil {
		return "", fmt.Errorf("ask tool: failed to parse provider response: %w", err)
	}

	return userInput, nil
}
func (t AskTool) RequiresConfirmation(args json.RawMessage) bool { return false }

func (t AskTool) CallString(args json.RawMessage) string {
	prompt := getToolParam(args, "prompt")
	return fmt.Sprintf("Asking user: %s", truncate(prompt, 50))
}
