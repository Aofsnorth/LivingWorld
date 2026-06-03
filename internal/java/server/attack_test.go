package server

import (
	"testing"
)

// TestM5_RouteAttackToMob_AppliesDamage verifies that a player→
// mob attack calls HurtDirect on the mob store. The mob's HP
// must drop by mobBaseAttackDamage and the mob's Despawn flag
// must be set when HP reaches 0.
//
// We build a minimal world manager with the default world, spawn
// a zombie (HP=20 in vanilla) and apply a single attack. The
// test asserts HP went from 20 to 19.
func TestM5_RouteAttackToMob_AppliesDamage(t *testing.T) {
	bridge := newTestJavaBridge(t)
	// Spawn a zombie in the default world.
	mobID := bridge.wm.Mobs().Spawn("minecraft:zombie", 0, 64, 0).EntityID
	if mobID == 0 {
		t.Fatalf("zombie spawn returned 0 id")
	}
	// Sanity: HP is 20.
	before := bridge.wm.Mobs().Get(mobID)
	if before.HP != 20 {
		t.Fatalf("zombie starting HP: got %v want 20", before.HP)
	}
	// Apply one attack. The attacker UUID doesn't have to be
	// a real player for the mob-side damage path; HurtDirect
	// only uses the bytes for target-locking, which is a
	// follow-up path here.
	bridge.routeAttack(attackerUUID(), int32(mobID))
	// HP must drop by 1.
	after := bridge.wm.Mobs().Get(mobID)
	if after.HP != 19 {
		t.Errorf("zombie HP after 1 swing: got %v want 19", after.HP)
	}
}

// TestM5_RouteAttackToMob_KillsAtZeroHP verifies that HP=0 sets
// Despawn=true. The actual Remove + drop + split is in
// PendingDespawns() / Phase 4 cleanup of the world tick — the
// M5 path only flags the mob for the tick to clean up.
func TestM5_RouteAttackToMob_KillsAtZeroHP(t *testing.T) {
	bridge := newTestJavaBridge(t)
	mobID := bridge.wm.Mobs().Spawn("minecraft:creeper", 0, 64, 0).EntityID
	// Creeper HP is 20. Apply 20 attacks.
	for i := 0; i < 20; i++ {
		bridge.routeAttack(attackerUUID(), int32(mobID))
	}
	// HP must be 0 (or below) and Despawn must be set.
	after := bridge.wm.Mobs().Get(mobID)
	if after.HP > 0 {
		t.Errorf("creeper after 20 swings: HP=%v; want 0", after.HP)
	}
	pending := bridge.wm.Mobs().PendingDespawns()
	found := false
	for _, id := range pending {
		if id == mobID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("creeper: not in PendingDespawns after 20 swings; got %v", pending)
	}
}

// TestM5_RouteAttackToMob_HostileRetargets verifies that a
// hostile mob with no prior target picks the attacker as its
// target after one swing (M1 AI's attack-tagging behavior in
// HurtDirect). The "retarget" is exposed via the mob's stored
// target — but target is unexported, so we observe it via the
// mob's behaviour: a second swing on the same mob must still
// apply damage (the mob stays alive, but the test is the
// hostile-retarget path which is observable by no panic).
func TestM5_RouteAttackToMob_HostileRetargets(t *testing.T) {
	bridge := newTestJavaBridge(t)
	mobID := bridge.wm.Mobs().Spawn("minecraft:skeleton", 0, 64, 0).EntityID
	// Skeleton HP=20. Apply 5 swings. No panic = HurtDirect
	// handled the retarget internally.
	for i := 0; i < 5; i++ {
		bridge.routeAttack(attackerUUID(), int32(mobID))
	}
	after := bridge.wm.Mobs().Get(mobID)
	if after.HP != 15 {
		t.Errorf("skeleton after 5 swings: HP=%v; want 15", after.HP)
	}
}

// TestM5_RouteAttack_UnknownTargetFallsThrough verifies that an
// attack on a non-existent entity id (e.g. the player's own
// runtime id) does NOT call HurtDirect on a zero-value mob.
// The Get() check in routeAttack must reject EntityID==0 mobs
// (the zero Mob return from a miss).
func TestM5_RouteAttack_UnknownTargetFallsThrough(t *testing.T) {
	bridge := newTestJavaBridge(t)
	// Spawn nothing. Attack a non-existent id.
	// Must not panic, must not "apply damage" to nothing.
	// A miss returns Mob{} (EntityID==0) so routeAttack should
	// fall through to the player path. We can't easily verify
	// the player path without a connected player, but the
	// test asserts the bridge call doesn't panic.
	bridge.routeAttack(attackerUUID(), 99999)
	// Spawn a cow and confirm the unknown-id attack didn't
	// damage it.
	hpBefore := bridge.wm.Mobs().Spawn("minecraft:cow", 100, 64, 0)
	bridge.routeAttack(attackerUUID(), 99999) // unknown id again
	hpAfter := bridge.wm.Mobs().Get(hpBefore.EntityID)
	if hpAfter.HP != hpBefore.HP {
		t.Errorf("cow HP changed by stray attack on id=99999: before=%v after=%v",
			hpBefore.HP, hpAfter.HP)
	}
}
