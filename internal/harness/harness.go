package harness

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"livingworld/internal/infrastructure/logging"
)

// Option configures a Harness at construction. Options are applied in order;
// later options win. The zero-value Harness is usable and defaults to a
// no-op metrics recorder, the standard logger, SIGINT/SIGTERM signal
// handling, and a 30s shutdown timeout.
type Option func(*Harness)

// WithLogger sets the harness logger. It is propagated to every Runtime so
// components never reach for a global logger (DIP).
func WithLogger(l logging.Logger) Option { return func(h *Harness) { h.logger = l } }

// WithMetrics sets the metrics recorder. Pass a NoopRecorder (the default) or
// a backend-specific implementation.
func WithMetrics(m MetricsRecorder) Option {
	if m == nil {
		m = NoopRecorder{}
	}
	return func(h *Harness) { h.metrics = m }
}

// WithSignalSource sets the signal source. Use a fake in tests; pass
// NewSignalSource() (the default) for production. Pass a noopSignalSource
// (via WithNoopSignals) when an external loop owns shutdown.
func WithSignalSource(s SignalSource) Option {
	if s == nil {
		s = NewSignalSource()
	}
	return func(h *Harness) { h.signals = s }
}

// WithNoopSignals disables signal-driven shutdown. The harness will only stop
// when Stop is called explicitly. Use this when an outer event loop (e.g. the
// TUI) owns signal handling and drives shutdown through Stop.
func WithNoopSignals() Option {
	return func(h *Harness) { h.signals = noopSignalSource{} }
}

// WithShutdownTimeout bounds the graceful shutdown phase. Components that do
// not finish Stop within this window are still returned from; their Stop
// receives a cancelled context. Must be positive.
func WithShutdownTimeout(d time.Duration) Option {
	return func(h *Harness) {
		if d > 0 {
			h.shutdownTimeout = d
		}
	}
}

// On registers a hook for a phase. It is a convenience over the equivalent
// option and may be called after construction (but before Start) to attach
// feedforward/feedback observers.
func On(phase Phase, hook Hook) Option {
	return func(h *Harness) { h.hooks.On(phase, hook) }
}

// Harness orchestrates the component lifecycle. It is constructed with New,
// populated with Register, and driven with Start/Run/Stop. A Harness is safe
// to construct but a single instance is not designed to be Start-ed more than
// once; build a new Harness for a fresh run.
type Harness struct {
	registry  *registry
	lifecycle *lifecycle
	hooks     *hookBus
	health    *healthReporter

	logger          logging.Logger
	metrics         MetricsRecorder
	signals         SignalSource
	shutdownTimeout time.Duration

	// components is the resolved dependency-ordered slice, set during Start
	// and read by shutdown. Guarded by startOnce / the lifecycle.
	components []Component

	// failCause records a Start-phase failure so Err can report the original
	// cause even though shutdown may also produce secondary errors.
	failMu      sync.Mutex
	failCause   error
	stoppedOnce sync.Once
	stopCh      chan struct{}
	doneCh      chan struct{}
	shutdownErr error
}

// New returns a configured Harness. Apply options to override the logger,
// metrics, signal source, shutdown timeout, or to attach hooks.
func New(opts ...Option) *Harness {
	h := &Harness{
		registry:        newRegistry(),
		lifecycle:       newLifecycle(),
		hooks:           newHookBus(),
		health:          newHealthReporter(),
		logger:          logging.GetLogger("Harness"),
		metrics:         NoopRecorder{},
		signals:         NewSignalSource(),
		shutdownTimeout: 30 * time.Second,
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
	}
	for _, o := range opts {
		if o != nil {
			o(h)
		}
	}
	go h.shutdownLoop()
	return h
}

// Register adds a component. Must be called before Start. Returns an error on
// duplicate or empty keys; unresolved forward references are reported at Start.
func (h *Harness) Register(c Component) error { return h.registry.Register(c) }

// RegisterHealthcheck adds a named standalone probe. Must be called before
// Start so the reporter observes it.
func (h *Harness) RegisterHealthcheck(name string, fn Healthcheck) {
	h.health.Register(name, fn)
}

// OnHook attaches a hook to a phase after construction. Must be called before
// Start.
func (h *Harness) OnHook(phase Phase, hook Hook) { h.hooks.On(phase, hook) }

// State returns the current lifecycle state.
func (h *Harness) State() State { return h.lifecycle.State() }

// Err returns the failure cause when State == StateFailed, otherwise nil.
func (h *Harness) Err() error {
	h.failMu.Lock()
	fc := h.failCause
	h.failMu.Unlock()
	if fc != nil {
		return fc
	}
	return h.lifecycle.Err()
}

// Components returns the keys of registered components in registration order.
func (h *Harness) Components() []string {
	comps := h.registry.order
	out := make([]string, 0, len(comps))
	for _, c := range comps {
		out = append(out, c.Key())
	}
	return out
}

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

// Health runs every component probe and standalone check and returns the
// aggregate report. Safe to call while Running.
func (h *Harness) Health() HealthReport {
	rt := h.newRuntime(context.Background())
	return h.health.Report(rt, h.components)
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

// newRuntime builds the Runtime handed to components for a phase. The
// component lookup and state accessor close over the harness so components see
// live state and sibling instances.
func (h *Harness) newRuntime(ctx context.Context) Runtime {
	if ctx == nil {
		ctx = context.Background()
	}
	return runtimeImpl{
		Context: ctx,
		logger:  h.logger,
		metrics: h.metrics,
		lookup:  func(key string) any { return h.registry.get(key) },
		state:   h.lifecycle.State,
	}
}

// runtimeImpl is the concrete Runtime. It embeds context.Context so it
// satisfies the cancellation/deadline contract in addition to the harness
// accessors.
type runtimeImpl struct {
	context.Context
	logger  logging.Logger
	metrics MetricsRecorder
	lookup  func(string) any
	state   func() State
}

func (r runtimeImpl) Logger() logging.Logger   { return r.logger }
func (r runtimeImpl) Metrics() MetricsRecorder { return r.metrics }
func (r runtimeImpl) Component(key string) any { return r.lookup(key) }
func (r runtimeImpl) State() State             { return r.state() }

// safeCall runs fn and converts a panic into an error so a faulty component
// cannot crash the harness.
func safeCall(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return fn()
}

// resultTag turns an error into a metrics tag value.
func resultTag(err error) string {
	if err == nil {
		return "ok"
	}
	return "error"
}
