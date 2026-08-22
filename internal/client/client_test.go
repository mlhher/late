package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSupportsVisionOverride(t *testing.T) {
	// Case 1: Config without EnableImages, and supportsVision is false
	c1 := NewClient(Config{
		BaseURL:      "http://localhost:8080",
		EnableImages: false,
	})
	if c1.SupportsVision() {
		t.Errorf("expected SupportsVision() to be false when EnableImages is false and backend support is unknown/false")
	}

	// Case 2: Config with EnableImages = true
	c2 := NewClient(Config{
		BaseURL:      "http://localhost:8080",
		EnableImages: true,
	})
	if !c2.SupportsVision() {
		t.Errorf("expected SupportsVision() to be true when EnableImages is true")
	}

	// Case 3: Config without EnableImages, but supportsVision is true
	c3 := NewClient(Config{
		BaseURL: "http://localhost:8080",
	})
	c3.supportsVision = true
	if !c3.SupportsVision() {
		t.Errorf("expected SupportsVision() to be true when c.supportsVision is true")
	}
}

// Sample SSE chunk JSON strings shared across stream tests.
const (
	sampleChunkHello = `{"id":"c1","choices":[{"delta":{"content":"Hello"}}]}`
	sampleChunkWorld = `{"id":"c1","choices":[{"delta":{"content":" world"}}]}`
	sampleChunkStop  = `{"id":"c1","choices":[{"delta":{"content":""},"finish_reason":"stop"}]}`
)

// defaultRequest returns a minimal ChatCompletionRequest used by most stream tests.
func defaultRequest() ChatCompletionRequest {
	return ChatCompletionRequest{
		Model:    "test-model",
		Messages: []ChatMessage{{Role: "user", Content: TextContent("hi")}},
	}
}

// streamTest is a shared test fixture for ChatCompletionStream tests.
type streamTest struct {
	server *httptest.Server
	client *Client
}

// newStreamTest creates a test server and client pair. The caller should call
// st.Handle() to set the response handler before making requests.
func newStreamTest(t *testing.T) *streamTest {
	t.Helper()
	st := &streamTest{
		server: httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
		})),
	}
	st.client = NewClient(Config{BaseURL: st.server.URL})
	return st
}

// Handle replaces the server's response handler.
func (st *streamTest) Handle(handler http.HandlerFunc) {
	st.server.Config.Handler = handler
}

// Close shuts down the test server. Call via defer immediately after creation.
func (st *streamTest) Close() {
	st.server.Close()
}

// collectStream drains the output/error channels from ChatCompletionStream
// into a slice of ChatCompletionChunk and an optional error.
func collectStream(t *testing.T, ctx context.Context, c *Client, req ChatCompletionRequest) ([]ChatCompletionChunk, error) {
	t.Helper()
	outCh, errCh := c.ChatCompletionStream(ctx, req)

	var chunks []ChatCompletionChunk
	var streamErr error

	// Collect in a goroutine to avoid deadlocks if channels don't close.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for chunk := range outCh {
			chunks = append(chunks, chunk)
		}
		select {
		case err, ok := <-errCh:
			if ok && err != nil {
				streamErr = err
			}
		default:
		}
	}()

	// Wait with a timeout to catch hangs.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stream collection timed out — channels were not closed")
	}

	return chunks, streamErr
}

// sseChunks builds an SSE-formatted response body from JSON strings.
func sseChunks(jsons ...string) string {
	var b strings.Builder
	for _, j := range jsons {
		fmt.Fprintf(&b, "data: %s\n", j)
	}
	return b.String()
}

// accumulateContent concatenates the delta content strings from all choices across chunks.
func accumulateContent(chunks []ChatCompletionChunk) string {
	var b strings.Builder
	for _, ch := range chunks {
		if len(ch.Choices) > 0 {
			b.WriteString(ch.Choices[0].Delta.Content.String())
		}
	}
	return b.String()
}

func TestChatCompletionStream_Termination(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantChunks int
		wantErr    bool
	}{
		{
			name:       "DONE_sentinel",
			body:       sseChunks(sampleChunkHello, "[DONE]"),
			wantChunks: 1,
			wantErr:    false,
		},
		{
			name:       "connection_close_no_sentinel",
			body:       sseChunks(sampleChunkHello),
			wantChunks: 1,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newStreamTest(t)
			defer st.Close()

			st.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tt.body)
			}))

			chunks, err := collectStream(t, context.Background(), st.client, defaultRequest())

			if got := len(chunks); got != tt.wantChunks {
				t.Errorf("got %d chunks, want %d", got, tt.wantChunks)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestChatCompletionStream_ContentAccumulation(t *testing.T) {
	st := newStreamTest(t)
	defer st.Close()

	chunkPayloads := []string{
		sampleChunkHello,
		sampleChunkWorld,
		sampleChunkStop,
	}

	st.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseChunks(chunkPayloads...), "\ndata: [DONE]\n\n")
	}))

	resultChunks, err := collectStream(t, context.Background(), st.client, defaultRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify chunk count (3 data lines, none are [DONE])
	if got := len(resultChunks); got != 3 {
		t.Errorf("got %d chunks, want 3", got)
	}

	// Verify content accumulation
	if got := accumulateContent(resultChunks); got != "Hello world" {
		t.Errorf("accumulated content = %q, want %q", got, "Hello world")
	}

	// Verify finish reason on last chunk
	if resultChunks[2].Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason = %q, want %q", resultChunks[2].Choices[0].FinishReason, "stop")
	}
}

func TestChatCompletionStream_Non200Status(t *testing.T) {
	st := newStreamTest(t)
	defer st.Close()

	st.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"message":"internal error"}}`)
	}))

	_, err := collectStream(t, context.Background(), st.client, defaultRequest())
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

func TestChatCompletionStream_InvalidJSON(t *testing.T) {
	st := newStreamTest(t)
	defer st.Close()

	st.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseChunks("{invalid json}", `{"id":"c1","choices":[{"delta":{"content":"valid"}}]}`, "[DONE]"))
	}))

	chunks, err := collectStream(t, context.Background(), st.client, defaultRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have 1 valid chunk (the invalid JSON was skipped).
	if got := len(chunks); got != 1 {
		t.Errorf("got %d chunks, want 1 (invalid JSON should be skipped)", got)
	}
}
