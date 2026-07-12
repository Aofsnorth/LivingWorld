package harness

import (
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
