package tool

import (
	"context"
	"encoding/json" // used for hash generation
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"late/internal/common"
	"mvdan.cc/sh/v3/syntax"
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
		lineStr := fmt.Sprintf("%d | %s\n", l.lineNum, l.content)
		if sb.Len()+len(lineStr) > maxReadFileChars {
			sb.WriteString("... (output truncated)")
			break
		}
		sb.WriteString(lineStr)
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
	path := getToolParam(args, "path")
	if path == "" {
		return true // Default to safe if we can't parse yet
	}
	return !IsSafePath(path)
}

func (t WriteFileTool) CallString(args json.RawMessage) string {
	path := getToolParam(args, "path")
	if path == "" {
		return "Writing to file..."
	}
	if cwd, err := os.Getwd(); err == nil {
		path = strings.Replace(path, cwd, ".", 1)
	}
	return fmt.Sprintf("Writing to file %s", truncate(path, 50))
}

// Commands that do not require user confirmation for ShellTool.
// Only genuinely read-only commands belong here.
var whitelistedCommands = map[string]bool{
	"grep":   true,
	"ls":     true,
	"cat":    true,
	"head":   true,
	"tail":   true,
	"pwd":    true,
	"date":   true,
	"whoami": true,
	"wc":     true,
	"seq":    true,
	"file":   true,
	"echo":   true,
}

// Windows PowerShell commands that are considered read-only/safe for
// auto-approval when no risky syntax is present.
var whitelistedWindowsCommands = map[string]bool{
	"cat":            true,
	"date":           true,
	"dir":            true,
	"echo":           true,
	"gc":             true,
	"gci":            true,
	"get-childitem":  true,
	"get-content":    true,
	"get-date":       true,
	"get-location":   true,
	"ls":             true,
	"measure-object": true,
	"pwd":            true,
	"select-string":  true,
	"sls":            true,
	"type":           true,
	"whoami":         true,
	"write-output":   true,
}

// tokenizePowerShellCommand splits a command into tokens while honoring
// single/double quotes and PowerShell backtick escaping.
func tokenizePowerShellCommand(command string) []string {
	tokens := make([]string, 0)
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for i := 0; i < len(command); i++ {
		ch := command[i]

		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}

		if !inSingle && ch == '`' {
			escaped = true
			continue
		}

		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		if !inSingle && !inDouble {
			if ch == ';' || ch == '|' {
				flush()
				tokens = append(tokens, string(ch))
				continue
			}
			if ch == '&' {
				flush()
				if i+1 < len(command) && command[i+1] == '&' {
					tokens = append(tokens, "&&")
					i++
				} else {
					tokens = append(tokens, "&")
				}
				continue
			}
			if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
				flush()
				continue
			}
		}

		current.WriteByte(ch)
	}

	flush()
	return tokens
}

func getPowerShellBaseCommands(command string) []string {
	tokens := tokenizePowerShellCommand(command)
	commands := make([]string, 0)
	expectCommand := true

	for _, token := range tokens {
		switch token {
		case ";", "|", "||", "&&", "&":
			expectCommand = true
			continue
		}
		if expectCommand {
			commands = append(commands, strings.ToLower(token))
			expectCommand = false
		}
	}

	return commands
}

func containsPowerShellRiskySyntax(command string) bool {
	lower := strings.ToLower(command)
	if strings.ContainsAny(command, "\n\r\x00") {
		return true
	}
	if strings.ContainsAny(command, "><") {
		return true
	}
	if strings.Contains(lower, "$(") {
		return true
	}

	for _, keyword := range []string{
		" invoke-expression",
		" iex ",
		" start-process",
		" invoke-command",
		" new-object",
		" remove-item",
		" rename-item",
		" move-item",
		" copy-item",
		" set-content",
		" add-content",
		" out-file",
		" clear-content",
		" set-itemproperty",
		" -encodedcommand",
	} {
		if strings.Contains(" "+lower, keyword) {
			return true
		}
	}

	return false
}

func extractPowerShellTargetPath(command string) string {
	tokens := tokenizePowerShellCommand(strings.TrimSpace(command))
	if len(tokens) < 2 {
		return ""
	}

	cmd := strings.ToLower(tokens[0])
	target := ""

	switch cmd {
	case "mkdir", "md":
		target = tokens[1]
	case "new-item", "ni":
		if len(tokens) == 2 {
			target = tokens[1]
		} else if len(tokens) >= 3 && strings.EqualFold(tokens[1], "-Path") {
			target = tokens[2]
		}
	default:
		return ""
	}

	if target == "" || strings.HasPrefix(target, "-") {
		return ""
	}
	if strings.HasPrefix(target, "~") || strings.Contains(target, "$") || strings.ContainsAny(target, "*?[") {
		return ""
	}

	return target
}

// isSafeWordPart returns true if the WordPart is a literal or a quoted string
// that contains only literals (no expansions, no subshells).
func isSafeWordPart(p syntax.WordPart) bool {
	switch n := p.(type) {
	case *syntax.Lit:
		return true
	case *syntax.SglQuoted:
		return true
	case *syntax.DblQuoted:
		for _, qp := range n.Parts {
			if !isSafeWordPart(qp) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// analyzeBashCommand performs a deep AST analysis of a bash command to identify
// security risks (blocking) and complexity (confirmation requirements).
func (t *ShellTool) analyzeBashCommand(command string) (isBlocked bool, blockReason error, needsConfirmation bool) {
	parser := syntax.NewParser()
	f, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		// If we can't parse it, it might be a weird one-liner that's valid shell but not POSIX/Bash.
		// Conservative approach: require confirmation, but don't block unless we're sure.
		return false, nil, true
	}

	needsConfirmation = false
	isBlocked = false

	syntax.Walk(f, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CallExpr:
			if len(n.Args) > 0 {
				// Try to get the command name even if quoted
				var cmdName string
				word := n.Args[0]
				if len(word.Parts) == 1 {
					switch p := word.Parts[0].(type) {
					case *syntax.Lit:
						cmdName = p.Value
					case *syntax.SglQuoted:
						cmdName = p.Value
					case *syntax.DblQuoted:
						if len(p.Parts) == 1 {
							if lit, ok := p.Parts[0].(*syntax.Lit); ok {
								cmdName = lit.Value
							}
						}
					}
				}

				if cmdName == "" {
					needsConfirmation = true
				} else {
					if cmdName == "cd" {
						isBlocked = true
						needsConfirmation = true
						blockReason = fmt.Errorf("Do not use `cd` to change directories. Use the `cwd` parameter in the shell tool instead.")
						return false
					}
					if !whitelistedCommands[cmdName] {
						needsConfirmation = true
					}
				}
			}
			if len(n.Assigns) > 0 {
				needsConfirmation = true
			}
			// Check if any argument is not a simple literal/safe quoted part
			for _, arg := range n.Args {
				if arg != nil {
					for _, p := range arg.Parts {
						if !isSafeWordPart(p) {
							needsConfirmation = true
						}
					}
				}
			}

		case *syntax.Redirect:
			// Op is RedirOperator. Check if it's an output redirect.
			// RdrOut (>), AppOut (>>), RdrAll (&>), AppAll (&>>), RdrClob (>|), AppClob (not used in bash really but safe to block)
			switch n.Op {
			case syntax.RdrOut, syntax.AppOut, syntax.RdrAll, syntax.AppAll, syntax.RdrClob, syntax.AppClob, syntax.DplOut:
				isBlocked = true
				needsConfirmation = true
				blockReason = fmt.Errorf("Output redirection (>) is blocked. Use `write_file` or `target_edit` to modify files.")
				return false
			}
		case *syntax.BinaryCmd:
			// Pipes (|), logical operators (&&, ||), etc.
			needsConfirmation = true
		case *syntax.CmdSubst, *syntax.Subshell, *syntax.ProcSubst:
			// $(cmd), `cmd`, (cmd), <(cmd)
			needsConfirmation = true
		case *syntax.IfClause, *syntax.WhileClause, *syntax.ForClause, *syntax.CaseClause, *syntax.Block:
			// Control structures
			needsConfirmation = true
		case *syntax.ParamExp:
			// ${var}
			needsConfirmation = true
		}
		return true
	})

	return isBlocked, blockReason, needsConfirmation
}

// ValidateBashCommand validates shell commands before execution.
// Returns an error if the command uses malicious patterns like cat shenanigans or cd commands.
func (t *ShellTool) ValidateBashCommand(command string) error {
	blocked, err, _ := t.analyzeBashCommand(command)
	if blocked {
		return err
	}
	return nil
}

// WrapError wraps a validation error with descriptive guidance based on the orchestrator ID.
func (t *ShellTool) WrapError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	orchestratorID := common.GetOrchestratorID(ctx)

	var errorMsg string
	if strings.Contains(strings.ToLower(orchestratorID), "coder") {
		errorMsg = fmt.Sprintf("Do not use %s commands like `cat > file` or `echo > file` to write files. Use the native `write_file` or `target_edit` tools instead. %s", shellDisplayName(), err.Error())
	} else {
		errorMsg = fmt.Sprintf("You are an architect/planner agent. You cannot write files. To modify files, you must spawn a coder subagent using `spawn_subagent` tool. %s", err.Error())
	}

	return fmt.Errorf("%s", errorMsg)
}

// IsCommandBlocked checks if a shell command should be blocked entirely (not asked for confirmation).
// Returns true and an error if the command is blocked (e.g., cd commands).
func (t *ShellTool) IsCommandBlocked(command string) (bool, error) {
	blocked, err, _ := t.analyzeBashCommand(command)
	return blocked, err
}

// Maximum number of output lines to prevent memory exhaustion
const maxBashOutputLines = 1024

// Roughly 8192 tokens (assuming ~4 chars per token)
const maxReadFileChars = 32768

// ShellTool executes host-native shell commands with security restrictions.
type ShellTool struct{}

func shellDisplayName() string {
	if runtime.GOOS == "windows" {
		return "PowerShell"
	}
	return "bash"
}

func (t ShellTool) Name() string { return "bash" }
func (t ShellTool) Description() string {
	return fmt.Sprintf("Execute a %s command.", shellDisplayName())
}
func (t ShellTool) Parameters() json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{
		"type": "object",
		"properties": {
			"command": { "type": "string", "description": "The full %s command to execute." },
			"cwd": { "type": "string", "description": "Working directory for execution. Use this instead of 'cd' commands to change directories." }
		},
		"required": ["command"]
	}`, shellDisplayName()))
}
func (t ShellTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	// Validate command before any execution
	if err := t.ValidateBashCommand(params.Command); err != nil {
		return "", t.WrapError(ctx, err)
	}

	// Enforce approval in the execution path so shell commands fail closed
	// even if middleware wiring is missing.
	if t.RequiresConfirmation(args) {
		approved, ok := ctx.Value(common.ToolApprovalKey).(bool)
		if !ok || !approved {
			return "", fmt.Errorf("shell command requires explicit approval before execution")
		}
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

	// Execute command using a platform-specific shell wrapper.
	cmd := newShellCommand(ctx, params.Command)
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
func (t ShellTool) RequiresConfirmation(args json.RawMessage) bool {
	var params struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return true // Default to requiring confirmation if we can't parse
	}

	if runtime.GOOS == "windows" {
		// Denylist first: if command shape is risky or can hide execution intent,
		// always require confirmation.
		if containsPowerShellRiskySyntax(params.Command) {
			return true
		}

		// Permit creation only for simple, explicit new paths inside allowed roots.
		if target := extractPowerShellTargetPath(params.Command); target != "" && isNewPath(target, params.Cwd) {
			return false
		}

		// Parser-backed base command extraction allows safer classification than
		// naive whitespace splitting.
		baseCommands := getPowerShellBaseCommands(params.Command)
		for _, cmd := range baseCommands {
			if !whitelistedWindowsCommands[cmd] {
				return true
			}
		}
		return false
	}

	if runtime.GOOS == "windows" {
		// Denylist first: if command shape is risky or can hide execution intent,
		// always require confirmation.
		if containsPowerShellRiskySyntax(params.Command) {
			return true
		}

		// Permit creation only for simple, explicit new paths inside allowed roots.
		if target := extractPowerShellTargetPath(params.Command); target != "" && isNewPath(target, params.Cwd) {
			return false
		}

		// Parser-backed base command extraction allows safer classification than
		// naive whitespace splitting.
		baseCommands := getPowerShellBaseCommands(params.Command)
		for _, cmd := range baseCommands {
			if !whitelistedWindowsCommands[cmd] {
				return true
			}
		}
		return false
	}

	_, _, needsConfirmation := t.analyzeBashCommand(params.Command)
	return needsConfirmation

}

func (t ShellTool) CallString(args json.RawMessage) string {
	var params struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		if runtime.GOOS == "windows" {
			return "Executing in PowerShell: (invalid args)"
		}
		return "Executing: (invalid args)"
	}

	// Build the display string
	var result string
	if runtime.GOOS == "windows" {
		result = fmt.Sprintf("Executing in PowerShell: %s", params.Command)
	} else {
		result = fmt.Sprintf("Executing: %s", params.Command)
	}
	if params.Cwd != "" {
		result += " in dir: " + params.Cwd
	}
	return result
}

// WriteImplementationPlanTool writes the implementation plan to a fixed file.
type WriteImplementationPlanTool struct{}

func (t WriteImplementationPlanTool) Name() string { return "write_implementation_plan" }
func (t WriteImplementationPlanTool) Description() string {
	return "Write the implementation plan to ./implementation_plan.md in the current working directory."
}
func (t WriteImplementationPlanTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"plan": { "type": "string", "description": "The full content of the implementation plan in Markdown format." }
		},
		"required": ["plan"]
	}`)
}
func (t WriteImplementationPlanTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	if params.Plan == "" {
		return "", fmt.Errorf("Implementation plan cannot be empty")
	}

	path := "implementation_plan.md"
	if err := os.WriteFile(path, []byte(params.Plan), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully wrote implementation plan to %s", path), nil
}
func (t WriteImplementationPlanTool) RequiresConfirmation(args json.RawMessage) bool { return false }

func (t WriteImplementationPlanTool) CallString(args json.RawMessage) string {
	return "Writing implementation plan to ./implementation_plan.md"
}
