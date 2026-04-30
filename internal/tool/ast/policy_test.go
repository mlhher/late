package ast

import (
	"testing"
)

func TestPolicyEngine_Decide_Blocked(t *testing.T) {
	pe := &PolicyEngine{}

	t.Run("cd blocks", func(t *testing.T) {
		ir := emptyIR(PlatformUnix)
		ir.RiskFlags = []ReasonCode{ReasonCd}
		d := pe.Decide(ir)
		if !d.IsBlocked {
			t.Error("expected IsBlocked for ReasonCd")
		}
		if d.BlockReason == nil {
			t.Error("expected non-nil BlockReason for ReasonCd")
		}
	})

	t.Run("redirect blocks", func(t *testing.T) {
		ir := emptyIR(PlatformUnix)
		ir.RiskFlags = []ReasonCode{ReasonRedirect}
		d := pe.Decide(ir)
		if !d.IsBlocked {
			t.Error("expected IsBlocked for ReasonRedirect")
		}
	})

	t.Run("syntax error fail-closed", func(t *testing.T) {
		ir := emptyIR(PlatformUnix)
		ir.RiskFlags = []ReasonCode{ReasonSyntaxError}
		d := pe.Decide(ir)
		if !d.NeedsConfirmation {
			t.Error("expected NeedsConfirmation for ReasonSyntaxError")
		}
		if d.IsBlocked {
			t.Error("syntax error should not be IsBlocked, only NeedsConfirmation")
		}
	})
}

func TestPolicyEngine_Decide_SoftSignals(t *testing.T) {
	pe := &PolicyEngine{}
	for _, rc := range []ReasonCode{ReasonSubshell, ReasonExpansion, ReasonInvokeExpr, ReasonDestructive} {
		t.Run(string(rc), func(t *testing.T) {
			ir := emptyIR(PlatformUnix)
			ir.RiskFlags = []ReasonCode{rc}
			d := pe.Decide(ir)
			if !d.NeedsConfirmation {
				t.Errorf("expected NeedsConfirmation for %v", rc)
			}
			if d.IsBlocked {
				t.Errorf("expected not IsBlocked for soft signal %v", rc)
			}
		})
	}
}

func TestPolicyEngine_Decide_AllowlistedAutoApprove(t *testing.T) {
	pe := &PolicyEngine{
		AllowedCommands: map[string]map[string]bool{
			"ls":   {},
			"grep": {},
		},
	}

	ir := emptyIR(PlatformUnix)
	ir.Commands = []string{"ls", "grep"}
	ir.RiskFlags = []ReasonCode{ReasonOperator}
	ir.Operators = []string{"|"}

	d := pe.Decide(ir)
	if d.NeedsConfirmation {
		t.Error("expected auto-approve for pipe between all-allowlisted commands")
	}
	if d.IsBlocked {
		t.Error("expected not blocked for allowlisted pipe")
	}
}

func TestPolicyEngine_Decide_PartialAllowlistRequiresConfirm(t *testing.T) {
	pe := &PolicyEngine{
		AllowedCommands: map[string]map[string]bool{
			"ls": {},
		},
	}

	ir := emptyIR(PlatformUnix)
	ir.Commands = []string{"ls", "rm"}
	ir.RiskFlags = []ReasonCode{ReasonOperator}

	d := pe.Decide(ir)
	if !d.NeedsConfirmation {
		t.Error("expected NeedsConfirmation: rm is not in allow-list")
	}
}

func TestPolicyEngine_Decide_VersionMismatch(t *testing.T) {
	pe := &PolicyEngine{}
	ir := emptyIR(PlatformUnix)
	ir.Version = "99"
	d := pe.Decide(ir)
	if !d.NeedsConfirmation {
		t.Error("expected NeedsConfirmation for version mismatch")
	}
}

func TestPolicyEngine_Decide_EmptyCommandsNoConfirm(t *testing.T) {
	pe := &PolicyEngine{}
	ir := emptyIR(PlatformUnix)
	// No commands, no risk flags — unusual but should not confirm.
	d := pe.Decide(ir)
	if d.NeedsConfirmation {
		t.Error("empty IR with no risk flags should not require confirmation")
	}
}

func TestPolicyEngine_Decide_UnknownCommandRequiresConfirm(t *testing.T) {
	pe := &PolicyEngine{}
	ir := emptyIR(PlatformUnix)
	ir.Commands = []string{"wget"}
	d := pe.Decide(ir)
	if !d.NeedsConfirmation {
		t.Error("expected NeedsConfirmation for unknown command")
	}
}

// TestPolicyEngine_FlagEnforcement verifies that the policy engine validates
// flags against the stored allow-list, mirroring legacy BashAnalyzer behaviour.
// A previously-approved "find ." must NOT silently permit "find . -exec rm -rf".
func TestPolicyEngine_FlagEnforcement(t *testing.T) {
	t.Run("unapproved flag blocks auto-approve", func(t *testing.T) {
		// Only "find" with no flags was stored in the allow-list.
		pe := &PolicyEngine{
			AllowedCommands: map[string]map[string]bool{
				"find": {},
			},
		}
		ir := emptyIR(PlatformUnix)
		ir.Commands = []string{"find"}
		ir.CommandArgs = map[string][]string{
			"find": {"-exec"},
		}

		d := pe.Decide(ir)
		if !d.NeedsConfirmation {
			t.Error("expected NeedsConfirmation: -exec was not in stored allow-list")
		}
	})

	t.Run("approved flag permits auto-approve", func(t *testing.T) {
		pe := &PolicyEngine{
			AllowedCommands: map[string]map[string]bool{
				"find": {"-name": true, "-type": true},
			},
		}
		ir := emptyIR(PlatformUnix)
		ir.Commands = []string{"find"}
		ir.CommandArgs = map[string][]string{
			"find": {"-name", "-type"},
		}

		d := pe.Decide(ir)
		if d.NeedsConfirmation {
			t.Error("expected auto-approve: all flags are in the stored allow-list")
		}
	})

	t.Run("no flags used auto-approves bare command", func(t *testing.T) {
		pe := &PolicyEngine{
			AllowedCommands: map[string]map[string]bool{
				"find": {},
			},
		}
		ir := emptyIR(PlatformUnix)
		ir.Commands = []string{"find"}
		// CommandArgs empty — bare "find ." with no flags.

		d := pe.Decide(ir)
		if d.NeedsConfirmation {
			t.Error("expected auto-approve for bare allow-listed command with no flags")
		}
	})

	t.Run("pipe: unapproved flag on one command blocks", func(t *testing.T) {
		pe := &PolicyEngine{
			AllowedCommands: map[string]map[string]bool{
				"find": {},
				"grep": {"-r": true},
			},
		}
		ir := emptyIR(PlatformUnix)
		ir.Commands = []string{"find", "grep"}
		ir.Operators = []string{"|"}
		ir.RiskFlags = []ReasonCode{ReasonOperator}
		ir.CommandArgs = map[string][]string{
			"find": {"-exec"}, // not in stored flags → must block
			"grep": {"-r"},
		}

		d := pe.Decide(ir)
		if !d.NeedsConfirmation {
			t.Error("expected NeedsConfirmation: find -exec not in stored allow-list")
		}
	})

	t.Run("unix: find -exec rm -rf parsed as unapproved flag", func(t *testing.T) {
		// End-to-end through the Unix parser.
		pe := &PolicyEngine{
			AllowedCommands: map[string]map[string]bool{
				"find": {}, // only bare find was approved
			},
		}
		p := &UnixParser{}
		ir, err := p.Parse(`find . -exec rm -rf {} \;`)
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}

		d := pe.Decide(ir)
		if !d.NeedsConfirmation {
			t.Error("expected NeedsConfirmation: -exec not in stored allow-list")
		}
	})
}
