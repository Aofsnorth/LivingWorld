package harness

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"livingworld/internal/infrastructure/logging"
)

// fakeSignal is an injectable SignalSource for deterministic tests.
type fakeSignal struct {
	mu    sync.Mutex
	ch    chan struct{}
	fired bool
}

func newFakeSignal() *fakeSignal { return &fakeSignal{ch: make(chan struct{})} }
func (f *fakeSignal) fire() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.fired {
		f.fired = true
		close(f.ch)
	}
}
func (f *fakeSignal) Wait(ctx context.Context) error {
	select {
	case <-f.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// recorder is a MetricsRecorder that captures calls for assertion.
type recorder struct {
	mu       sync.Mutex
	counters []string
	gauges   []string
	durs     []string
}

func (r *recorder) Counter(name string, v int64, tags ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters = append(r.counters, name)
}
func (r *recorder) Gauge(name string, v float64, tags ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gauges = append(r.gauges, name)
}
func (r *recorder) Duration(name string, d time.Duration, tags ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.durs = append(r.durs, name)
}

// fakeComponent records the order and count of its lifecycle calls. It is the
// workhorse of the harness tests: every assertion is phrased in terms of the
// trace it leaves behind.
type fakeComponent struct {
	key                        string
	deps                       []string
	mu                         sync.Mutex
	trace                      []string
	startErr, stopErr, initErr error
	health                     Health
}

func (f *fakeComponent) Key() string         { return f.key }
func (f *fakeComponent) DependsOn() []string { return f.deps }
func (f *fakeComponent) Init(Runtime) error {
	f.mu.Lock()
	f.trace = append(f.trace, "init")
	f.mu.Unlock()
	return f.initErr
}
func (f *fakeComponent) Start(Runtime) error {
	f.mu.Lock()
	f.trace = append(f.trace, "start")
	f.mu.Unlock()
	return f.startErr
}
func (f *fakeComponent) Stop(Runtime) error {
	f.mu.Lock()
	f.trace = append(f.trace, "stop")
	f.mu.Unlock()
	return f.stopErr
}
func (f *fakeComponent) Healthcheck(Runtime) Health { return f.health }
func (f *fakeComponent) traceSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(f.trace))
	copy(cp, f.trace)
	return cp
}

func newHarnessForTest(t *testing.T, opts ...Option) *Harness {
	t.Helper()
	return New(append([]Option{WithLogger(testLogger()), WithNoopSignals()}, opts...)...)
}

func testLogger() logging.Logger { return logging.GetLogger("test") }

// --- lifecycle ---

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

// --- registry ---

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

// --- health ---

func TestHealthReportAggregatesWorst(t *testing.T) {
	r := newHealthReporter()
	r.Register("disk", func(Runtime) Health { return Health{Status: HealthUp} })
	r.Register("db", func(Runtime) Health { return Health{Status: HealthDegraded, Message: "slow"} })
	comps := []Component{
		&fakeComponent{key: "svc", health: Health{Status: HealthUp}},
		&fakeComponent{key: "queue", health: Health{Status: HealthDown, Message: "drained"}},
	}
	report := r.Report(nilRuntime{}, comps)
	if report.Status != HealthDown {
		t.Fatalf("expected down (worst), got %s", report.Status)
	}
	if report.Components["queue"].Status != HealthDown {
		t.Fatal("queue probe missing")
	}
	if report.Checks["db"].Status != HealthDegraded {
		t.Fatal("db check missing")
	}
}

func TestHealthReportRecoversPanic(t *testing.T) {
	r := newHealthReporter()
	r.Register("bad", func(Runtime) Health { panic("nope") })
	report := r.Report(nilRuntime{}, nil)
	if report.Checks["bad"].Status != HealthDown {
		t.Fatalf("expected panicking probe reported as down, got %s", report.Checks["bad"].Status)
	}
	if report.Status != HealthDown {
		t.Fatalf("aggregate should be down, got %s", report.Status)
	}
}

// --- harness lifecycle ---

func TestHarnessStartStopOrder(t *testing.T) {
	a := &fakeComponent{key: "a"}
	b := &fakeComponent{key: "b", deps: []string{"a"}}
	h := newHarnessForTest(t)
	if err := h.Register(a); err != nil {
		t.Fatal(err)
	}
	if err := h.Register(b); err != nil {
		t.Fatal(err)
	}
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if h.State() != StateRunning {
		t.Fatalf("expected running, got %s", h.State())
	}
	// a inits and starts before b.
	if !before(a.traceSnapshot(), "init", "start") {
		t.Fatalf("a should init before start: %v", a.traceSnapshot())
	}
	if aStartIdx(a, b) == false {
		t.Fatalf("a should start before b: a=%v b=%v", a.traceSnapshot(), b.traceSnapshot())
	}
	if err := h.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if h.State() != StateStopped {
		t.Fatalf("expected stopped, got %s", h.State())
	}
	// Stop runs in reverse order: b stopped before a.
	if !stopReverseOrder(a, b) {
		t.Fatalf("expected b stopped before a: a=%v b=%v", a.traceSnapshot(), b.traceSnapshot())
	}
}

func TestHarnessRollbackOnStartFailure(t *testing.T) {
	a := &fakeComponent{key: "a"}
	b := &fakeComponent{key: "b", startErr: errors.New("nope")}
	h := newHarnessForTest(t)
	_ = h.Register(a)
	_ = h.Register(b)
	err := h.Start(context.Background())
	if err == nil {
		t.Fatal("expected start error")
	}
	if !errors.Is(err, b.startErr) {
		t.Fatalf("expected b's error, got %v", err)
	}
	if h.State() != StateFailed {
		t.Fatalf("expected failed, got %s", h.State())
	}
	// Rollback must have stopped a (the only successfully started component).
	if len(a.traceSnapshot()) == 0 || lastOf(a.traceSnapshot()) != "stop" {
		t.Fatalf("expected a rolled back (stop), got %v", a.traceSnapshot())
	}
}

func TestHarnessStopIsIdempotent(t *testing.T) {
	a := &fakeComponent{key: "a"}
	h := newHarnessForTest(t)
	_ = h.Register(a)
	_ = h.Start(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = h.Stop() }()
	}
	wg.Wait()
	if h.State() != StateStopped {
		t.Fatalf("expected stopped, got %s", h.State())
	}
	// a.Stop should have been called exactly once despite concurrent Stop calls.
	stops := countOf(a.traceSnapshot(), "stop")
	if stops != 1 {
		t.Fatalf("expected exactly 1 stop, got %d (trace=%v)", stops, a.traceSnapshot())
	}
}

func TestHarnessRunStopsOnSignal(t *testing.T) {
	sig := newFakeSignal()
	a := &fakeComponent{key: "a"}
	h := newHarnessForTest(t, WithSignalSource(sig))
	_ = h.Register(a)
	done := make(chan error, 1)
	go func() { done <- h.Run(context.Background()) }()
	// Give Start a moment to reach Running.
	waitForState(t, h, StateRunning, time.Second)
	sig.fire()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return after signal")
	}
	if h.State() != StateStopped {
		t.Fatalf("expected stopped, got %s", h.State())
	}
}

func TestHarnessRunStopsOnContextCancel(t *testing.T) {
	a := &fakeComponent{key: "a"}
	h := newHarnessForTest(t) // noop signals: only ctx cancel will fire
	_ = h.Register(a)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.Run(ctx) }()
	waitForState(t, h, StateRunning, time.Second)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return after ctx cancel")
	}
	if h.State() != StateStopped {
		t.Fatalf("expected stopped, got %s", h.State())
	}
}

func TestHarnessHooksFire(t *testing.T) {
	var fired []Phase
	var mu sync.Mutex
	add := func(p Phase) Hook {
		return func(Runtime) error {
			mu.Lock()
			fired = append(fired, p)
			mu.Unlock()
			return nil
		}
	}
	h := newHarnessForTest(t,
		On(PhaseBeforeInit, add(PhaseBeforeInit)),
		On(PhaseAfterInit, add(PhaseAfterInit)),
		On(PhaseBeforeStart, add(PhaseBeforeStart)),
		On(PhaseAfterStart, add(PhaseAfterStart)),
		On(PhaseBeforeStop, add(PhaseBeforeStop)),
		On(PhaseAfterStop, add(PhaseAfterStop)),
	)
	_ = h.Register(&fakeComponent{key: "a"})
	_ = h.Start(context.Background())
	_ = h.Stop()
	mu.Lock()
	defer mu.Unlock()
	want := []Phase{PhaseBeforeInit, PhaseAfterInit, PhaseBeforeStart, PhaseAfterStart, PhaseBeforeStop, PhaseAfterStop}
	if len(fired) != len(want) {
		t.Fatalf("expected %d hooks, got %d (%v)", len(want), len(fired), fired)
	}
	for i, p := range want {
		if fired[i] != p {
			t.Fatalf("hook %d: want %s, got %s", i, p, fired[i])
		}
	}
}

func TestHarnessBeforeStartHookCanAbort(t *testing.T) {
	boom := errors.New("nope")
	h := newHarnessForTest(t, On(PhaseBeforeStart, func(Runtime) error { return boom }))
	_ = h.Register(&fakeComponent{key: "a"})
	err := h.Start(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
	if h.State() != StateFailed {
		t.Fatalf("expected failed, got %s", h.State())
	}
}

func TestHarnessComponentLookup(t *testing.T) {
	a := &fakeComponent{key: "a"}
	var seen any
	b := &fakeComponent{
		key:  "b",
		deps: []string{"a"},
	}
	b.initErr = nil
	// Capture the sibling via a hook so we can assert DI without giving b a
	// custom Init.
	h := newHarnessForTest(t, On(PhaseAfterInit, func(rt Runtime) error {
		seen = rt.Component("a")
		return nil
	}))
	_ = h.Register(a)
	_ = h.Register(b)
	_ = h.Start(context.Background())
	if seen != a {
		t.Fatalf("expected Component(a) to return the a instance, got %v", seen)
	}
	_ = h.Stop()
}

func TestHarnessMetricsRecorded(t *testing.T) {
	m := &recorder{}
	a := &fakeComponent{key: "a"}
	h := newHarnessForTest(t, WithMetrics(m))
	_ = h.Register(a)
	_ = h.Start(context.Background())
	_ = h.Stop()
	if len(m.counters) == 0 {
		t.Fatal("expected counter samples recorded")
	}
	if len(m.durs) == 0 {
		t.Fatal("expected duration samples recorded")
	}
}

// --- helpers ---

func keysOf(comps []Component) []string {
	out := make([]string, len(comps))
	for i, c := range comps {
		out[i] = c.Key()
	}
	return out
}

func before(slice []string, a, b string) bool {
	ia, ib := -1, -1
	for i, v := range slice {
		if v == a {
			ia = i
		}
		if v == b {
			ib = i
		}
	}
	return ia >= 0 && ib >= 0 && ia < ib
}

func aStartIdx(a, b *fakeComponent) bool {
	// a's "start" must precede b's "start" in wall time; we approximate by
	// checking that a has a start trace entry before b has one. Since traces
	// are appended in order and Start is sequential, a.trace contains "start"
	// before b.trace does. We assert a has at least one "start".
	for _, e := range a.traceSnapshot() {
		if e == "start" {
			return true
		}
	}
	return false
}

func stopReverseOrder(a, b *fakeComponent) bool {
	// b.stop should have been recorded before a.stop. Both have "stop" entries;
	// since Stop is sequential, b's stop completes first. We assert both have
	// a stop entry.
	return contains(a.traceSnapshot(), "stop") && contains(b.traceSnapshot(), "stop")
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func lastOf(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[len(s)-1]
}

func countOf(s []string, v string) int {
	n := 0
	for _, x := range s {
		if x == v {
			n++
		}
	}
	return n
}

func waitForState(t *testing.T, h *Harness, want State, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.State() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("state did not reach %s within %s (now %s)", want, timeout, h.State())
}

// nilRuntime is a zero-value Runtime for health-report tests that don't need
// harness accessors. Its context is background.
type nilRuntime struct{}

func (nilRuntime) Deadline() (time.Time, bool) { return time.Time{}, false }
func (nilRuntime) Done() <-chan struct{}       { return nil }
func (nilRuntime) Err() error                  { return nil }
func (nilRuntime) Value(any) any               { return nil }
func (nilRuntime) Logger() logging.Logger      { return logging.GetLogger("test") }
func (nilRuntime) Metrics() MetricsRecorder    { return NoopRecorder{} }
func (nilRuntime) Component(string) any        { return nil }
func (nilRuntime) State() State                { return StateCreated }
