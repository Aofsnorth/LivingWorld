package mobs

import (
	"math/rand"
	"testing"
	"time"
)

// TestSunburn_NoReentrantDeadlock guards the fix for the latent self-deadlock:
// OnFireDamage is wired by the world layer back to Store.HurtFire, which
// re-locks the store mutex. Tick must defer those callbacks until after it
// releases the lock. A zombie standing in full sky light triggers the sun-burn
// system every tick; if the deferral regresses, Tick never returns.
func TestSunburn_NoReentrantDeadlock(t *testing.T) {
	s := New()
	z := s.Spawn("minecraft:zombie", 0, 64, 0)
	ctx := AIContext{
		RNG:        rand.New(rand.NewSource(1)),
		SolidAt:    func(x, y, z int) bool { return y < 64 },
		SkyLightAt: func(x, y, z int) uint8 { return 15 },
		Players:    func() []PlayerTarget { return nil },
		OnFireDamage: func(mobID int64, dmg float32) {
			s.HurtFire(mobID, dmg) // re-locks the same store
		},
	}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 5; i++ {
			s.Tick(ctx)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: Tick did not return (OnFireDamage re-entrancy)")
	}

	// The deferred HurtFire should have applied sun-burn HP loss and set the
	// fire overlay.
	got := s.Get(z.EntityID)
	if got.HP >= got.MaxHP {
		t.Errorf("sun-burn should have reduced HP: HP=%.2f MaxHP=%.2f", got.HP, got.MaxHP)
	}
	if got.FireTicks == 0 {
		t.Errorf("sun-burn should have set FireTicks for the fire overlay")
	}
}
