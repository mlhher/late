package tool

import (
	"context"
	"encoding/json" // used for hash generation
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

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

// Commands that do not require user confirmation for BashTool
var whitelistedCommands = map[string]bool{
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

func (t BashTool) Name() string { return "bash" }
func (t BashTool) Description() string {
	return "Execute a bash command. You MUST provide the 'command' (the specific command name) seperated from the 'args' (i.e. {command: 'ls', args: '-la'} instead of {command: 'ls -la'}). If the tool call contains arguments inside the 'command' it will be rejected."
}
func (t BashTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": { "type": "string", "description": "The actual shell command you want to call without arguments (e.g. 'ls' instead of 'ls -la')." },
			"args": { "type": "array", "items": { "type": "string" }, "description": "Arguments to pass to the command (optional, e.g. if command is 'ls' args could be '-la')" },
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
		// TODO: Maybe adjust the prompt.
		return "", fmt.Errorf("command contains spaces. You MUST provide the commands arguments inside the 'args' parameter. The 'command' parameter MUST only be the exact command you want to call e.g. 'ls'. If you want to call e.g. 'ls -la' you have to use the appropriate fields (command 'ls' and args '-la')")
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
		// TODO: Look at this further
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
	return !whitelistedCommands[cmd]
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
