package tui

import (
	"encoding/json"
	"fmt"

	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.Width == 0 || m.Height == 0 {
		return ""
	}

	baseView := appStyle.Width(m.Width).Height(m.Height).Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			m.Viewport.View(),
			m.inputView(),
			m.statusBarView(),
		),
	)

	return baseView
}

func (m *Model) inputView() string {
	w := m.Width - 4 // Internal padding for input
	if w < 1 {
		w = 1
	}
	bgColor := lipgloss.Color("#191919")

	// Render textarea directly — its styles already set background via FocusedStyle/BlurredStyle
	textareaView := m.Input.View()
	content := inputStyle.Width(w - 2).Render(textareaView)

	// Wrap in a fixed-size container that fills the background
	return lipgloss.NewStyle().
		Width(m.Width).
		Height(InputHeight).
		Background(bgColor).
		Padding(0, 2).
		AlignVertical(lipgloss.Bottom).
		Render(content)
}

func (m *Model) statusBarView() string {
	w := max(m.Width, 1)

	s := m.GetAgentState(m.Focused.ID())

	modeStr := " CHAT "
	statusText := s.StatusText

	switch s.State {
	case StateThinking:
		modeStr = " THINKING "
	case StateStreaming:
		modeStr = " STREAMING "
	case StateConfirmTool:
		modeStr = " CONFIRM "
		statusText = "Authorize Tool Execution (y/n)"
	}

	mode := statusModeStyle.Render(modeStr)
	status := statusTextStyle.Render(statusText)

	// Build key hints
	stopKey := statusKeyStyle.Render("Ctrl+g") + " Stop "

	// Add hierarchy hints
	var hierarchyHint string
	if m.Focused.Parent() != nil {
		hierarchyHint = statusKeyStyle.Render("Esc") + " Back "
	}
	if len(m.Focused.Children()) > 0 {
		hierarchyHint += statusKeyStyle.Render("Tab") + " Subagents "
	}

	hints := lipgloss.JoinHorizontal(lipgloss.Left, hierarchyHint, stopKey)

	spaceWidth := w - lipgloss.Width(mode) - lipgloss.Width(status) - lipgloss.Width(hints)
	if spaceWidth < 0 {
		spaceWidth = 0
	}
	space := strings.Repeat(" ", spaceWidth)

	content := lipgloss.JoinHorizontal(lipgloss.Left, mode, status, space, hints)
	return statusBarBaseStyle.Width(w).Render(content)
}

func (m *Model) updateViewport() {
	if m.Focused == nil {
		return
	}

	history := m.Focused.History()
	msgWidth := m.Viewport.Width - 2
	if msgWidth < 1 {
		msgWidth = 80
	}

	// Simplified history rendering (symmetric for all agents)
	var blocks []string
	for _, msg := range history {
		switch msg.Role {
		case "user":
			blocks = append(blocks, userMsgStyle.Width(msgWidth).Render(msg.Content))
		case "assistant":
			var assistantParts []string
			if msg.ReasoningContent != "" {
				assistantParts = append(assistantParts, tagStyle.Width(msgWidth+1).Render("Thinking Process:"))
				assistantParts = append(assistantParts, thinkingStyle.Width(msgWidth-2).Render(msg.ReasoningContent))
			}
			if msg.Content != "" {
				md, _ := m.Renderer.Render(msg.Content)
				assistantParts = append(assistantParts, aiMsgStyle.Width(msgWidth).Render(strings.TrimRight(md, "\n")))
			}
			for _, tc := range msg.ToolCalls {
				// Try to use CallString() for meaningful display
				callStr := tc.Function.Name
				if registry := m.Root.Registry(); registry != nil {
					if tool := registry.Get(tc.Function.Name); tool != nil {
						if args := json.RawMessage(tc.Function.Arguments); len(args) > 0 {
							callStr = tool.CallString(args)
						}
					}
				}
				assistantParts = append(assistantParts, tagStyle.Width(msgWidth+1).Render(fmt.Sprintf("◆ %s", callStr)))
			}
			blocks = append(blocks, lipgloss.JoinVertical(lipgloss.Left, assistantParts...))
		}
	}

	s := m.GetAgentState(m.Focused.ID())

	// Render streaming content if active
	// Dedup check: Only render streaming if NOT in an interaction state (where history already has the tools)
	if (s.State == StateStreaming || s.State == StateThinking) && s.State != StateConfirmTool {
		var activeParts []string
		if s.StreamingState.ReasoningContent != "" {
			activeParts = append(activeParts, tagStyle.Width(msgWidth+1).Render("Thinking Process:"))
			activeParts = append(activeParts, thinkingStyle.Width(msgWidth-2).Render(s.StreamingState.ReasoningContent))
		}
		if s.StreamingState.Content != "" {
			md, _ := m.Renderer.Render(s.StreamingState.Content)
			activeParts = append(activeParts, aiMsgStyle.Width(msgWidth).Render(strings.TrimRight(md, "\n")))
		}
		for _, tc := range s.StreamingState.ToolCalls {
			// Try to use CallString() for meaningful display (no trailing ... since CallString adds it)
			callStr := tc.Function.Name
			if registry := m.Root.Registry(); registry != nil {
				if tool := registry.Get(tc.Function.Name); tool != nil {
					if args := json.RawMessage(tc.Function.Arguments); len(args) > 0 {
						callStr = tool.CallString(args)
					}
				}
			}
			activeParts = append(activeParts, tagStyle.Width(msgWidth+1).Render(fmt.Sprintf("%s %s", m.Spinner.View(), callStr)))
		}
		if len(activeParts) > 0 {
			blocks = append(blocks, lipgloss.JoinVertical(lipgloss.Left, activeParts...))
		} else if s.State == StateThinking {
			blocks = append(blocks, thinkingStyle.Render("Thinking..."))
		}
	}

	// Render Interactions
	if s.State == StateConfirmTool && s.PendingConfirm != nil {
		tc := s.PendingConfirm.ToolCall
		prompt := fmt.Sprintf("The agent wants to execute **%s**.\n\n```json\n%s\n```\n\n> Press **[ y ]** to Approve  |  **[ n ]** to Deny", tc.Function.Name, tc.Function.Arguments)
		md, _ := m.Renderer.Render(prompt)
		blocks = append(blocks, aiMsgStyle.Width(msgWidth).Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("#FFD700")).Render(md))
	}

	if m.Err != nil {
		blocks = append(blocks, thinkingStyle.Foreground(lipgloss.Color("#FF0000")).Render(fmt.Sprintf("Error: %v", m.Err)))
	}

	fullContent := lipgloss.JoinVertical(lipgloss.Left, blocks...)
	atBottom := m.Viewport.AtBottom()
	m.Viewport.SetContent(fullContent)
	if atBottom {
		m.Viewport.GotoBottom()
	}
}
