package harness

import (
	"fmt"
	"sync"
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
