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
