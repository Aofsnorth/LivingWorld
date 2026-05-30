package plugin

import "testing"

func TestTypedHandlerReceivesEvent(t *testing.T) {
	m := NewManager()
	var got string
	m.OnPlayerJoin(func(e *PlayerJoinEvent) { got = e.PlayerName })

	m.Emit(&PlayerJoinEvent{BaseEvent: BaseEvent{Type_: EventPlayerJoin}, PlayerName: "Arthenyx"})
	if got != "Arthenyx" {
		t.Fatalf("handler saw %q, want Arthenyx", got)
	}
}

func TestEmitCancellable(t *testing.T) {
	m := NewManager()
	m.OnBlockBreak(func(e *BlockBreakEvent) {
		if e.BlockID == 7 { // protect bedrock-ish
			e.Cancel()
		}
	})

	protectedEvt := &BlockBreakEvent{BaseEvent: BaseEvent{Type_: EventBlockBreak}, BlockID: 7}
	if !m.EmitCancellable(protectedEvt) {
		t.Errorf("expected protected break to be cancelled")
	}

	ok := &BlockBreakEvent{BaseEvent: BaseEvent{Type_: EventBlockBreak}, BlockID: 1}
	if m.EmitCancellable(ok) {
		t.Errorf("expected normal break to proceed")
	}
}

// fakePlugin verifies the Plugin lifecycle and Host delivery.
type fakePlugin struct {
	enabled bool
	host    Host
}

func (f *fakePlugin) Name() string    { return "fake" }
func (f *fakePlugin) Version() string { return "1.0" }
func (f *fakePlugin) OnEnable(h Host) error {
	f.enabled = true
	f.host = h
	return nil
}
func (f *fakePlugin) OnDisable() error { f.enabled = false; return nil }

func TestPluginLifecycle(t *testing.T) {
	m := NewManager()
	p := &fakePlugin{}
	if err := m.Register(p); err != nil {
		t.Fatalf("register: %v", err)
	}
	if !p.enabled {
		t.Errorf("plugin not enabled")
	}
	if err := m.Register(p); err == nil {
		t.Errorf("duplicate registration should fail")
	}
	if got := m.List(); len(got) != 1 || got[0] != "fake" {
		t.Errorf("List() = %v", got)
	}
	if err := m.Unregister("fake"); err != nil {
		t.Fatalf("unregister: %v", err)
	}
	if p.enabled {
		t.Errorf("plugin still enabled after unregister")
	}
}


// panicPlugin registers a handler that panics, to verify isolation + auto-disable.
type panicPlugin struct {
	m     *PluginManager
	calls int
}

func (p *panicPlugin) Name() string    { return "panicker" }
func (p *panicPlugin) Version() string { return "1.0" }
func (p *panicPlugin) OnEnable(h Host) error {
	p.m.OnPlayerJoin(func(e *PlayerJoinEvent) { p.calls++; panic("boom") })
	return nil
}
func (p *panicPlugin) OnDisable() error { return nil }

func TestHandlerPanicIsolatedAndPluginDisabled(t *testing.T) {
	m := NewManager()
	p := &panicPlugin{m: m}
	if err := m.Register(p); err != nil {
		t.Fatalf("register: %v", err)
	}

	// First emit triggers the panic; it must be recovered (no crash) and the
	// owning plugin disabled along with its handler.
	m.Emit(&PlayerJoinEvent{BaseEvent: BaseEvent{Type_: EventPlayerJoin}, PlayerName: "a"})
	if got := m.List(); len(got) != 0 {
		t.Fatalf("expected plugin disabled, still registered: %v", got)
	}

	// Second emit must not re-invoke the removed handler.
	m.Emit(&PlayerJoinEvent{BaseEvent: BaseEvent{Type_: EventPlayerJoin}, PlayerName: "b"})
	if p.calls != 1 {
		t.Fatalf("handler invoked %d times, want 1 (should be removed after panic)", p.calls)
	}
}
