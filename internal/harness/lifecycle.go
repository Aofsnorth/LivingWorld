package harness

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// State is the lifecycle phase the harness is currently in. Transitions are
// strictly linear and enforced by the state machine; any out-of-order
// transition returns ErrInvalidTransition.
type State int

// Lifecycle states, in progression order.
const (
	StateCreated      State = iota // components registered, no phase has run
	StateInitializing              // Init phase in progress
	StateInitialized               // Init complete, Start not yet begun
	StateStarting                  // Start phase in progress
	StateRunning                   // all components started, serving traffic
	StateStopping                  // Stop phase in progress
	StateStopped                   // fully shut down
	StateFailed                    // a lifecycle phase failed; see Harness.Err
)

// String returns a human-readable state name.
func (s State) String() string {
	switch s {
	case StateCreated:
		return "created"
	case StateInitializing:
		return "initializing"
	case StateInitialized:
		return "initialized"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	case StateFailed:
		return "failed"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// IsTerminal reports whether s is a terminal state (no further transitions
// are valid except a reset).
func (s State) IsTerminal() bool { return s == StateStopped || s == StateFailed }

// lifecycle is a thread-safe state machine. It guards the legal transitions
// and records the failure cause when entering StateFailed.
type lifecycle struct {
	mu    sync.RWMutex
	st    State
	cause error
}

func newLifecycle() *lifecycle { return &lifecycle{st: StateCreated} }

// State returns the current state.
func (l *lifecycle) State() State {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.st
}

// Err returns the failure cause when State == StateFailed, otherwise nil.
func (l *lifecycle) Err() error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.cause
}

// allowedTransitions defines the legal successor states for each state. The
// forced shutdown path (beginStop) bypasses this table.
var allowedTransitions = map[State]map[State]bool{
	StateCreated:      {StateInitializing: true, StateFailed: true},
	StateInitializing: {StateInitialized: true, StateFailed: true},
	StateInitialized:  {StateStarting: true, StateFailed: true},
	StateStarting:     {StateRunning: true, StateFailed: true},
	StateRunning:      {StateStopping: true},
	StateStopping:     {StateStopped: true, StateFailed: true},
	StateStopped:      {},
	StateFailed:       {},
}

// transition moves from -> to, returning ErrInvalidTransition if the move is
// not permitted from the current state or if `to` is not a legal successor of
// `from`. A failing transition (to == StateFailed) records cause for later
// retrieval.
func (l *lifecycle) transition(from, to State, cause error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.st != from {
		return fmt.Errorf("harness: invalid transition %s -> %s (currently %s): %w",
			from, to, l.st, ErrInvalidTransition)
	}
	if !allowedTransitions[from][to] {
		return fmt.Errorf("harness: illegal transition %s -> %s: %w",
			from, to, ErrInvalidTransition)
	}
	l.st = to
	if to == StateFailed {
		l.cause = cause
	}
	return nil
}

// beginStop forces the state to StateStopping from any non-terminal state. It
// is used by the shutdown path which must be able to tear down components
// whether the harness is Running, mid-Start, or already Failed. Calling it
// when already Stopping or Stopped is a no-op.
func (l *lifecycle) beginStop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.st == StateStopping || l.st == StateStopped {
		return
	}
	l.st = StateStopping
}

// endStop finalizes shutdown. If cause is non-nil the state becomes StateFailed
// and the cause is recorded; otherwise StateStopped. A previously-recorded
// failure cause (set via transition to StateFailed) is preserved.
func (l *lifecycle) endStop(cause error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.st == StateFailed && l.cause != nil {
		// Preserve the original failure cause; shutdown errors are secondary.
		if cause != nil {
			l.cause = fmt.Errorf("%w (shutdown also failed: %v)", l.cause, cause)
		}
		return
	}
	if cause != nil {
		l.st = StateFailed
		l.cause = cause
	} else {
		l.st = StateStopped
	}
}

// ErrInvalidTransition is returned when a lifecycle transition is requested
// from a state that does not permit it.
var ErrInvalidTransition = fmt.Errorf("invalid lifecycle transition")

// Start runs the Init and Start phases and returns once the harness is Running.
// On failure it rolls back by stopping every component that was initialized,
// records the cause via Err, and returns it. Calling Start twice is an error.
func (h *Harness) Start(ctx context.Context) error {
	if err := h.lifecycle.transition(StateCreated, StateInitializing, nil); err != nil {
		return err
	}
	comps, err := h.registry.resolve()
	if err != nil {
		return h.abortStart(ctx, err)
	}
	h.components = comps
	rt := h.newRuntime(ctx)

	// --- Init phase ---
	if err := h.hooks.dispatch(PhaseBeforeInit, rt); err != nil {
		return h.abortStart(ctx, fmt.Errorf("before-init hook: %w", err))
	}
	for _, c := range comps {
		if err := h.initComponent(rt, c); err != nil {
			return h.abortStart(ctx, err)
		}
	}
	if err := h.hooks.dispatch(PhaseAfterInit, rt); err != nil {
		return h.abortStart(ctx, fmt.Errorf("after-init hook: %w", err))
	}
	if err := h.lifecycle.transition(StateInitializing, StateInitialized, nil); err != nil {
		return h.abortStart(ctx, err)
	}

	// --- Start phase ---
	if err := h.lifecycle.transition(StateInitialized, StateStarting, nil); err != nil {
		return h.abortStart(ctx, err)
	}
	if err := h.hooks.dispatch(PhaseBeforeStart, rt); err != nil {
		return h.abortStart(ctx, fmt.Errorf("before-start hook: %w", err))
	}
	for _, c := range comps {
		if err := h.startComponent(rt, c); err != nil {
			return h.abortStart(ctx, err)
		}
	}
	if err := h.hooks.dispatch(PhaseAfterStart, rt); err != nil {
		return h.abortStart(ctx, fmt.Errorf("after-start hook: %w", err))
	}
	if err := h.lifecycle.transition(StateStarting, StateRunning, nil); err != nil {
		return h.abortStart(ctx, err)
	}
	h.metrics.Counter("harness.start.count", 1, "result", "ok")
	h.logger.Info("harness: running with %d component(s)", len(comps))
	return nil
}

// abortStart records the failure cause, triggers a full rollback shutdown of
// every resolved component, waits for it to finish, and returns the cause.
func (h *Harness) abortStart(ctx context.Context, cause error) error {
	h.failMu.Lock()
	h.failCause = cause
	h.failMu.Unlock()
	h.lifecycle.transition(h.lifecycle.State(), StateFailed, cause)
	h.metrics.Counter("harness.start.count", 1, "result", "failed")
	h.triggerShutdown()
	<-h.doneCh
	return cause
}

// Run starts the harness and blocks until a shutdown signal is received or
// Stop is called, then performs a graceful shutdown and returns. The returned
// error is the Start error (if any) or the shutdown error.
func (h *Harness) Run(ctx context.Context) error {
	if err := h.Start(ctx); err != nil {
		return err
	}
	sigCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Drive shutdown from the signal source. When it fires (or the parent
	// context is cancelled) we trigger shutdown; the dedicated shutdownLoop
	// performs the actual teardown exactly once.
	go func() {
		_ = h.signals.Wait(sigCtx)
		h.triggerShutdown()
	}()
	<-h.doneCh
	return h.shutdownErr
}

// Stop requests a graceful shutdown and blocks until it completes. It is
// idempotent and safe to call concurrently from multiple goroutines.
func (h *Harness) Stop() error {
	h.triggerShutdown()
	<-h.doneCh
	return h.shutdownErr
}

// triggerShutdown closes stopCh exactly once, arming the shutdownLoop.
func (h *Harness) triggerShutdown() {
	h.stoppedOnce.Do(func() { close(h.stopCh) })
}

// shutdownLoop runs in its own goroutine from construction. It waits for a
// shutdown request and then tears down every resolved component in reverse
// dependency order. Runs at most once.
func (h *Harness) shutdownLoop() {
	<-h.stopCh
	h.shutdownErr = h.shutdown()
	close(h.doneCh)
}

// shutdown performs the teardown: BeforeStop hooks, component Stops in reverse
// order, AfterStop hooks. Each Stop is isolated so one failure does not skip
// the rest. The shutdown context is bounded by the shutdown timeout.
func (h *Harness) shutdown() error {
	h.lifecycle.beginStop()
	ctx, cancel := context.WithTimeout(context.Background(), h.shutdownTimeout)
	defer cancel()
	rt := h.newRuntime(ctx)

	if err := h.hooks.dispatch(PhaseBeforeStop, rt); err != nil {
		h.logger.Warn("harness: before-stop hook failed: %v", err)
	}

	var errs []error
	for i := len(h.components) - 1; i >= 0; i-- {
		c := h.components[i]
		if err := h.stopComponent(rt, c); err != nil {
			errs = append(errs, err)
		}
	}

	if err := h.hooks.dispatch(PhaseAfterStop, rt); err != nil {
		h.logger.Warn("harness: after-stop hook failed: %v", err)
	}

	cause := errors.Join(errs...)
	// If Start failed, surface the original cause as primary.
	h.failMu.Lock()
	fc := h.failCause
	h.failMu.Unlock()
	if fc != nil {
		cause = fc
	}
	h.lifecycle.endStop(cause)
	h.metrics.Counter("harness.shutdown.count", 1, "result", resultTag(cause))
	h.logger.Info("harness: stopped")
	return cause
}

// initComponent runs a single component's Init with metrics and panic
// isolation. A panicking Init is converted to an error so one bad component
// cannot crash the harness.
func (h *Harness) initComponent(rt Runtime, c Component) error {
	start := time.Now()
	h.logger.Debug("harness: init %s", c.Key())
	err := safeCall(func() error { return c.Init(rt) })
	h.metrics.Duration("harness.component.init", time.Since(start),
		"component", c.Key(), "result", resultTag(err))
	if err != nil {
		return fmt.Errorf("init %s: %w", c.Key(), err)
	}
	return nil
}

func (h *Harness) startComponent(rt Runtime, c Component) error {
	start := time.Now()
	h.logger.Info("harness: start %s", c.Key())
	err := safeCall(func() error { return c.Start(rt) })
	h.metrics.Duration("harness.component.start", time.Since(start),
		"component", c.Key(), "result", resultTag(err))
	if err != nil {
		return fmt.Errorf("start %s: %w", c.Key(), err)
	}
	return nil
}

func (h *Harness) stopComponent(rt Runtime, c Component) error {
	start := time.Now()
	h.logger.Debug("harness: stop %s", c.Key())
	err := safeCall(func() error { return c.Stop(rt) })
	h.metrics.Duration("harness.component.stop", time.Since(start),
		"component", c.Key(), "result", resultTag(err))
	if err != nil {
		return fmt.Errorf("stop %s: %w", c.Key(), err)
	}
	return nil
}
