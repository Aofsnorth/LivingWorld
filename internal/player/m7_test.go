package player

import (
	"testing"

	"github.com/google/uuid"
)

// TestM7_HitIFrames_StampsWindow verifies HitIFrames sets
// Player.IFrames to the supplied value when the player is
// online, and is a no-op for offline players.
func TestM7_HitIFrames_StampsWindow(t *testing.T) {
	m := NewManager()
	id := uuid.New()
	p := NewPlayer(id, "tester", EditionJava)
	m.AddPlayer(p)

	m.HitIFrames(id, 20)
	if p.IFrames != 20 {
		t.Errorf("IFrames = %d, want 20", p.IFrames)
	}
	// Refresh with a new value overwrites.
	m.HitIFrames(id, 10)
	if p.IFrames != 10 {
		t.Errorf("IFrames after refresh = %d, want 10", p.IFrames)
	}
	// Offline player: no panic, no-op.
	m.HitIFrames(uuid.New(), 20)
}

// TestM7_HitIFrames_NonPositiveNoOp asserts ticks<=0 is ignored
// (the bridge never passes 0, but the contract is "stamp a
// positive window or do nothing").
func TestM7_HitIFrames_NonPositiveNoOp(t *testing.T) {
	m := NewManager()
	id := uuid.New()
	p := NewPlayer(id, "tester", EditionJava)
	m.AddPlayer(p)
	m.HitIFrames(id, 0)
	if p.IFrames != 0 {
		t.Errorf("IFrames = %d, want 0 (zero-tick no-op)", p.IFrames)
	}
	m.HitIFrames(id, -5)
	if p.IFrames != 0 {
		t.Errorf("IFrames = %d, want 0 (negative-tick no-op)", p.IFrames)
	}
}

// TestM7_IFramesTick_DecrementsConnected drives IFramesTick
// across N invocations to confirm each tick decrements the
// counter by exactly 1, and the counter stops at 0 (no
// underflow).
func TestM7_IFramesTick_DecrementsConnected(t *testing.T) {
	m := NewManager()
	id := uuid.New()
	p := NewPlayer(id, "tester", EditionJava)
	m.AddPlayer(p)
	m.HitIFrames(id, 5)
	for i := 4; i >= 0; i-- {
		m.IFramesTick()
		if p.IFrames != i {
			t.Errorf("after tick %d: IFrames = %d, want %d", 5-i, p.IFrames, i)
		}
	}
	// One more tick should not underflow.
	m.IFramesTick()
	if p.IFrames != 0 {
		t.Errorf("IFrames = %d, want 0 (no underflow)", p.IFrames)
	}
}

// TestM7_IFramesTick_SkipsIdle asserts IFramesTick is a no-op
// for players with IFrames == 0 — the read-locked pass is
// supposed to short-circuit so an empty-effect-room tick is
// cheap.
func TestM7_IFramesTick_SkipsIdle(t *testing.T) {
	m := NewManager()
	id := uuid.New()
	p := NewPlayer(id, "tester", EditionJava)
	m.AddPlayer(p)
	if p.IFrames != 0 {
		t.Fatalf("precondition: IFrames = %d, want 0", p.IFrames)
	}
	m.IFramesTick()
	if p.IFrames != 0 {
		t.Errorf("IFrames = %d after tick, want 0 (idle pass)", p.IFrames)
	}
}
