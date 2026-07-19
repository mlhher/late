package agent

import (
	"encoding/json"
	"fmt"
	"late/internal/assets"
	"late/internal/client"
	"late/internal/common"
	"late/internal/executor"
	"late/internal/orchestrator"
	"late/internal/session"
	"late/internal/tui"
	"os"
)

// NewSubagentOrchestrator creates a new BaseOrchestrator for a subagent.
func NewSubagentOrchestrator(
	c *client.Client,
	goal string,
	ctxFiles []string,
	agentType string,
	enabledTools map[string]bool,
	injectCWD bool,
	gemmaThinking bool,
	maxTurns int,
	parent common.Orchestrator,
	messenger tui.Messenger,
) (common.Orchestrator, error) {
	// 1. Determine System Prompt
	configs := assets.GetSubagents()
	var config *assets.SubagentConfig
	for _, c := range configs {
		if c.Name == agentType {
			temp := c
			config = &temp
			break
		}
	}

	if config == nil {
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}

	content, err := assets.PromptsFS.ReadFile(config.PromptFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded subagent prompt: %w", err)
	}
	systemPrompt := string(content)

	if injectCWD {
		cwd, err := os.Getwd()
		if err == nil {
			systemPrompt = common.ReplacePlaceholders(systemPrompt, map[string]string{
				"${{CWD}}": cwd,
			})
		}
	}

	if gemmaThinking {
		systemPrompt = "<|think|>" + systemPrompt
	}

	// 2. Setup Subagent Session (Isolated History)
	sess := session.New(c, "", []client.ChatMessage{}, systemPrompt, true)

	// Inherit all tools from parent (including MCP tools)
	if parent != nil && parent.Registry() != nil {
		for _, t := range parent.Registry().All() {
			// Skip spawn_subagent and write_implementation_plan to prevent recursion/confusion
			name := t.Name()
			if name == "spawn_subagent" || name == "write_implementation_plan" {
				continue
			}
			sess.Registry.Register(t)
		}
	}

	// Register explicitly allowed tools for the subagent
	subagentTools := make(map[string]bool)
	for _, t := range config.AllowedTools {
		if enabledTools[t] { // only enable if it's also enabled globally
			subagentTools[t] = true
		}
	}
	executor.RegisterTools(sess.Registry, subagentTools)

	// 3. Construct Initial Context
	initialMsg := fmt.Sprintf("Goal: %s\n\n", goal)
	if len(ctxFiles) > 0 {
		initialMsg += "Context Files:\n"
		for _, f := range ctxFiles {
			content, err := os.ReadFile(f)
			if err == nil {
				initialMsg += fmt.Sprintf("- %s:\n```\n%s\n```\n", f, string(content))
			}
		}
	}

	if err := sess.AddUserMessage(initialMsg); err != nil {
		return nil, fmt.Errorf("failed to add initial message: %w", err)
	}

	// 4. Create Orchestrator
	id := fmt.Sprintf("%s-subagent-%d", agentType, len(parent.Children()))
	mws := parent.Middlewares()

	if messenger != nil {
		mws = []common.ToolMiddleware{
			tui.TUIConfirmMiddleware(messenger, sess.Registry),
		}
	}

	child := orchestrator.NewBaseOrchestrator(id, sess, mws, maxTurns)
	child.SetContext(parent.Context())

	if p, ok := parent.(*orchestrator.BaseOrchestrator); ok {
		p.AddChild(child)
	}

	return child, nil
}

func FormatToolConfirmPrompt(tc client.ToolCall) string {
	var jsonObj map[string]interface{}
	args := tc.Function.Arguments
	if err := json.Unmarshal([]byte(args), &jsonObj); err == nil {
		pretty, _ := json.MarshalIndent(jsonObj, "", "  ")
		args = string(pretty)
	}
	return fmt.Sprintf("Execute **%s**:\n\n```json\n%s\n```", tc.Function.Name, args)
}
