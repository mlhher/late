package orchestrator

import (
	"context"
	"fmt"
	"late/internal/archive"
	"late/internal/client"
	"late/internal/common"
	"late/internal/config"
	"late/internal/executor"
	"late/internal/session"
	"late/internal/tool"
	"log"
	"os"
	"sync"
	"time"
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

	// Stop mechanism
	stopCh chan struct{}

	// Max turns configuration
	maxTurns int

	// Archive subsystem (nil when compaction is disabled)
	archiveSub *archiveState
}

// archiveState holds loaded archive and search service for one session run.
type archiveState struct {
	sub *tool.ArchiveSubsystem
	cfg config.ArchiveCompactionConfig
}

func NewBaseOrchestrator(id string, sess *session.Session, middlewares []common.ToolMiddleware, maxTurns int) *BaseOrchestrator {
	return &BaseOrchestrator{
		id:          id,
		sess:        sess,
		middlewares: middlewares,
		eventCh:     make(chan common.Event, 100),
		ctx:         context.Background(),
		stopCh:      make(chan struct{}),
		maxTurns:    maxTurns,
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

func (o *BaseOrchestrator) SetMaxTurns(maxTurns int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.maxTurns = maxTurns
}

func (o *BaseOrchestrator) MaxTokens() int {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.sess.Client().ContextSize()
}

func (o *BaseOrchestrator) RefreshContextSize(ctx context.Context) {
	o.sess.Client().RefreshContextSize(ctx)
}

func (o *BaseOrchestrator) ID() string { return o.id }

func (o *BaseOrchestrator) Submit(text string) error {
	o.mu.Lock()
	// Clear any old cancellation state so a new run isn't instantly aborted
	o.cancel = nil
	// Reset the base context if it was already cancelled
	if o.ctx.Err() != nil {
		o.ctx = context.Background()
	}
	o.mu.Unlock()

	if err := o.sess.AddUserMessage(text); err != nil {
		return err
	}

	o.eventCh <- common.StatusEvent{ID: o.id, Status: "thinking"}
	// Start the run loop in a background goroutine
	go o.run()
	return nil
}

func (o *BaseOrchestrator) Execute(text string) (string, error) {
	o.mu.Lock()
	if o.ctx.Err() != nil {
		o.ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(o.ctx)
	o.cancel = cancel
	o.ctx = ctx // Set the Context for this execution
	o.mu.Unlock()

	defer cancel()

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
	var extraBody map[string]any

	// Pre-run archive compaction hook (fail-open).
	o.runArchivePreHook()

	onStartTurn := func() {
		o.RefreshContextSize(ctx)
		o.mu.Lock()
		o.acc.Reset()
		o.mu.Unlock()
		o.eventCh <- common.StatusEvent{ID: o.id, Status: "thinking"}
	}

	onEndTurn := func() {
		o.RefreshContextSize(ctx)
		o.mu.Lock()
		usage := o.acc.Usage
		o.acc.Reset()
		o.mu.Unlock()
		o.eventCh <- common.ContentEvent{ID: o.id, Usage: usage}
	}

	res, err := executor.RunLoop(
		ctx,
		o.sess,
		o.maxTurns,
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
				Usage:            accCopy.Usage,
			}
		},
		o.middlewares,
		o.forceCompact,
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
	var extraBody map[string]any

	o.mu.Lock()
	if o.ctx.Err() != nil {
		o.ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(o.ctx)
	o.cancel = cancel
	o.ctx = ctx // Set the context so Execute/RunLoop can share the cancelable context safely
	o.mu.Unlock()

	defer cancel() // Ensure we don't leak the context when run() finishes

	// Inject orchestrator ID into context for tool interactions
	ctx = context.WithValue(ctx, common.OrchestratorIDKey, o.id)

	// Pre-run archive compaction hook (fail-open).
	o.runArchivePreHook()

	onStartTurn := func() {
		o.RefreshContextSize(ctx)
		o.mu.Lock()
		o.acc.Reset()
		o.mu.Unlock()
		o.eventCh <- common.StatusEvent{ID: o.id, Status: "thinking"}
	}

	onEndTurn := func() {
		o.RefreshContextSize(ctx)
		o.mu.Lock()
		usage := o.acc.Usage
		o.acc.Reset()
		o.mu.Unlock()
		o.eventCh <- common.ContentEvent{ID: o.id, Usage: usage}
	}

	_, err := executor.RunLoop(
		ctx,
		o.sess,
		o.maxTurns,
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
				Usage:            accCopy.Usage,
			}
		},
		o.middlewares,
		o.forceCompact,
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

	// Check if stop was requested and send StopRequestedEvent
	if o.IsStopRequested() {
		o.eventCh <- common.StopRequestedEvent{ID: o.id}
	}
}

func (o *BaseOrchestrator) Events() <-chan common.Event {
	return o.eventCh
}

func (o *BaseOrchestrator) Cancel() {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.cancel != nil {
		o.cancel()
	}

	select {
	case o.stopCh <- struct{}{}:
		// Signal sent
	default:
		// Already signaled, ignore
	}
}

func (o *BaseOrchestrator) IsStopRequested() bool {
	select {
	case <-o.stopCh:
		return true
	default:
		return false
	}
}

func (o *BaseOrchestrator) History() []client.ChatMessage {
	return o.sess.History
}

func (o *BaseOrchestrator) Session() *session.Session {
	return o.sess
}

func (o *BaseOrchestrator) SystemPrompt() string {
	return o.sess.SystemPrompt()
}

func (o *BaseOrchestrator) ToolDefinitions() []client.ToolDefinition {
	return o.sess.GetToolDefinitions()
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

// GetArchiveSubsystem returns the parent's archive subsystem so subagents can
// search the parent's session archive. Returns nil when compaction is disabled.
func (o *BaseOrchestrator) GetArchiveSubsystem() *tool.ArchiveSubsystem {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.archiveSub == nil {
		return nil
	}
	return o.archiveSub.sub
}

// GetArchiveSearchSettings returns maxResults and caseSensitive for archive search tools.
func (o *BaseOrchestrator) GetArchiveSearchSettings() (int, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.archiveSub == nil {
		return 10, false
	}
	return o.archiveSub.cfg.ArchiveSearchMaxResults, o.archiveSub.cfg.ArchiveSearchCaseSensitive
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

// forceCompact performs an emergency compaction when the context window overflows.
// It ignores the normal threshold — it always compacts regardless of history length.
// Returns true if compaction succeeded and the run loop should retry the turn.
func (o *BaseOrchestrator) forceCompact() bool {
	histPath := o.sess.HistoryPath
	if histPath == "" {
		return false
	}

	// Prefer the already-loaded archive settings; only re-read config from disk as fallback.
	var settings config.ArchiveCompactionConfig
	o.mu.RLock()
	existing := o.archiveSub
	o.mu.RUnlock()
	if existing != nil {
		settings = existing.cfg
	} else {
		cfg, err := config.LoadConfig()
		if err != nil || !cfg.IsArchiveCompactionEnabled() {
			return false
		}
		settings = cfg.ArchiveCompactionSettings()
	}

	var arch *archive.SessionArchive
	archPath := archive.ArchivePath(histPath)
	// Reuse the already-loaded archive when available to avoid unnecessary disk I/O.
	if existing != nil && existing.sub != nil && existing.sub.Archive != nil {
		arch = existing.sub.Archive
	} else if loaded, loadErr := archive.Load(archPath, o.id); loadErr == nil {
		arch = loaded
	} else {
		arch = archive.New(archive.BaseSessionID(histPath))
	}

	// Use a threshold of 0 to force compaction regardless of history length.
	compactCfg := archive.CompactionConfig{
		ThresholdMessages:  0,
		KeepRecentMessages: settings.KeepRecentMessages,
		ChunkSize:          settings.ArchiveChunkSize,
		StaleAfterSeconds:  settings.LockStaleAfterSeconds,
	}

	log.Printf("[archive] emergency compaction triggered by context overflow (history=%d)", len(o.sess.History))
	res, newActive, newArch, compactErr := archive.Compact(histPath, o.id, o.sess.History, arch, compactCfg)
	if compactErr != nil || res.NoOp {
		log.Printf("[archive] emergency compaction failed or no-op: %v", compactErr)
		return false
	}

	notice := fmt.Sprintf(
		"[System] Context window was full. %d messages were moved to the session archive. "+
			"Use search_session_archive to retrieve historical context.",
		res.ArchivedCount,
	)
	newActive = append(newActive, client.ChatMessage{Role: "user", Content: notice})

	o.mu.Lock()
	o.sess.History = newActive
	o.mu.Unlock()
	if err := session.SaveHistory(histPath, newActive); err != nil {
		log.Printf("[archive] emergency compaction: failed to save history: %v", err)
	}

	svc := archive.NewSearchService(newArch)
	svc.MarkDirty()

	o.mu.Lock()
	if o.archiveSub != nil && o.archiveSub.sub != nil {
		// Update the existing ArchiveSubsystem in-place so any already-registered
		// tools (search_session_archive, retrieve_archived_message) automatically
		// see the freshly compacted archive without needing to be re-registered.
		o.archiveSub.sub.Archive = newArch
		o.archiveSub.sub.Search = svc
	} else {
		o.archiveSub = &archiveState{
			sub: &tool.ArchiveSubsystem{Archive: newArch, Search: svc},
			cfg: settings,
		}
	}
	sub := o.archiveSub.sub
	o.mu.Unlock()

	// Update session meta counters so 'late session list -v' reflects the emergency compaction.
	metaID := archive.BaseSessionID(histPath)
	if meta, loadErr := session.LoadSessionMeta(metaID); loadErr == nil && meta != nil {
		meta.CompactionCount = newArch.CompactionCount
		meta.ArchivedMessageCount = newArch.ArchivedMessageCount
		meta.LastCompactionAt = time.Now().UTC()
		if saveErr := session.SaveSessionMeta(*meta); saveErr != nil {
			log.Printf("[archive] emergency compaction: failed to save session meta counters: %v", saveErr)
		}
	}

	reg := o.sess.Registry
	if reg != nil && reg.Get("search_session_archive") == nil {
		tool.RegisterArchiveTools(reg, sub, settings.ArchiveSearchMaxResults, settings.ArchiveSearchCaseSensitive)
	}

	log.Printf("[archive] emergency compaction complete: archived=%d msgs", res.ArchivedCount)
	return true
}

// runArchivePreHook runs archive compaction before a run loop if enabled.
// Fail-open: any error is logged but does not block execution.
func (o *BaseOrchestrator) runArchivePreHook() {
	histPath := o.sess.HistoryPath
	if histPath == "" {
		return
	}

	cfg, err := config.LoadConfig()
	if err != nil || !cfg.IsArchiveCompactionEnabled() {
		return
	}
	settings := cfg.ArchiveCompactionSettings()

	// Phase 8: verify archive file permissions (warn only).
	archPath := archive.ArchivePath(histPath)
	if info, statErr := os.Stat(archPath); statErr == nil {
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			log.Printf("[archive] warning: archive file %s has loose permissions (%o); expected 0600", archPath, perm)
		}
	}

	var arch *archive.SessionArchive
	o.mu.Lock()
	existing := o.archiveSub
	o.mu.Unlock()

	if existing != nil && existing.sub != nil && existing.sub.Archive != nil {
		arch = existing.sub.Archive
	} else {
		arch, err = archive.Load(archPath, o.id)
		if err != nil {
			log.Printf("[archive] failed to load archive for hook: %v", err)
			return
		}
		// Reconcile on first load: detect messages duplicated between archive and active
		// history, which can happen after a crash mid-compaction.
		reconciledHistory, warnings := archive.ReconcileOnStartup(arch, o.sess.History)
		for _, w := range warnings {
			log.Printf("[archive] reconcile: %s", w)
		}
		if len(warnings) > 0 {
			log.Printf("[archive] reconcile: found %d message(s) already archived; they will be deduplicated on next compaction", len(warnings))
			o.mu.Lock()
			o.sess.History = reconciledHistory
			o.mu.Unlock()
		}
	}

	compactCfg := archive.CompactionConfig{
		ThresholdMessages:  settings.CompactionThresholdMessages,
		KeepRecentMessages: settings.KeepRecentMessages,
		ChunkSize:          settings.ArchiveChunkSize,
		StaleAfterSeconds:  settings.LockStaleAfterSeconds,
	}

	log.Printf("[archive] pre-run hook: history=%d msgs, threshold=%d", len(o.sess.History), settings.CompactionThresholdMessages)
	compactStart := time.Now()

	res, newActive, newArch, err := archive.Compact(
		histPath, o.id,
		o.sess.History,
		arch,
		compactCfg,
	)
	compactDur := time.Since(compactStart)

	if err != nil {
		log.Printf("[archive] compaction hook error: %v", err)
		return
	}
	if res.LockHeld {
		log.Printf("[archive] compaction skipped (lock held by another process)")
	}
	if !res.NoOp {
		log.Printf("[archive] compaction complete: archived=%d msgs in %s", res.ArchivedCount, compactDur)

		// Inject a synthetic notice so the model is aware compaction occurred.
		notice := fmt.Sprintf(
			"[System] %d messages were moved to the session archive to free context space. "+
				"Use search_session_archive to search for historical context, "+
				"or retrieve_archived_message to fetch a specific message by reference.",
			res.ArchivedCount,
		)
		newActive = append(newActive, client.ChatMessage{
			Role:    "user",
			Content: notice,
		})

		o.mu.Lock()
		o.sess.History = newActive
		o.mu.Unlock()
		if err := session.SaveHistory(histPath, newActive); err != nil {
			log.Printf("[archive] failed to persist compacted history: %v", err)
		}

		// Phase 8: update session meta counters.
		metaID := archive.BaseSessionID(histPath)
		if meta, loadErr := session.LoadSessionMeta(metaID); loadErr == nil && meta != nil {
			meta.CompactionCount = newArch.CompactionCount
			meta.ArchivedMessageCount = newArch.ArchivedMessageCount
			meta.LastCompactionAt = time.Now().UTC()
			if saveErr := session.SaveSessionMeta(*meta); saveErr != nil {
				log.Printf("[archive] failed to save session meta counters: %v", saveErr)
			}
		}
	}

	svc := archive.NewSearchService(newArch)
	if !res.NoOp {
		svc.MarkDirty()
	}
	searchStart := time.Now()
	svc.WarmUp() // eagerly build the index so the first query is fast
	log.Printf("[archive] search index ready in %s", time.Since(searchStart))

	o.mu.Lock()
	firstInit := o.archiveSub == nil || o.archiveSub.sub == nil
	if !firstInit {
		// Already initialized — update in-place so registered tools (search_session_archive,
		// retrieve_archived_message) keep their *ArchiveSubsystem pointer. Replacing
		// o.archiveSub with a new struct would leave the tools searching a stale archive.
		o.archiveSub.sub.Archive = newArch
		if !res.NoOp {
			// Compaction produced a new archive — refresh the search index.
			o.archiveSub.sub.Search = svc
		}
		o.archiveSub.cfg = settings
	} else {
		o.archiveSub = &archiveState{
			sub: &tool.ArchiveSubsystem{
				Archive: newArch,
				Search:  svc,
			},
			cfg: settings,
		}
	}
	sub := o.archiveSub.sub
	o.mu.Unlock()

	// Register archive tools on first initialization only (subsequent calls update in-place).
	if firstInit && sub != nil {
		reg := o.sess.Registry
		if reg != nil {
			tool.RegisterArchiveTools(reg, sub,
				settings.ArchiveSearchMaxResults,
				settings.ArchiveSearchCaseSensitive)
			log.Printf("[archive] tools registered (search_session_archive, retrieve_archived_message)")
		}
	}
}
