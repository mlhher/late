package tool

import (
	"runtime"

	"late/internal/tool/ast"
)

// windowsBuiltinAllowedCommands returns the hardcoded safe PowerShell cmdlets
// that mirror whitelistedWindowsCommands in powershell_analyzer.go. These are
// merged into AllowedCommands so the PolicyEngine auto-approves them in
// enforcement mode without requiring the user to manually allowlist them.
func windowsBuiltinAllowedCommands() map[string]map[string]bool {
	builtins := []string{
		"cat", "date", "dir", "echo",
		"gc", "gci", "get-childitem", "get-content", "get-date", "get-location",
		"ls", "measure-object", "pwd",
		"select-string", "sls", "type", "whoami", "write-output",
		"write-host",
	}
	m := make(map[string]map[string]bool, len(builtins))
	for _, cmd := range builtins {
		m[cmd] = map[string]bool{}
	}
	return m
}

// mergeAllowedCommands merges src into dst, returning dst.
func mergeAllowedCommands(dst, src map[string]map[string]bool) map[string]map[string]bool {
	for cmd, flags := range src {
		if _, ok := dst[cmd]; !ok {
			dst[cmd] = make(map[string]bool)
		}
		for f := range flags {
			dst[cmd][f] = true
		}
	}
	return dst
}

// astAnalyzer wraps the ast pipeline and implements CommandAnalyzer so it can
// be dropped into ShellTool.getAnalyzer as a drop-in replacement (Phase 5).
type astAnalyzer struct {
	parser ast.Parser
	policy *ast.PolicyEngine
	cwd    string
}

func newASTAnalyzer(platform ast.Platform, cwd string, allowed map[string]map[string]bool) *astAnalyzer {
	// On Windows, seed the policy engine with the built-in safe cmdlets so
	// that Get-ChildItem, ls, pwd etc. auto-approve without user allowlisting.
	if runtime.GOOS == "windows" {
		allowed = mergeAllowedCommands(allowed, windowsBuiltinAllowedCommands())
	}
	return &astAnalyzer{
		parser: ast.NewParser(platform, cwd),
		policy: &ast.PolicyEngine{AllowedCommands: allowed},
		cwd:    cwd,
	}
}

func (a *astAnalyzer) Analyze(command string) CommandAnalysis {
	ir, err := a.parser.Parse(command)
	if err != nil {
		// Fail closed on any parse error.
		return CommandAnalysis{NeedsConfirmation: true}
	}
	d := a.policy.Decide(ir)

	// New-path carveout: if the only signal is ReasonNewPath (mkdir/New-Item
	// creating a net-new path within the cwd), apply the same auto-approval
	// the legacy PowerShellAnalyzer uses. This check runs in Go so we can
	// call isNewPath with the session cwd without touching the bridge.
	if d.NeedsConfirmation && !d.IsBlocked && ir.Platform == ast.PlatformWindows {
		if ast.HasRiskOnly(ir, ast.ReasonNewPath) {
			if target := extractPowerShellTargetPath(command); target != "" && isNewPath(target, a.cwd) {
				return CommandAnalysis{NeedsConfirmation: false}
			}
		}
	}

	return CommandAnalysis{
		IsBlocked:         d.IsBlocked,
		BlockReason:       d.BlockReason,
		NeedsConfirmation: d.NeedsConfirmation,
	}
}

// shadowAnalyzerShim bridges the ast.LegacyAnalysis interface with the
// concrete CommandAnalyzer types in this package so ShadowAnalyzer can wrap
// them without importing tool (which would be circular).
type shadowAnalyzerShim struct {
	inner CommandAnalyzer
}

func (s *shadowAnalyzerShim) Analyze(command string) ast.LegacyAnalysis {
	ca := s.inner.Analyze(command)
	return ast.LegacyAnalysis{
		IsBlocked:         ca.IsBlocked,
		BlockReason:       ca.BlockReason,
		NeedsConfirmation: ca.NeedsConfirmation,
	}
}

// shadowWrapper wraps an ast.ShadowAnalyzer and implements CommandAnalyzer.
type shadowWrapper struct {
	shadow *ast.ShadowAnalyzer
}

func (sw *shadowWrapper) Analyze(command string) CommandAnalysis {
	la := sw.shadow.Analyze(command)
	return CommandAnalysis{
		IsBlocked:         la.IsBlocked,
		BlockReason:       la.BlockReason,
		NeedsConfirmation: la.NeedsConfirmation,
	}
}
