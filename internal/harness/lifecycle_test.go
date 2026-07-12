package harness

import (
	"errors"
	"testing"
)

func TestLifecycleValidTransitions(t *testing.T) {
	l := newLifecycle()
	for _, tc := range []struct{ from, to State }{
		{StateCreated, StateInitializing},
		{StateInitializing, StateInitialized},
		{StateInitialized, StateStarting},
		{StateStarting, StateRunning},
		{StateRunning, StateStopping},
		{StateStopping, StateStopped},
	} {
		if err := l.transition(tc.from, tc.to, nil); err != nil {
			t.Fatalf("transition %s->%s: %v", tc.from, tc.to, err)
		}
	}
	if l.State() != StateStopped {
		t.Fatalf("expected stopped, got %s", l.State())
	}
}

func TestLifecycleInvalidTransition(t *testing.T) {
	l := newLifecycle()
	if err := l.transition(StateCreated, StateRunning, nil); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestLifecycleFailedRecordsCause(t *testing.T) {
	l := newLifecycle()
	cause := errors.New("boom")
	if err := l.transition(StateCreated, StateFailed, cause); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(l.Err(), cause) {
		t.Fatalf("expected cause recorded, got %v", l.Err())
	}
	if !l.State().IsTerminal() {
		t.Fatalf("expected terminal state, got %s", l.State())
	}
}
