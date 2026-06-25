package harness

import (
	"context"
	"time"

	"livingworld/internal/infrastructure/logging"
)

// Component is the unit of composition. A component owns a single
// responsibility (SRP) and progresses through three phases:
//
//   - Init:  cheap setup that does not start serving (bind callbacks, register
//     handlers, open config). Runs once, in dependency order.
//   - Start: begin serving / listening / ticking. Runs once, in dependency
//     order, after every component's Init has completed.
//   - Stop:  release resources and stop serving. Runs once, in reverse
//     dependency order.
//
// Implementations must be safe to construct but need not be safe to call
// Start/Stop concurrently — the harness serializes lifecycle phases. Stop
// should be idempotent so that rollback after a partial Start is safe.
type Component interface {
	// Key is the stable, unique identifier used by Dependents and by
	// Runtime.Component to look this component up. It must be non-empty and
	// unique within a Harness.
	Key() string

	// Init performs pre-serve setup. It must not block on external traffic.
	Init(rt Runtime) error

	// Start begins serving. It should return once the component is ready to
	// accept work, not when it finishes serving.
	Start(rt Runtime) error

	// Stop releases resources. It must be safe to call even if Start never
	// ran or only partially completed.
	Stop(rt Runtime) error
}

// Dependent is an optional interface a Component may implement to declare the
// keys of components that must be initialized and started before it. The
// registry uses this to topologically order startup (and reverse the order for
// shutdown). A dependency that is not registered is reported as an error at
// registration time rather than silently ignored.
type Dependent interface {
	DependsOn() []string
}

// Runtime is the per-phase context handed to every Component method. It
// extends context.Context with the cross-cutting concerns a component is
// allowed to reach for:
//
//   - Logger:  structured logging (DIP — never use the global logger).
//   - Metrics: the observability recorder (DIP — default is a no-op).
//   - Component: sibling lookup for dependency injection. The returned value
//     is the registered Component instance; callers type-assert to the
//     concrete type they depend on. Lookups for unknown keys return nil.
//
// Keeping these behind the Runtime interface means a component never imports
// the harness internals and can be tested with a fake Runtime (ISP).
type Runtime interface {
	context.Context

	// Logger returns the harness logger, pre-tagged for the calling phase.
	Logger() logging.Logger

	// Metrics returns the metrics recorder. The default is a no-op recorder
	// so components can instrument freely without forcing a backend.
	Metrics() MetricsRecorder

	// Component returns the registered component instance for key, or nil if
	// no component with that key is registered. Ordering guarantees the
	// returned component has already completed its Init (and, during Start,
	// its Start) when key is declared in DependsOn.
	Component(key string) any

	// State returns the current lifecycle state.
	State() State
}

// MetricsRecorder is the observability seam for computational feedback
// sensors. Implementations may forward to Prometheus, OpenTelemetry, or any
// other backend. The default NoopRecorder discards everything, so components
// can call it unconditionally without coupling the harness to a backend (DIP).
type MetricsRecorder interface {
	Counter(name string, value int64, tags ...string)
	Gauge(name string, value float64, tags ...string)
	Duration(name string, d time.Duration, tags ...string)
}

// NoopRecorder is a MetricsRecorder that discards all calls. It is the zero
// value used when no backend is configured.
type NoopRecorder struct{}

// Counter discards the sample.
func (NoopRecorder) Counter(string, int64, ...string) {}

// Gauge discards the sample.
func (NoopRecorder) Gauge(string, float64, ...string) {}

// Duration discards the sample.
func (NoopRecorder) Duration(string, time.Duration, ...string) {}
