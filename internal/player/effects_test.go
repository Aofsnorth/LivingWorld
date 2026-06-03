package player

import (
	"sync"
	"testing"

	"livingworld/internal/world"

	"github.com/google/uuid"
)

// fakeEffectBus records every WorldEffectEvent the manager publishes
// so tests can assert on add / remove traffic without spinning up the
// real world. Safe under concurrent publish.
type fakeEffectBus struct {
	mu  sync.Mutex
	evs []world.WorldEffectEvent
}

func (b *fakeEffectBus) PublishWorldEffect(ev world.WorldEffectEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.evs = append(b.evs, ev)
}

func (b *fakeEffectBus) last() (world.WorldEffectEvent, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.evs) == 0 {
		return world.WorldEffectEvent{}, false
	}
	return b.evs[len(b.evs)-1], true
}

func (b *fakeEffectBus) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.evs)
}

func newTestPlayer(t *testing.T) (*Manager, *Player, *fakeEffectBus) {
	t.Helper()
	m := NewManager()
	id := uuid.New()
	p := NewPlayer(id, "tester", EditionJava)
	m.AddPlayer(p)
	bus := &fakeEffectBus{}
	m.SetEffectBus(bus)
	return m, p, bus
}

func TestM6_EffectIDForHitEffectType(t *testing.T) {
	cases := map[string]int32{
		"hunger":   EffectHunger,
		"poison":   EffectPoison,
		"wither":   EffectWither,
		"slowness": EffectSlowness,
		"":         0, // empty / unknown → 0
		"bogus":    0,
	}
	for s, want := range cases {
		if got := EffectIDForHitEffectType(s); got != want {
			t.Errorf("EffectIDForHitEffectType(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestM6_AddEffect_PopulatesBag(t *testing.T) {
	m, p, bus := newTestPlayer(t)
	m.AddEffect(p.UUID, EffectPoison, 0, 200) // 10 s
	if !p.effects.has(EffectPoison) {
		t.Fatalf("poison effect not in bag after AddEffect")
	}
	if p.effects.active != 1 {
		t.Errorf("active=%d, want 1", p.effects.active)
	}
	// AddEffect must publish exactly one EffectStatus event.
	if got := bus.count(); got != 1 {
		t.Fatalf("event count = %d, want 1", got)
	}
	last, ok := bus.last()
	if !ok {
		t.Fatal("no event recorded")
	}
	if last.Kind != world.EffectStatus {
		t.Errorf("event kind = %q, want %q", last.Kind, world.EffectStatus)
	}
	if last.EffectID != EffectPoison {
		t.Errorf("event EffectID = %d, want %d", last.EffectID, EffectPoison)
	}
	if last.Aux != 200 {
		t.Errorf("event Aux (duration) = %d, want 200", last.Aux)
	}
}

func TestM6_RemoveEffect_ClearsBagAndPublishes(t *testing.T) {
	m, p, bus := newTestPlayer(t)
	m.AddEffect(p.UUID, EffectPoison, 0, 200)
	m.RemoveEffect(p.UUID, EffectPoison)
	if p.effects.has(EffectPoison) {
		t.Fatalf("poison effect still in bag after RemoveEffect")
	}
	if p.effects.active != 0 {
		t.Errorf("active=%d, want 0", p.effects.active)
	}
	if got := bus.count(); got != 2 {
		t.Fatalf("event count = %d, want 2 (1 add + 1 remove)", got)
	}
	last, _ := bus.last()
	if last.Kind != world.EffectStatusRemove {
		t.Errorf("event kind = %q, want %q", last.Kind, world.EffectStatusRemove)
	}
}

func TestM6_TickEffects_DecrementsAndDamages(t *testing.T) {
	m, p, _ := newTestPlayer(t)
	ctrl := &fakeController{}
	m.SetController(p.UUID, ctrl)

	// Poison: 0.5 HP every 10 ticks. Apply for 25 ticks.
	// First damage at tick 10, second at tick 20, expires at tick 25.
	m.AddEffect(p.UUID, EffectPoison, 0, 25)

	// Tick 1..9: no damage.
	for i := 0; i < 9; i++ {
		m.TickEffects()
	}
	if got := ctrl.hurtCount(); got != 0 {
		t.Errorf("after 9 ticks, hurt=%d, want 0", got)
	}
	if !p.effects.has(EffectPoison) {
		t.Fatalf("poison dropped before expiry")
	}

	// Tick 10: first damage.
	m.TickEffects()
	if got := ctrl.hurtCount(); got != 1 {
		t.Errorf("after 10 ticks, hurt=%d, want 1", got)
	}

	// Tick 11..19: 9 more, no damage.
	for i := 0; i < 9; i++ {
		m.TickEffects()
	}
	if got := ctrl.hurtCount(); got != 1 {
		t.Errorf("after 19 ticks, hurt=%d, want 1", got)
	}

	// Tick 20: second damage.
	m.TickEffects()
	if got := ctrl.hurtCount(); got != 2 {
		t.Errorf("after 20 ticks, hurt=%d, want 2", got)
	}

	// Tick 21..25: 5 more, expires.
	for i := 0; i < 5; i++ {
		m.TickEffects()
	}
	if p.effects.has(EffectPoison) {
		t.Fatalf("poison still in bag after 25 ticks")
	}
	if got := ctrl.hurtCount(); got != 2 {
		t.Errorf("after expiry, hurt=%d, want 2 (no third damage)", got)
	}
}

func TestM6_TickEffects_ExpiresPublishesRemove(t *testing.T) {
	m, p, bus := newTestPlayer(t)
	// 1-tick effect so the very first TickEffects expires it.
	m.AddEffect(p.UUID, EffectPoison, 0, 1)
	m.TickEffects()
	if p.effects.has(EffectPoison) {
		t.Fatalf("poison still in bag after 1-tick expiry")
	}
	last, ok := bus.last()
	if !ok {
		t.Fatal("no event recorded")
	}
	if last.Kind != world.EffectStatusRemove {
		t.Errorf("event kind = %q, want %q", last.Kind, world.EffectStatusRemove)
	}
}

func TestM6_AddEffect_UnknownType_NoOp(t *testing.T) {
	m, p, bus := newTestPlayer(t)
	m.AddEffect(p.UUID, 99, 0, 100) // invalid id
	if p.effects.active != 0 {
		t.Errorf("invalid effect id populated bag: active=%d", p.effects.active)
	}
	if got := bus.count(); got != 0 {
		t.Errorf("event count = %d, want 0 (invalid id must not publish)", got)
	}
}

func TestM6_AddEffect_OfflinePlayer_Dropped(t *testing.T) {
	m := NewManager()
	m.AddEffect(uuid.New(), EffectPoison, 0, 100) // no AddPlayer; must not panic
}

// fakeController is a minimal Controller that counts Hurt() calls.
type fakeController struct {
	mu    sync.Mutex
	hurts int
}

func (c *fakeController) SendMessage(string)             {}
func (c *fakeController) Kick(string)                    {}
func (c *fakeController) Push(float64, float64, float64) {}
func (c *fakeController) Hurt(float32) {
	c.mu.Lock()
	c.hurts++
	c.mu.Unlock()
}
func (c *fakeController) hurtCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hurts
}
