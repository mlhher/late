package orchestrator

import (
	"context"
	"late/internal/client"
	"late/internal/common"
	"late/internal/executor"
	"late/internal/session"
	"sync"
)

// BaseOrchestrator implements common.Orchestrator and manages an agent's run loop.
type BaseOrchestrator struct {
	id          string
	sess        *session.Session
	middlewares []common.ToolMiddleware
	eventCh     chan common.Event

	mu       sync.RWMutex
	parent   common.Orchestrator
	children []common.Orchestrator

	// Running state tracker
	acc    executor.StreamAccumulator
	ctx    context.Context
	cancel context.CancelFunc
}

func NewBaseOrchestrator(id string, sess *session.Session, middlewares []common.ToolMiddleware) *BaseOrchestrator {
	return &BaseOrchestrator{
		id:          id,
		sess:        sess,
		middlewares: middlewares,
		eventCh:     make(chan common.Event, 100),
		ctx:         context.Background(),
	}
}

func (o *BaseOrchestrator) SetMiddlewares(middlewares []common.ToolMiddleware) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.middlewares = middlewares
}

func (o *BaseOrchestrator) SetContext(ctx context.Context) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.ctx = ctx
}

func (o *BaseOrchestrator) ID() string { return o.id }

func (o *BaseOrchestrator) Submit(text string) error {
	if err := o.sess.AddUserMessage(text); err != nil {
		return err
	}

	o.eventCh <- common.StatusEvent{ID: o.id, Status: "thinking"}
	// Start the run loop in a background goroutine
	go o.run()
	return nil
}

func (o *BaseOrchestrator) Execute(text string) (string, error) {
	ctx, cancel := context.WithCancel(o.ctx)

	o.mu.Lock()
	o.cancel = cancel
	o.mu.Unlock()

	// Inject orchestrator ID into context for tool interactions
	ctx = context.WithValue(ctx, common.OrchestratorIDKey, o.id)

	if err := o.sess.AddUserMessage(text); err != nil {
		return "", err
	}

	o.eventCh <- common.StatusEvent{ID: o.id, Status: "thinking"}
	defer func() {
		o.eventCh <- common.StatusEvent{ID: o.id, Status: "idle"}
	}()

	// Build extra body
	extraBody := map[string]any{
		"id_slot":      1,
		"cache_prompt": false,
	}

	onStartTurn := func() {
		o.mu.Lock()
		o.acc.Reset()
		o.mu.Unlock()
		o.eventCh <- common.StatusEvent{ID: o.id, Status: "thinking"}
	}

	onEndTurn := func() {
		o.mu.Lock()
		o.acc.Reset()
		o.mu.Unlock()
		o.eventCh <- common.ContentEvent{ID: o.id}
	}

	res, err := executor.RunLoop(
		ctx,
		o.sess,
		20, // maxTurns
		extraBody,
		onStartTurn,
		onEndTurn,
		func(res common.StreamResult) {
			o.mu.Lock()
			o.acc.Append(res)
			accCopy := o.acc
			o.mu.Unlock()

			o.eventCh <- common.ContentEvent{
				ID:               o.id,
				Content:          accCopy.Content,
				ReasoningContent: accCopy.Reasoning,
				ToolCalls:        accCopy.ToolCalls,
			}
		},
		o.middlewares,
	)

	if err != nil {
		o.eventCh <- common.StatusEvent{ID: o.id, Status: "error", Error: err}
	} else {
		o.eventCh <- common.StatusEvent{ID: o.id, Status: "closed"}
	}
	return res, err
}

func (o *BaseOrchestrator) run() {
	// Build extra body
	extraBody := map[string]any{
		"id_slot":      1,
		"cache_prompt": false,
	}

	ctx, cancel := context.WithCancel(o.ctx)

	o.mu.Lock()
	o.cancel = cancel
	o.mu.Unlock()

	// Inject orchestrator ID into context for tool interactions
	ctx = context.WithValue(ctx, common.OrchestratorIDKey, o.id)

	onStartTurn := func() {
		o.mu.Lock()
		o.acc.Reset()
		o.mu.Unlock()
		o.eventCh <- common.StatusEvent{ID: o.id, Status: "thinking"}
	}

	onEndTurn := func() {
		o.mu.Lock()
		o.acc.Reset()
		o.mu.Unlock()
		o.eventCh <- common.ContentEvent{ID: o.id}
	}

	_, err := executor.RunLoop(
		ctx,
		o.sess,
		20, // maxTurns
		extraBody,
		onStartTurn,
		onEndTurn,
		func(res common.StreamResult) {
			o.mu.Lock()
			o.acc.Append(res)
			accCopy := o.acc // Copy for event
			o.mu.Unlock()

			o.eventCh <- common.ContentEvent{
				ID:               o.id,
				Content:          accCopy.Content,
				ReasoningContent: accCopy.Reasoning,
				ToolCalls:        accCopy.ToolCalls,
			}
		},
		o.middlewares,
	)

	// Reset accumulator after finished or ready for next turn
	o.mu.Lock()
	o.acc.Reset()
	o.mu.Unlock()

	if err != nil {
		o.eventCh <- common.StatusEvent{ID: o.id, Status: "error", Error: err}
	} else {
		o.eventCh <- common.StatusEvent{ID: o.id, Status: "idle"}
	}
}

func (o *BaseOrchestrator) Events() <-chan common.Event {
	return o.eventCh
}

func (o *BaseOrchestrator) Cancel() {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.cancel != nil {
		o.cancel()
	}
}

func (o *BaseOrchestrator) History() []client.ChatMessage {
	return o.sess.History
}

func (o *BaseOrchestrator) Context() context.Context {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.ctx
}

func (o *BaseOrchestrator) Middlewares() []common.ToolMiddleware {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.middlewares
}

func (o *BaseOrchestrator) Registry() *common.ToolRegistry {
	return o.sess.Registry
}

func (o *BaseOrchestrator) Children() []common.Orchestrator {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.children
}

func (o *BaseOrchestrator) Parent() common.Orchestrator {
	return o.parent
}

func (o *BaseOrchestrator) Reset() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sess.History = []client.ChatMessage{}
	return session.SaveHistory(o.sess.HistoryPath, nil)
}

func (o *BaseOrchestrator) AddChild(child common.Orchestrator) {
	o.mu.Lock()
	o.children = append(o.children, child)
	o.mu.Unlock()

	o.eventCh <- common.ChildAddedEvent{
		ParentID: o.id,
		Child:    child,
	}
}
