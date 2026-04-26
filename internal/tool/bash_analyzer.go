package tool

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// tier1AllowList defines simple commands and their permitted flags.
// Positional arguments (not starting with '-') are allowed if the command is in this list.
var tier1AllowList = map[string]map[string]bool{
	"ls":     {"-l": true, "-a": true, "-la": true, "-1": true, "-R": true, "-h": true, "--color": true, "-F": true},
	"cat":    {"-n": true, "-b": true, "-v": true},
	"head":   {"-n": true, "-c": true},
	"tail":   {"-n": true, "-c": true, "-f": true},
	"pwd":    {"-P": true, "-L": true},
	"date":   {"-u": true, "-R": true},
	"whoami": {},
	"wc":     {"-l": true, "-w": true, "-c": true, "-m": true},
	"seq":    {},
	"file":   {"-b": true, "-i": true},
	"echo":   {"-n": true, "-e": true},
	"du":     {"-h": true, "-s": true, "-a": true, "-c": true},
	"df":     {"-h": true, "-T": true},
	"stat":   {"-c": true, "-f": true},
	"lsof":   {"-i": true, "-p": true, "-u": true, "-n": true, "-P": true},
	"grep":   {"-i": true, "-v": true, "-l": true, "-n": true, "-r": true, "-R": true, "-E": true, "-F": true, "-w": true, "-x": true, "-c": true},
}

// tier2AllowList defines complex commands with subcommands and their permitted flags.
var tier2AllowList = map[string]map[string]map[string]bool{
	"git": {
		"status":    {"-s": true, "--short": true, "--long": true, "-b": true, "--branch": true, "--porcelain": true},
		"log":       {"--oneline": true, "--stat": true, "-n": true, "--author": true, "--graph": true, "--patch": true, "-p": true, "--reverse": true, "--all": true},
		"diff":      {"--stat": true, "--cached": true, "--staged": true, "-p": true, "--patch": true, "--color": true, "--name-only": true, "--name-status": true},
		"show":      {"--stat": true, "--oneline": true, "-p": true, "--patch": true, "--name-only": true},
		"tag":       {"-l": true, "--list": true},
		"rev-parse": {"--show-toplevel": true, "--abbrev-ref": true, "--short": true},
		"remote":    {"-v": true},
	},
	"go": {
		"doc": {"-all": true, "-src": true, "-u": true},
		"mod": {"tidy": true, "graph": true, "verify": true, "why": true, "download": true},
	},
}

// findAllowedFlags defines flags permitted for the 'find' command.
var findAllowedFlags = map[string]bool{
	"-name":      true,
	"-iname":     true,
	"-type":      true,
	"-maxdepth":  true,
	"-mindepth":  true,
	"-size":      true,
	"-mtime":     true,
	"-atime":     true,
	"-ctime":     true,
	"-newer":     true,
	"-user":      true,
	"-group":     true,
	"-path":      true,
	"-ipath":     true,
	"-links":     true,
	"-empty":     true,
	"-not":       true,
	"-and":       true,
	"-or":        true,
}

// allowedEnvVars contains environment variables that are safe to set.
var allowedEnvVars = map[string]bool{
	"DEBUG":       true,
	"LANG":        true,
	"LC_ALL":      true,
	"TERM":        true,
	"COLOR":       true,
	"GOOS":        true,
	"GOARCH":      true,
	"CGO_ENABLED": true,
}

type BashAnalyzer struct{}

func (b *BashAnalyzer) Analyze(command string) CommandAnalysis {
	parser := syntax.NewParser()
	f, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return CommandAnalysis{NeedsConfirmation: true}
	}

	analysis := CommandAnalysis{}

	var checkNode func(node syntax.Node) bool
	checkNode = func(node syntax.Node) bool {
		if node == nil {
			return true
		}
		switch n := node.(type) {
		case *syntax.CallExpr:
			if !b.isSafeCall(n, &analysis) {
				analysis.NeedsConfirmation = true
			}
			if analysis.IsBlocked {
				return false
			}

		case *syntax.Redirect:
			switch n.Op {
			case syntax.RdrOut, syntax.AppOut, syntax.RdrAll, syntax.AppAll, syntax.RdrClob, syntax.AppClob, syntax.DplOut:
				analysis.IsBlocked = true
				analysis.NeedsConfirmation = true
				analysis.BlockReason = fmt.Errorf("Output redirection (>) is blocked. Use `write_file` or `target_edit` to modify files.")
				return false
			}

		case *syntax.BinaryCmd:
			if !checkNode(n.X) || !checkNode(n.Y) {
				return false
			}

		case *syntax.Stmt:
			for _, redir := range n.Redirs {
				if !checkNode(redir) {
					return false
				}
			}
			if !checkNode(n.Cmd) {
				return false
			}

		case *syntax.File:
			for _, stmt := range n.Stmts {
				if !checkNode(stmt) {
					return false
				}
			}

		case *syntax.Block:
			for _, stmt := range n.Stmts {
				if !checkNode(stmt) {
					return false
				}
			}
			analysis.NeedsConfirmation = true

		case *syntax.CmdSubst, *syntax.Subshell, *syntax.ProcSubst:
			analysis.NeedsConfirmation = true

		case *syntax.IfClause, *syntax.WhileClause, *syntax.ForClause, *syntax.CaseClause:
			analysis.NeedsConfirmation = true

		case *syntax.ParamExp:
			analysis.NeedsConfirmation = true
		}
		return true
	}

	checkNode(f)

	return analysis
}

func (b *BashAnalyzer) isSafeCall(n *syntax.CallExpr, analysis *CommandAnalysis) bool {
	if len(n.Args) == 0 {
		return true
	}

	cmdName := b.extractCommandName(n.Args[0])
	if cmdName == "" || strings.Contains(cmdName, "/") {
		return false
	}

	// SECURITY: Block 'cd' explicitly.
	if cmdName == "cd" {
		analysis.IsBlocked = true
		analysis.BlockReason = fmt.Errorf("Do not use `cd` to change directories. Use the `cwd` parameter in the shell tool instead.")
		return false
	}

	// Step 1: Environment check
	for _, assign := range n.Assigns {
		if assign.Name == nil || !allowedEnvVars[assign.Name.Value] {
			return false
		}
		if assign.Value == nil {
			return false
		}
		for _, p := range assign.Value.Parts {
			if !isSafeWordPart(p) {
				return false
			}
		}
	}

	// Step 2: Tier Categorization and Validation
	if allowedFlags, ok := tier1AllowList[cmdName]; ok {
		return b.validateTier1(cmdName, n.Args[1:], allowedFlags)
	}

	if subcommands, ok := tier2AllowList[cmdName]; ok {
		return b.validateTier2(cmdName, n.Args[1:], subcommands)
	}

	if cmdName == "find" {
		return b.validateFind(n.Args[1:])
	}

	// Default Deny
	return false
}

func (b *BashAnalyzer) validateTier1(cmd string, args []*syntax.Word, allowedFlags map[string]bool) bool {
	for _, arg := range args {
		val := b.extractCommandName(arg)
		if strings.HasPrefix(val, "-") {
			if !allowedFlags[val] {
				return false
			}
		} else {
			// Positional argument
			if !b.isSafePositionalArg(arg) {
				return false
			}
		}
	}
	return true
}

func (b *BashAnalyzer) validateTier2(cmd string, args []*syntax.Word, subcommands map[string]map[string]bool) bool {
	if len(args) == 0 {
		return true // Just the base command is help
	}

	subCmd := b.extractCommandName(args[0])
	if subCmd == "" || strings.HasPrefix(subCmd, "-") {
		return false // Subcommand expected
	}

	allowedFlags, ok := subcommands[subCmd]
	if !ok {
		return false // Subcommand not whitelisted
	}

	// Validate remaining arguments
	for _, arg := range args[1:] {
		val := b.extractCommandName(arg)
		if strings.HasPrefix(val, "-") {
			if !allowedFlags[val] {
				return false
			}
		} else {
			// Positional argument
			if !b.isSafePositionalArg(arg) {
				return false
			}
		}
	}
	return true
}

func (b *BashAnalyzer) validateFind(args []*syntax.Word) bool {
	for _, arg := range args {
		val := b.extractCommandName(arg)
		if strings.HasPrefix(val, "-") {
			// Find flags often start with - but are not exactly like standard flags.
			// Still, we check them against an allow-list.
			if !findAllowedFlags[val] {
				return false
			}
		} else {
			// Positional argument (path, etc)
			if !b.isSafePositionalArg(arg) {
				return false
			}
		}
	}
	return true
}

func (b *BashAnalyzer) isSafePositionalArg(word *syntax.Word) bool {
	if word == nil {
		return true
	}
	// Ensure it doesn't look like a flag (injection prevention)
	val := b.extractCommandName(word)
	if strings.HasPrefix(val, "-") {
		return false
	}

	for _, p := range word.Parts {
		if !isSafeWordPart(p) {
			return false
		}
	}
	return true
}

func (b *BashAnalyzer) extractCommandName(word *syntax.Word) string {
	if word == nil || len(word.Parts) == 0 {
		return ""
	}

	var rawName string
	if len(word.Parts) == 1 {
		switch p := word.Parts[0].(type) {
		case *syntax.Lit:
			rawName = p.Value
		case *syntax.SglQuoted:
			rawName = p.Value
		case *syntax.DblQuoted:
			if len(p.Parts) == 1 {
				if lit, ok := p.Parts[0].(*syntax.Lit); ok {
					rawName = lit.Value
				}
			}
		}
	}

	return rawName
}

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
