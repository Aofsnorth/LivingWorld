package harness

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// SignalSource is the seam through which the harness observes shutdown
// signals. The default implementation listens for SIGINT and SIGTERM;
// tests inject a fake source so shutdown can be driven deterministically
// without sending real signals to the process (DIP + testability).
type SignalSource interface {
	// Wait blocks until a shutdown signal is observed or ctx is cancelled.
	// It must be safe to abandon.
	Wait(ctx context.Context) error
}

// osSignalSource is the default SignalSource. It listens for the configured
// signals and stops listening once Wait returns.
type osSignalSource struct {
	signals []os.Signal
}

// NewSignalSource returns a SignalSource that listens for the given signals.
// If no signals are passed, SIGINT and SIGTERM are used.
func NewSignalSource(signals ...os.Signal) SignalSource {
	if len(signals) == 0 {
		signals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	}
	return &osSignalSource{signals: signals}
}

// Wait blocks until one of the configured signals is delivered or ctx is
// cancelled. The signal channel is created per call so multiple Waiters do not
// steal each other's signals.
func (s *osSignalSource) Wait(ctx context.Context) error {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, s.signals...)
	defer signal.Stop(ch)
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// noopSignalSource never reports a signal on its own; it is used when an
// external loop (e.g. the TUI) owns signal handling and drives shutdown via
// Harness.Stop instead.
type noopSignalSource struct{}

func (noopSignalSource) Wait(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}
