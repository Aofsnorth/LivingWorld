package harness

import (
	"errors"
	"testing"
)

func TestRegistryResolvesDependencyOrder(t *testing.T) {
	r := newRegistry()
	a := &fakeComponent{key: "a", deps: []string{"b", "c"}}
	b := &fakeComponent{key: "b"}
	c := &fakeComponent{key: "c", deps: []string{"b"}}
	for _, comp := range []Component{a, b, c} {
		if err := r.Register(comp); err != nil {
			t.Fatalf("register %s: %v", comp.Key(), err)
		}
	}
	order, err := r.resolve()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	keys := keysOf(order)
	// b before c, b and c before a.
	if !before(keys, "b", "c") || !before(keys, "b", "a") || !before(keys, "c", "a") {
		t.Fatalf("unexpected order: %v", keys)
	}
}

func TestRegistryDetectsCycle(t *testing.T) {
	r := newRegistry()
	_ = r.Register(&fakeComponent{key: "a", deps: []string{"b"}})
	_ = r.Register(&fakeComponent{key: "b", deps: []string{"a"}})
	if _, err := r.resolve(); !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

func TestRegistryMissingDependency(t *testing.T) {
	r := newRegistry()
	if err := r.Register(&fakeComponent{key: "a", deps: []string{"ghost"}}); err != nil {
		t.Fatalf("register should allow forward ref: %v", err)
	}
	if _, err := r.resolve(); !errors.Is(err, ErrUnresolvedDependency) {
		t.Fatalf("expected ErrUnresolvedDependency, got %v", err)
	}
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	r := newRegistry()
	if err := r.Register(&fakeComponent{key: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&fakeComponent{key: "a"}); !errors.Is(err, ErrInvalidComponent) {
		t.Fatalf("expected ErrInvalidComponent, got %v", err)
	}
}
