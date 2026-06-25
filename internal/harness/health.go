package harness

import (
	"sort"
	"sync"
	"time"
)

// HealthStatus is the rolling verdict of a health probe. Severity increases
// left-to-right so that worstOf can pick the most severe status by max.
type HealthStatus int

const (
	// HealthUp means the probe is healthy and the component is serving.
	HealthUp HealthStatus = iota
	// HealthStarting means the component is not yet ready (e.g. warming up).
	HealthStarting
	// HealthDegraded means the component is serving but with reduced capacity.
	HealthDegraded
	// HealthDown means the component is not serving and needs attention.
	HealthDown
	// HealthStopped means the component has been intentionally stopped.
	HealthStopped
)

// String returns a human-readable status name.
func (s HealthStatus) String() string {
	switch s {
	case HealthUp:
		return "up"
	case HealthStarting:
		return "starting"
	case HealthDegraded:
		return "degraded"
	case HealthDown:
		return "down"
	case HealthStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// Health is the result of a single health probe. Detail is an opaque bag for
// backend-specific diagnostics (latency, queue depth, listener address, ...).
type Health struct {
	Status  HealthStatus
	Message string
	Detail  map[string]any
}

// Healthchecked is an optional interface a Component may implement to expose
// a probe. It is the computational feedback sensor of the harness.
type Healthchecked interface {
	Healthcheck(rt Runtime) Health
}

// Healthcheck is a standalone probe function. It is an alternative to
// implementing Healthchecked, useful for ad-hoc checks that are not owned by a
// component (e.g. a disk-space probe).
type Healthcheck func(rt Runtime) Health

// HealthReport is the aggregate verdict across every component and standalone
// probe. The overall Status is the most severe of the individual results,
// except that HealthStopped dominates when the harness is stopped.
type HealthReport struct {
	Status     HealthStatus
	CheckedAt  time.Time
	Components map[string]Health
	Checks     map[string]Health
}

// worstOf returns the more severe of two statuses. HealthStopped is treated as
// the most severe so a stopped harness reports stopped overall.
func worstOf(a, b HealthStatus) HealthStatus {
	if a == HealthStopped || b == HealthStopped {
		return HealthStopped
	}
	if a > b {
		return a
	}
	return b
}

// healthReporter aggregates component probes and standalone checks into a
// HealthReport. It is safe to call concurrently with probe registration.
type healthReporter struct {
	mu     sync.RWMutex
	checks map[string]Healthcheck
}

func newHealthReporter() *healthReporter {
	return &healthReporter{checks: map[string]Healthcheck{}}
}

// Register adds a named standalone probe. The name must be unique.
func (h *healthReporter) Register(name string, fn Healthcheck) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = fn
}

// Report runs every component probe (if c implements Healthchecked) and every
// standalone probe, returning the aggregate. Probes are run sequentially; a
// panicking probe is recovered and recorded as HealthDown so one bad probe
// cannot crash the reporter.
func (h *healthReporter) Report(rt Runtime, components []Component) HealthReport {
	report := HealthReport{
		CheckedAt:  time.Now(),
		Components: map[string]Health{},
		Checks:     map[string]Health{},
	}

	for _, c := range components {
		hc, ok := c.(Healthchecked)
		if !ok {
			continue
		}
		health := safeProbe(func() Health { return hc.Healthcheck(rt) })
		report.Components[c.Key()] = health
		report.Status = worstOf(report.Status, health.Status)
	}

	h.mu.RLock()
	names := make([]string, 0, len(h.checks))
	for n := range h.checks {
		names = append(names, n)
	}
	h.mu.RUnlock()
	sort.Strings(names) // deterministic order
	for _, n := range names {
		h.mu.RLock()
		fn := h.checks[n]
		h.mu.RUnlock()
		health := safeProbe(func() Health { return fn(rt) })
		report.Checks[n] = health
		report.Status = worstOf(report.Status, health.Status)
	}

	return report
}

// safeProbe runs fn and converts a panic into a HealthDown result so a single
// faulty probe never crashes the reporter.
func safeProbe(fn func() Health) (h Health) {
	defer func() {
		if r := recover(); r != nil {
			h = Health{Status: HealthDown, Message: "probe panicked"}
		}
	}()
	return fn()
}
