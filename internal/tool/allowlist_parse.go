package tool

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// tier2Commands is the set of commands that have a mandatory subcommand (e.g.
// "git log", "go mod"). It is used by ParseCommandsForAllowList to build the
// canonical allow-list key ("git log" rather than just "git").
var tier2Commands = map[string]bool{
	"git": true,
	"go":  true,
}

// tier2Positionals lists the positional sub-sub-command tokens that should be
// recorded in the allow-list for a given "cmd subcommand" key.  Only tokens
// in this set are included; generic path arguments (e.g. "./...") are ignored.
var tier2Positionals = map[string]map[string]bool{
	"go mod": {"tidy": true, "graph": true, "verify": true, "why": true, "download": true},
}

// wordResolver resolves shell AST word nodes to their string values.
// It only handles static literals — any dynamic expansion (variable, subshell,
// etc.) causes resolution to fail so callers can treat the result as opaque.
type wordResolver struct{}

func (r *wordResolver) resolveWord(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", true
	}
	var sb strings.Builder
	for _, p := range word.Parts {
		if !r.resolvePart(&sb, p) {
			return "", false
		}
	}
	return sb.String(), true
}

func (r *wordResolver) resolvePart(sb *strings.Builder, p syntax.WordPart) bool {
	switch n := p.(type) {
	case *syntax.Lit:
		sb.WriteString(n.Value)
		return true
	case *syntax.SglQuoted:
		sb.WriteString(n.Value)
		return true
	case *syntax.DblQuoted:
		for _, qp := range n.Parts {
			if !r.resolvePart(sb, qp) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// ParseCommandsForAllowList extracts stable keys (e.g., "git log") and their
// lists of flags for ALL commands in a potentially compound string (pipes,
// chains, etc).
func ParseCommandsForAllowList(command string) map[string][]string {
	parser := syntax.NewParser()
	f, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return nil
	}

	commands := make(map[string][]string)
	wr := &wordResolver{}

	syntax.Walk(f, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}

		cmdName, ok := wr.resolveWord(call.Args[0])
		if !ok || cmdName == "" {
			return true
		}

		var key string
		var subCmd string
		var startIdx int

		// Check for subcommand (only for known multi-level commands)
		if tier2Commands[cmdName] && len(call.Args) >= 2 {
			sc, ok := wr.resolveWord(call.Args[1])
			if ok && sc != "" && !strings.HasPrefix(sc, "-") {
				key = cmdName + " " + sc
				subCmd = sc
				startIdx = 2
			} else {
				key = cmdName
				startIdx = 1
			}
		} else {
			key = cmdName
			startIdx = 1
		}

		var flags []string
		for i := startIdx; i < len(call.Args); i++ {
			val, ok := wr.resolveWord(call.Args[i])
			if !ok {
				continue
			}

			if strings.HasPrefix(val, "-") {
				// Strip key-value pairs (e.g., --output=foo -> --output)
				flagKey := val
				if idx := strings.Index(val, "="); idx != -1 {
					flagKey = val[:idx]
				}

				// Normalize numeric flags
				if isNumericFlag(val) {
					flags = append(flags, "-*")
				} else {
					flags = append(flags, flagKey)
				}
			} else if subCmd != "" {
				// Positional argument — only record it when it is an explicitly
				// whitelisted sub-sub-command (e.g. 'tidy' in 'go mod tidy').
				// Generic path arguments like './...' are intentionally skipped.
				if tier2Positionals[key][val] {
					flags = append(flags, val)
				}
			}
		}

		if key != "" {
			commands[key] = append(commands[key], flags...)
		}

		return true
	})

	return commands
}

// isNumericFlag reports whether s is a flag consisting only of digits (e.g. -20).
func isNumericFlag(s string) bool {
	if len(s) < 2 || s[0] != '-' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isNumericFd reports whether s is a valid numeric file descriptor (or "-").
func isNumericFd(s string) bool {
	if s == "-" {
		return true
	}
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
