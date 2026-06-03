package world

import (
	"livingworld/internal/mobs"
	"testing"
)

// TestM7_DeathSpawnsXPOrb verifies the world-level glue: when a
// mob is killed (HP → 0), the OnDeath listener registered in
// NewManager asks the drops store to spawn an XP orb at the
// mob's last position. The number of orbs should match the
// XPRewardFor value (5 for a standard hostile).
func TestM7_DeathSpawnsXPOrb(t *testing.T) {
	mgr := NewManager()
	m := mgr.Mobs().Spawn("minecraft:zombie", 0, 64, 0)
	// Force HP to 1 so a single hit kills.
	mgr.Mobs().OnDeath(func(snap mobs.Mob) {
		// intentionally empty — the manager's listener is the one
		// we want to verify. This second listener is a no-op.
	})
	// Reach into the mob to drop HP to 1.
	mgr.Mobs().Get(m.EntityID) // sanity
	// We can't easily drop HP from outside the package, so
	// dispatch a single 99-damage hit through HurtDirect —
	// it will mark Despawn=true and fire the OnDeath listener.
	var attacker [16]byte
	mgr.Mobs().HurtDirectWithKnockback(m.EntityID, attacker, 99, 1, 0, 0)

	orbs := mgr.Drops().Orbs()
	if len(orbs) != 1 {
		t.Fatalf("expected 1 XP orb on zombie kill, got %d", len(orbs))
	}
	if orbs[0].Experience != 5 {
		t.Errorf("zombie orb XP = %d, want 5", orbs[0].Experience)
	}
	if orbs[0].EntityID == 0 {
		t.Error("orb has zero entity id")
	}
}

// TestM7_DeathZeroXPSkipsOrb asserts the manager's OnDeath
// listener is a no-op for iron_golem (XPRewardFor returns 0).
func TestM7_DeathZeroXPSkipsOrb(t *testing.T) {
	mgr := NewManager()
	m := mgr.Mobs().Spawn("minecraft:iron_golem", 0, 64, 0)
	var attacker [16]byte
	mgr.Mobs().HurtDirectWithKnockback(m.EntityID, attacker, 999, 1, 0, 0)
	if n := len(mgr.Drops().Orbs()); n != 0 {
		t.Errorf("iron golem drop count = %d, want 0", n)
	}
}

// TestM7_NonLethalDoesNotSpawnOrb asserts the OnDeath listener
// does NOT fire on a non-lethal hit, so no orb is created.
func TestM7_NonLethalDoesNotSpawnOrb(t *testing.T) {
	mgr := NewManager()
	m := mgr.Mobs().Spawn("minecraft:zombie", 0, 64, 0)
	var attacker [16]byte
	// 1 damage vs HP 20: non-lethal.
	mgr.Mobs().HurtDirectWithKnockback(m.EntityID, attacker, 1, 1, 0, 0)
	if n := len(mgr.Drops().Orbs()); n != 0 {
		t.Errorf("non-lethal hit spawned %d orbs, want 0", n)
	}
}
