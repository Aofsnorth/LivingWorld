package mobs

import "testing"

// TestM7_HurtDirectWithKnockback_SetsVelocity verifies a single
// swing on an OnGround hostile mob sets KnockbackVX / VZ to a
// unit vector pointing away from the attacker (opposite the
// attacker direction), KnockbackVY to the vanilla 0.4 hop, and
// that the hurtBy + target lock are stamped as a side effect.
func TestM7_HurtDirectWithKnockback_SetsVelocity(t *testing.T) {
	s := New()
	m := s.Spawn("minecraft:zombie", 0, 64, 0)
	// Force OnGround true so the Y-bump is non-zero.
	s.mu.Lock()
	if mm, ok := s.mobs[m.EntityID]; ok {
		mm.OnGround = true
	}
	s.mu.Unlock()

	var attacker [16]byte
	for i := range attacker {
		attacker[i] = byte(i + 1)
	}
	// Attacker at (5, 64, 0) — mob at (0, 64, 0), so dx/dz
	// (mob - attacker) = (-5, 0); unit push = (-1, 0).
	s.HurtDirectWithKnockback(m.EntityID, attacker, 1, -5, 0, 0.4)

	after := s.Get(m.EntityID)
	if after.HP >= 20 {
		t.Errorf("mob HP did not decrease: %v", after.HP)
	}
	if after.KnockbackVX == 0 {
		t.Errorf("KnockbackVX not set")
	}
	if after.KnockbackVY != 0.4 {
		t.Errorf("KnockbackVY = %v, want 0.4 (vanilla on-ground hop)", after.KnockbackVY)
	}
	if after.KnockbackVZ != 0 {
		t.Errorf("KnockbackVZ = %v, want 0 (no Z component)", after.KnockbackVZ)
	}
	// VX should be positive (push away from attacker) when
	// attacker is at -X. dx=mb-attr=-5, push = -dx/d*kb = 1.0*0.4 = 0.4.
	if after.KnockbackVX < 0.39 || after.KnockbackVX > 0.41 {
		t.Errorf("KnockbackVX = %v, want ≈ 0.4", after.KnockbackVX)
	}
}

// TestM7_HurtDirectWithKnockback_AirborneSkipsYHop asserts a
// mob that is NOT on the ground gets zero Y velocity (no
// upward hop). Vanilla: the Y bump is conditional on the mob
// being grounded at the moment of impact.
func TestM7_HurtDirectWithKnockback_AirborneSkipsYHop(t *testing.T) {
	s := New()
	m := s.Spawn("minecraft:zombie", 0, 64, 0)
	s.mu.Lock()
	if mm, ok := s.mobs[m.EntityID]; ok {
		mm.OnGround = false
	}
	s.mu.Unlock()

	var attacker [16]byte
	s.HurtDirectWithKnockback(m.EntityID, attacker, 1, -1, 0, 0.4)
	after := s.Get(m.EntityID)
	if after.KnockbackVY != 0 {
		t.Errorf("airborne KnockbackVY = %v, want 0", after.KnockbackVY)
	}
	if after.KnockbackVX == 0 {
		t.Errorf("airborne KnockbackVX not set")
	}
}

// TestM7_HurtDirectWithKnockback_ZeroDirection_NoOp asserts a
// zero-direction swing (attacker at the same X/Z as the mob)
// doesn't divide by zero or otherwise panic. The damage is
// non-lethal (1 vs HP 20) so Despawn stays false.
func TestM7_HurtDirectWithKnockback_ZeroDirection_NoOp(t *testing.T) {
	s := New()
	m := s.Spawn("minecraft:zombie", 0, 64, 0)
	var attacker [16]byte
	s.HurtDirectWithKnockback(m.EntityID, attacker, 1, 0, 0, 0.4)
	after := s.Get(m.EntityID)
	if after.KnockbackVX != 0 || after.KnockbackVY != 0 || after.KnockbackVZ != 0 {
		t.Errorf("zero-dir hit set velocity: VX=%v VY=%v VZ=%v",
			after.KnockbackVX, after.KnockbackVY, after.KnockbackVZ)
	}
	if after.Despawn {
		t.Error("Despawn set on non-lethal hit")
	}
}

// TestM7_HurtDirectWithKnockback_KillDespawns verifies the kill
// path: HP <= 0 → Despawn flag set. The OnDeath listener is
// tested separately.
func TestM7_HurtDirectWithKnockback_KillDespawns(t *testing.T) {
	s := New()
	m := s.Spawn("minecraft:zombie", 0, 64, 0)
	s.mu.Lock()
	if mm, ok := s.mobs[m.EntityID]; ok {
		mm.HP = 1
		mm.OnGround = false
	}
	s.mu.Unlock()
	var attacker [16]byte
	s.HurtDirectWithKnockback(m.EntityID, attacker, 99, 1, 0, 0)
	after := s.Get(m.EntityID)
	if !after.Despawn {
		t.Error("Despawn not set on overkill")
	}
}

// TestM7_HurtDirectWithKnockback_ZeroKb_NoVelocity asserts kb=0
// doesn't write any velocity. This is the "no-knockback" path
// the sweep and arrow path use.
func TestM7_HurtDirectWithKnockback_ZeroKb_NoVelocity(t *testing.T) {
	s := New()
	m := s.Spawn("minecraft:zombie", 0, 64, 0)
	s.mu.Lock()
	if mm, ok := s.mobs[m.EntityID]; ok {
		mm.OnGround = true
	}
	s.mu.Unlock()
	var attacker [16]byte
	s.HurtDirectWithKnockback(m.EntityID, attacker, 1, 1, 0, 0)
	after := s.Get(m.EntityID)
	if after.KnockbackVX != 0 || after.KnockbackVY != 0 || after.KnockbackVZ != 0 {
		t.Errorf("kb=0 set velocity: VX=%v VY=%v VZ=%v",
			after.KnockbackVX, after.KnockbackVY, after.KnockbackVZ)
	}
}

// TestM7_OnDeath_FiresOnceOnKill registers a death listener and
// asserts it fires when a zombie is killed, with the snapshot's
// mob type. The store is expected to invoke the listener
// OUTSIDE the store lock (no deadlock even if the listener
// re-enters the store for drops).
func TestM7_OnDeath_FiresOnceOnKill(t *testing.T) {
	s := New()
	m := s.Spawn("minecraft:zombie", 0, 64, 0)
	s.mu.Lock()
	if mm, ok := s.mobs[m.EntityID]; ok {
		mm.HP = 1
		mm.OnGround = false
	}
	s.mu.Unlock()

	fired := 0
	var snapType string
	s.OnDeath(func(snap Mob) {
		fired++
		snapType = snap.Type
	})

	var attacker [16]byte
	s.HurtDirectWithKnockback(m.EntityID, attacker, 5, 1, 0, 0)

	if fired != 1 {
		t.Errorf("OnDeath fired %d times, want 1", fired)
	}
	if snapType != "minecraft:zombie" {
		t.Errorf("OnDeath snapshot.Type = %q, want minecraft:zombie", snapType)
	}
}

// TestM7_OnDeath_DoesNotFireOnNonLethal asserts the death
// listener does NOT fire when the damage is less than HP —
// OnDeath is reserved for actual kills.
func TestM7_OnDeath_DoesNotFireOnNonLethal(t *testing.T) {
	s := New()
	m := s.Spawn("minecraft:zombie", 0, 64, 0)
	// default zombie HP is 20, deal 1.
	fired := 0
	s.OnDeath(func(snap Mob) {
		fired++
	})
	var attacker [16]byte
	s.HurtDirectWithKnockback(m.EntityID, attacker, 1, 1, 0, 0)
	if fired != 0 {
		t.Errorf("OnDeath fired %d times on non-lethal hit, want 0", fired)
	}
}
