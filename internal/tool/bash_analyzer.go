package tool

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// BashwhitelistedCommands contains commands that do not require user confirmation.
// Only genuinely read-only commands belong here.
var bashWhitelistedCommands = map[string]bool{
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

type BashAnalyzer struct{}

func (b *BashAnalyzer) Analyze(command string) CommandAnalysis {
	parser := syntax.NewParser()
	f, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		// If we can't parse it, it might be a weird one-liner that's valid shell but not POSIX/Bash.
		// Conservative approach: require confirmation, but don't block unless we're sure.
		return CommandAnalysis{NeedsConfirmation: true}
	}

	analysis := CommandAnalysis{}

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
					analysis.NeedsConfirmation = true
				} else {
					if cmdName == "cd" {
						analysis.IsBlocked = true
						analysis.NeedsConfirmation = true
						analysis.BlockReason = fmt.Errorf("Do not use `cd` to change directories. Use the `cwd` parameter in the shell tool instead.")
						return false
					}
					if !bashWhitelistedCommands[cmdName] {
						analysis.NeedsConfirmation = true
					}
				}
			}
			if len(n.Assigns) > 0 {
				analysis.NeedsConfirmation = true
			}
			// Check if any argument is not a simple literal/safe quoted part
			for _, arg := range n.Args {
				if arg != nil {
					for _, p := range arg.Parts {
						if !isSafeWordPart(p) {
							analysis.NeedsConfirmation = true
						}
					}
				}
			}

		case *syntax.Redirect:
			// Op is RedirOperator. Check if it's an output redirect.
			switch n.Op {
			case syntax.RdrOut, syntax.AppOut, syntax.RdrAll, syntax.AppAll, syntax.RdrClob, syntax.AppClob, syntax.DplOut:
				analysis.IsBlocked = true
				analysis.NeedsConfirmation = true
				analysis.BlockReason = fmt.Errorf("Output redirection (>) is blocked. Use `write_file` or `target_edit` to modify files.")
				return false
			}
		case *syntax.BinaryCmd:
			// Pipes (|), logical operators (&&, ||), etc.
			analysis.NeedsConfirmation = true
		case *syntax.CmdSubst, *syntax.Subshell, *syntax.ProcSubst:
			// $(cmd), `cmd`, (cmd), <(cmd)
			analysis.NeedsConfirmation = true
		case *syntax.IfClause, *syntax.WhileClause, *syntax.ForClause, *syntax.CaseClause, *syntax.Block:
			// Control structures
			analysis.NeedsConfirmation = true
		case *syntax.ParamExp:
			// ${var}
			analysis.NeedsConfirmation = true
		}
		return true
	})

	return analysis
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
