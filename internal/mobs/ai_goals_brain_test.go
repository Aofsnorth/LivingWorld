package mobs

import (
	"math"
	"testing"
)

// lookingAtYaw returns the Minecraft yaw (degrees) for a player at (px,pz)
// looking toward (tx,tz) on the horizontal plane.
func lookingAtYaw(px, pz, tx, tz float64) float64 {
	dx, dz := tx-px, tz-pz
	return math.Atan2(-dx, dz) * 180 / math.Pi
}

// TestEnderman_IgnoresNonGazingPlayer verifies a nearby player who isn't
// looking at the enderman is not targeted.
func TestEnderman_IgnoresNonGazingPlayer(t *testing.T) {
	s := New()
	e := s.Spawn("minecraft:enderman", 0, 64, 0)
	// Player at +X=5 looking AWAY (+X / east = yaw 270), not at the origin.
	p := PlayerTarget{UUID: [16]byte{1}, X: 5, Y: 64, Z: 0, LookYaw: 270, LookPitch: 0}
	ctx := aiTestCtx([]PlayerTarget{p})

	for i := 0; i < 10; i++ {
		s.Tick(ctx)
	}
	if got := s.Get(e.EntityID); got.target != zero16() {
		t.Errorf("enderman should not target a non-gazing player, target=%v", got.target)
	}
}

// TestEnderman_AggrosGazingPlayer verifies a player staring at the enderman
// provokes it, and the anger persists after they look away.
func TestEnderman_AggrosGazingPlayer(t *testing.T) {
	s := New()
	e := s.Spawn("minecraft:enderman", 0, 64, 0)
	// Player at +X=5 looking toward the enderman at origin → yaw faces -X.
	stareYaw := lookingAtYaw(5, 0, 0, 0)
	p := PlayerTarget{UUID: [16]byte{2}, X: 5, Y: 64, Z: 0, LookYaw: stareYaw, LookPitch: 0}
	ctx := aiTestCtx([]PlayerTarget{p})

	s.Tick(ctx)
	if got := s.Get(e.EntityID); got.target != p.UUID {
		t.Fatalf("enderman should aggro a gazing player, target=%v", got.target)
	}

	// Player looks away; anger memory should keep the target.
	p.LookYaw = stareYaw + 180
	ctx = aiTestCtx([]PlayerTarget{p})
	for i := 0; i < 5; i++ {
		s.Tick(ctx)
	}
	if got := s.Get(e.EntityID); got.target != p.UUID {
		t.Errorf("enderman should stay angry after the player looks away, target=%v", got.target)
	}
}

// TestEnderman_RetaliatesWhenHit verifies hitting an enderman (no gaze) still
// aggros it via the hurt-by target goal.
func TestEnderman_RetaliatesWhenHit(t *testing.T) {
	s := New()
	e := s.Spawn("minecraft:enderman", 0, 64, 0)
	att := PlayerTarget{UUID: [16]byte{3}, X: 3, Y: 64, Z: 0, LookYaw: 90}
	ctx := aiTestCtx([]PlayerTarget{att})

	s.Tick(ctx)
	s.Hurt(e.EntityID, att.UUID)
	s.Tick(ctx)
	if got := s.Get(e.EntityID); got.target != att.UUID {
		t.Errorf("enderman should retaliate against its attacker, target=%v", got.target)
	}
}
