package tool

import (
	"strings"
)

// whitelistedWindowsCommands contains PowerShell commands that are considered read-only/safe.
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
	"write-host":     true,
	"write-output":   true,
}

type PowerShellAnalyzer struct {
	Cwd string
}

func (p *PowerShellAnalyzer) Analyze(command string) CommandAnalysis {
	// Denylist first: if command shape is risky or can hide execution intent,
	// always require confirmation.
	if containsPowerShellRiskySyntax(command) {
		return CommandAnalysis{NeedsConfirmation: true}
	}

	// Permit creation only for simple, explicit new paths inside allowed roots.
	if target := extractPowerShellTargetPath(command); target != "" && isNewPath(target, p.Cwd) {
		return CommandAnalysis{NeedsConfirmation: false}
	}

	// Parser-backed base command extraction allows safer classification than
	// naive whitespace splitting.
	baseCommands := getPowerShellBaseCommands(command)
	for _, cmd := range baseCommands {
		if !whitelistedWindowsCommands[cmd] {
			return CommandAnalysis{NeedsConfirmation: true}
		}
	}
	return CommandAnalysis{NeedsConfirmation: false}
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
		// Scan for explicit -Path flag first, then fall back to the first
		// positional (non-flag) argument.  This handles argument orders like:
		//   New-Item foo
		//   New-Item -Path foo
		//   New-Item -ItemType Directory -Path foo
		//   New-Item -Path foo -ItemType Directory
		for i := 1; i < len(tokens); i++ {
			if strings.EqualFold(tokens[i], "-Path") || strings.EqualFold(tokens[i], "-p") {
				if i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "-") {
					target = tokens[i+1]
				}
				break
			}
			if !strings.HasPrefix(tokens[i], "-") {
				target = tokens[i]
				break
			}
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

// isNewPath is defined in permissions.go
