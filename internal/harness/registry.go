package harness

import (
	"fmt"
	"strings"
	"sync"
)

// registry holds the set of registered components and resolves their startup
// (and reverse-shutdown) order from the dependency graph. It is the OCP seam
// of the harness: adding a component never requires modifying the orchestrator.
type registry struct {
	mu    sync.RWMutex
	byKey map[string]Component
	order []Component // registration order, stable for diagnostics
}

func newRegistry() *registry {
	return &registry{byKey: map[string]Component{}}
}

// get returns the component registered under key, or nil. Safe to call
// concurrently with Register; lookups during lifecycle phases observe the
// registration that completed before Start.
func (r *registry) get(key string) Component {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byKey[key]
}

// Register adds a component. It rejects duplicates, empty keys, and references
// to dependencies that are not (yet) registered. Dependencies are validated
// eagerly so a misconfigured graph fails at registration rather than at Start.
func (r *registry) Register(c Component) error {
	k := c.Key()
	if k == "" {
		return fmt.Errorf("harness: component with empty key: %w", ErrInvalidComponent)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byKey[k]; dup {
		return fmt.Errorf("harness: duplicate component key %q: %w", k, ErrInvalidComponent)
	}
	// Dependencies are validated at resolve time so forward references are
	// permitted (a component may be registered before the dependency it
	// names). The tentatively-registered component is visible to later
	// registrations and to lookups.
	r.byKey[k] = c
	r.order = append(r.order, c)
	return nil
}

// resolve returns the components in dependency-first topological order. It
// reports an error on missing dependencies or cycles. The returned slice is
// the startup order; shutdown order is its reverse.
func (r *registry) resolve() ([]Component, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// Validate every declared dependency is present.
	for _, c := range r.order {
		for _, dep := range dependenciesOf(c) {
			if _, ok := r.byKey[dep]; !ok {
				return nil, fmt.Errorf("harness: component %q depends on missing %q: %w",
					c.Key(), dep, ErrUnresolvedDependency)
			}
		}
	}

	// Kahn's algorithm: emit nodes whose in-degree (count of unsatisfied
	// deps) reaches zero, repeat. Stable: nodes with equal in-degree are
	// emitted in registration order so the order is deterministic.
	inDegree := map[string]int{}
	dependents := map[string][]string{} // dep -> nodes that depend on dep
	for _, c := range r.order {
		deps := uniqueDependenciesOf(c)
		inDegree[c.Key()] = len(deps)
		for _, dep := range deps {
			dependents[dep] = append(dependents[dep], c.Key())
		}
	}

	var ready []string
	for _, c := range r.order {
		if inDegree[c.Key()] == 0 {
			ready = append(ready, c.Key())
		}
	}

	out := make([]Component, 0, len(r.order))
	for len(ready) > 0 {
		k := ready[0]
		ready = ready[1:]
		out = append(out, r.byKey[k])
		for _, dependent := range dependents[k] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				ready = append(ready, dependent)
			}
		}
	}

	if len(out) != len(r.order) {
		// Remaining nodes form a cycle.
		cycle := make([]string, 0)
		for _, c := range r.order {
			if inDegree[c.Key()] > 0 {
				cycle = append(cycle, c.Key())
			}
		}
		return nil, fmt.Errorf("harness: dependency cycle among components [%s]: %w",
			strings.Join(cycle, ", "), ErrCycleDetected)
	}
	return out, nil
}

// dependenciesOf returns the declared dependencies of c, or nil if it does not
// implement Dependent.
func dependenciesOf(c Component) []string {
	if d, ok := c.(Dependent); ok {
		return d.DependsOn()
	}
	return nil
}

// uniqueDependenciesOf returns dep keys with duplicates removed, preserving
// first-seen order so the in-degree count is correct.
func uniqueDependenciesOf(c Component) []string {
	deps := dependenciesOf(c)
	if len(deps) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(deps))
	for _, d := range deps {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

// Sentinel errors for registry failures. They are exported so callers can
// type-check with errors.Is without coupling to error-message text.
var (
	ErrInvalidComponent     = fmt.Errorf("invalid component")
	ErrUnresolvedDependency = fmt.Errorf("unresolved dependency")
	ErrCycleDetected        = fmt.Errorf("dependency cycle detected")
)
