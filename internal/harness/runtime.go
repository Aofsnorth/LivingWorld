package harness

import (
	"context"

	"livingworld/internal/infrastructure/logging"
)

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
