package harness

import (
	"context"
	"sync"
	"time"

	"livingworld/internal/infrastructure/logging"
)

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

// Health runs every component probe and standalone check and returns the
// aggregate report. Safe to call while Running.
func (h *Harness) Health() HealthReport {
	rt := h.newRuntime(context.Background())
	return h.health.Report(rt, h.components)
}
