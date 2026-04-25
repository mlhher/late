package tool

// CommandAnalysis result contains the security and complexity analysis of a shell command.
type CommandAnalysis struct {
	IsBlocked         bool
	BlockReason       error
	NeedsConfirmation bool
}

// CommandAnalyzer analyzes shell commands for security and safety.
type CommandAnalyzer interface {
	Analyze(command string) CommandAnalysis
}
