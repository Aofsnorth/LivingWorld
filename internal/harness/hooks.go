package harness

import "sync"

// Phase identifies a point in the lifecycle where hooks may run. Hooks are
// the feedforward/feedback seam of the harness: an observer can reject a
// transition (by returning an error) or merely observe it (by returning nil).
type Phase int

// Hook phases, fired in order around each lifecycle transition.
const (
	PhaseBeforeInit  Phase = iota // before any component Init runs
	PhaseAfterInit                // after all component Inits have completed
	PhaseBeforeStart              // before any component Start runs
	PhaseAfterStart               // after all component Starts have completed
	PhaseBeforeStop               // before any component Stop runs
	PhaseAfterStop                // after all component Stops have completed
)

// String returns a human-readable phase name.
func (p Phase) String() string {
	switch p {
	case PhaseBeforeInit:
		return "before-init"
	case PhaseAfterInit:
		return "after-init"
	case PhaseBeforeStart:
		return "before-start"
	case PhaseAfterStart:
		return "after-start"
	case PhaseBeforeStop:
		return "before-stop"
	case PhaseAfterStop:
		return "after-stop"
	default:
		return "unknown"
	}
}

// Hook is a function invoked at a Phase. Returning a non-nil error aborts the
// surrounding lifecycle transition: for Before* phases the transition does not
// proceed and the harness enters StateFailed; for After* phases the error is
// recorded but the transition has already happened.
type Hook func(rt Runtime) error

// hookBus stores hooks keyed by phase. Registration is append-only and
// dispatch runs hooks in registration order. Safe for concurrent registration
// (registration happens before Start in practice) and dispatch.
type hookBus struct {
	mu    sync.RWMutex
	hooks map[Phase][]Hook
}

func newHookBus() *hookBus {
	return &hookBus{hooks: map[Phase][]Hook{}}
}

// On registers a hook for a phase.
func (b *hookBus) On(phase Phase, hook Hook) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hooks[phase] = append(b.hooks[phase], hook)
}

// dispatch runs all hooks for a phase in registration order, returning the
// first error. A nil slice is a no-op.
func (b *hookBus) dispatch(phase Phase, rt Runtime) error {
	b.mu.RLock()
	hooks := b.hooks[phase]
	b.mu.RUnlock()
	for _, h := range hooks {
		if err := h(rt); err != nil {
			return err
		}
	}
	return nil
}
