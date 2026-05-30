package combat

import (
	"testing"

	"livingworld/internal/registry"
)

func TestKnockbackBareHit(t *testing.T) {
	// Attacker due +X of victim → victim pushed −X; on ground sets vertical pop.
	got := Knockback(registry.Vec3{}, 0.4, 1, 0, 0, true)
	if !approx(got.X, -0.4) || !approx(got.Z, 0) {
		t.Fatalf("horizontal=(%v,%v) want (-0.4,0)", got.X, got.Z)
	}
	if !approx(got.Y, 0.4) { // min(0.4, 0/2 + 0.4)
		t.Fatalf("Y=%v want 0.4", got.Y)
	}
}

func TestKnockbackFullResistance(t *testing.T) {
	v := registry.Vec3{X: 1, Y: 2, Z: 3}
	if got := Knockback(v, 0.4, 1, 0, 1, true); got != v {
		t.Errorf("full resistance: got %v want unchanged %v", got, v)
	}
}

func TestCritical(t *testing.T) {
	if got := Critical(6); !approx(got, 9) {
		t.Errorf("Critical(6)=%v want 9", got)
	}
}
