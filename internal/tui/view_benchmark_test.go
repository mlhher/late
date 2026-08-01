package tui

import (
	"fmt"
	"late/internal/client"
	"late/internal/common"
	"strings"
	"testing"
)

// benchmarkOrchestrator supplies mutable history while reusing the test
// orchestrator implementation shared by the tui test package.
type benchmarkOrchestrator struct {
	mockOrchestrator
	history []client.ChatMessage
}

func (o *benchmarkOrchestrator) History() []client.ChatMessage {
	return o.history
}

func benchmarkHistory(messageCount int) []client.ChatMessage {
	history := make([]client.ChatMessage, 0, messageCount)
	for i := 0; i < messageCount; i++ {
		if i%2 == 0 {
			history = append(history, client.ChatMessage{
				Role:    "user",
				Content: client.TextContent(fmt.Sprintf("Question %d: explain the relevant behavior in a concise way.", i/2)),
			})
			continue
		}
		history = append(history, client.ChatMessage{
			Role: "assistant",
			Content: client.TextContent(
				"Here is the answer.\n\nIt contains enough text to wrap across terminal lines and exercise the cached viewport composition path.",
			),
		})
	}
	return history
}

func newViewportBenchmarkModel(history []client.ChatMessage) (*Model, *AppState) {
	orch := &benchmarkOrchestrator{history: history}
	model := NewModel(orch, nil, nil)
	model.Width = 100
	model.Height = 40
	model.updateLayout()

	state := model.GetAgentState(orch.ID())
	return &model, state
}

// BenchmarkUpdateViewportCachedHistory exposes work that grows with completed
// chat history even after every historical Markdown block has been cached.
func BenchmarkUpdateViewportCachedHistory(b *testing.B) {
	for _, messageCount := range []int{10, 100, 500, 1000} {
		b.Run(fmt.Sprintf("messages_%d", messageCount), func(b *testing.B) {
			model, state := newViewportBenchmarkModel(benchmarkHistory(messageCount))
			state.State = StateStreaming

			// Populate RenderedHistory before timing. Only the active tail changes
			// during the benchmark.
			state.StreamingState = common.ContentEvent{ID: model.Focused.ID(), Content: "streaming a"}
			model.updateViewport()
			variants := [...]string{"streaming a", "streaming b"}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state.StreamingState.Content = variants[i&1]
				model.updateViewport()
			}
		})
	}
}

// BenchmarkUpdateViewportStreamingTail exposes rescanning and restyling of a
// single growing/incomplete response independently of completed history.
func BenchmarkUpdateViewportStreamingTail(b *testing.B) {
	for _, size := range []int{1 << 10, 16 << 10, 64 << 10} {
		b.Run(fmt.Sprintf("bytes_%d", size), func(b *testing.B) {
			model, state := newViewportBenchmarkModel(nil)
			state.State = StateStreaming

			prefix := strings.Repeat("streaming text ", size/len("streaming text "))
			variants := [...]string{prefix + "a", prefix + "b"}
			state.StreamingState = common.ContentEvent{ID: model.Focused.ID(), Content: variants[0]}
			model.updateViewport()

			b.ReportAllocs()
			b.SetBytes(int64(len(variants[0])))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state.StreamingState.Content = variants[i&1]
				model.updateViewport()
			}
		})
	}
}

// BenchmarkUpdateViewportStreamingReasoning measures the uncached reasoning
// path, which currently renders the complete reasoning text on every frame.
func BenchmarkUpdateViewportStreamingReasoning(b *testing.B) {
	for _, size := range []int{1 << 10, 16 << 10, 64 << 10} {
		b.Run(fmt.Sprintf("bytes_%d", size), func(b *testing.B) {
			model, state := newViewportBenchmarkModel(nil)
			state.State = StateStreaming

			prefix := strings.Repeat("reasoning text ", size/len("reasoning text "))
			variants := [...]string{prefix + "a", prefix + "b"}
			state.StreamingState = common.ContentEvent{ID: model.Focused.ID(), ReasoningContent: variants[0]}
			model.updateViewport()

			b.ReportAllocs()
			b.SetBytes(int64(len(variants[0])))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state.StreamingState.ReasoningContent = variants[i&1]
				model.updateViewport()
			}
		})
	}
}

// BenchmarkUpdateViewportHistoryCacheReset captures the end-of-turn pause
// caused when status handling clears RenderedHistory and every completed
// Markdown message must be rendered again.
func BenchmarkUpdateViewportHistoryCacheReset(b *testing.B) {
	for _, messageCount := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("messages_%d", messageCount), func(b *testing.B) {
			model, state := newViewportBenchmarkModel(benchmarkHistory(messageCount))
			model.updateViewport()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state.RenderedHistory = nil
				state.LastTotalContent = ""
				model.updateViewport()
			}
		})
	}
}

func TestStreamingViewportUsesBoundedHistoryWindow(t *testing.T) {
	model, state := newViewportBenchmarkModel(benchmarkHistory(1000))
	fullLineCount := len(state.CachedHistoryLines)
	state.State = StateStreaming
	state.StreamingState = common.ContentEvent{ID: model.Focused.ID(), Content: "active response"}

	model.updateViewport()

	if !state.StreamingWindow {
		t.Fatal("expected streaming viewport to use a bounded history window")
	}
	if state.StreamingWindowStart <= 0 {
		t.Fatal("expected a long history to be trimmed from the streaming viewport")
	}
	if got := strings.Count(model.Viewport.GetContent(), "\n") + 1; got >= fullLineCount {
		t.Fatalf("streaming viewport contains %d lines; expected fewer than full history's %d", got, fullLineCount)
	}
}

func TestRestoreFullHistoryForScroll(t *testing.T) {
	model, state := newViewportBenchmarkModel(benchmarkHistory(500))
	state.State = StateStreaming
	state.StreamingState = common.ContentEvent{ID: model.Focused.ID(), Content: "active response"}
	model.updateViewport()
	windowedLines := strings.Count(model.Viewport.GetContent(), "\n") + 1

	model.restoreFullHistoryForScroll()

	if state.StreamingWindow {
		t.Fatal("expected upward scrolling to leave streaming-window mode")
	}
	if got := strings.Count(model.Viewport.GetContent(), "\n") + 1; got <= windowedLines {
		t.Fatalf("restored viewport contains %d lines; expected more than windowed viewport's %d", got, windowedLines)
	}

	model.Viewport.ScrollUp(1)
	before := model.Viewport.GetContent()
	state.StreamingState.Content = "new content that arrived while reading history"
	model.updateViewport()
	if got := model.Viewport.GetContent(); got != before {
		t.Fatal("streaming update replaced the viewport while the user was reading older history")
	}
}

func TestIdleStatusKeepsRenderedHistoryCache(t *testing.T) {
	model, state := newViewportBenchmarkModel(benchmarkHistory(100))
	model.updateViewport()
	cached := len(state.RenderedHistory)
	if cached == 0 {
		t.Fatal("expected rendered history cache to be populated")
	}

	updated, _ := model.updateInternal(OrchestratorEventMsg{Event: common.StatusEvent{
		ID:     model.Focused.ID(),
		Status: "idle",
	}})

	if got := len(updated.GetAgentState(updated.Focused.ID()).RenderedHistory); got != cached {
		t.Fatalf("idle status retained %d cached messages; want %d", got, cached)
	}
}
