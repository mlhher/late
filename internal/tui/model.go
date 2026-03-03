package tui

import (
	"late/internal/common"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

func NewModel(root common.Orchestrator, renderer *glamour.TermRenderer) Model {
	ti := textarea.New()
	ti.Placeholder = "Ask Late anything..."
	ti.Focus()
	ti.CharLimit = 2000
	ti.SetWidth(72)
	ti.SetHeight(InputHeight - 2)
	ti.ShowLineNumbers = false
	ti.Prompt = ""    // Remove the line prompt characters
	ti.SetValue("> ") // Set initial "fake" prompt to force background render logic on first line
	ti.KeyMap.InsertNewline.SetEnabled(false)

	// Set opaque background for textarea content
	bgStyle := lipgloss.NewStyle().Background(lipgloss.Color("#191919")).Foreground(textColor)
	ti.FocusedStyle.Base = bgStyle
	ti.FocusedStyle.Text = bgStyle
	ti.FocusedStyle.Placeholder = bgStyle.Foreground(lipgloss.Color("#666666"))
	ti.FocusedStyle.CursorLine = bgStyle
	ti.FocusedStyle.Prompt = bgStyle

	ti.BlurredStyle.Base = bgStyle
	ti.BlurredStyle.Text = bgStyle
	ti.BlurredStyle.Placeholder = bgStyle.Foreground(lipgloss.Color("#666666"))
	ti.BlurredStyle.CursorLine = bgStyle
	ti.BlurredStyle.Prompt = bgStyle

	// Initialize with 0, so that the first WindowSizeMsg sets correct dimensions
	// This prevents the "50% width" issue if the default 60 is too small for a large terminal
	vp := viewport.New(0, 0)
	vp.SetContent("Welcome to Late. Type your prompt below.")

	// Determine active state
	initialState := StateIdle
	if root.History() != nil && len(root.History()) > 0 {
		last := root.History()[len(root.History())-1]
		if last.Role == "assistant" && len(last.ToolCalls) > 0 {
			// Check if we are waiting for a tool result?
			// For now, default to thinking if history exists, or idle.
		}
	}

	m := Model{
		Mode:           ViewChat,
		Root:           root,
		Focused:        root,
		Input:          ti,
		Viewport:       vp,
		Renderer:       renderer,
		Width:          80,
		Height:         24, // Default start height
		AgentStates:    make(map[string]*AppState),
		InspectingTool: false,
		Spinner:        spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	// Initialize root state
	m.AgentStates[root.ID()] = &AppState{
		State:      initialState,
		StatusText: "Ready",
	}

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.Spinner.Tick)
}
