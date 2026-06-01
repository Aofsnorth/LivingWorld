// Package combat tests: pure damage-math functions must remain deterministic
// across the entire repository. Each test pins a vanilla behavior so a future
// refactor that silently changes parity will fail in CI rather than ship.
package combat

import (
	"math"
	"testing"
)

const eps = 1e-9

// floatEq compares floats with an epsilon suited to hand-computed damage
// values. Both inputs and expected are non-negative.
func floatEq(a, b float64) bool { return math.Abs(a-b) < eps }

// TestAfterArmorVanillaFormula covers the canonical Java formula:
//
//	reduction = min(20, max(armor/5, armor - dmg/(2+toughness/4)))
//	reduced   = dmg * (1 - reduction/25)
//
// Reference values computed by hand (one for each branch).
func TestAfterArmorVanillaFormula(t *testing.T) {
	cases := []struct {
		name             string
		dmg, armor, tough float64
		want             float64
	}{
		// No armor: full damage.
		{"no_armor", 7, 0, 0, 7},
		// armor/5 branch (5/5=1, max(1, 5-7/2)=max(1, 1.5)=1.5 → use 1.5)
		// 7 * (1 - 1.5/25) = 7 * 0.94 = 6.58
		{"small_armor", 7, 5, 0, 6.58},
		// armor - dmg/(2+tough/4) branch dominates when armor is large.
		// 20 - 19/(2+0) = 20 - 9.5 = 10.5; max(20/5=4, 10.5)=10.5
		// min(20, 10.5) = 10.5; 19 * (1 - 10.5/25) = 19 * 0.58 = 11.02
		{"high_armor", 19, 20, 0, 11.02},
		// Toughness softens armor penetration on big hits.
		// 20 - 100/(2+8/4) = 20 - 25 = -5; max(4, -5) = 4; 100*(1-4/25) = 100*0.84 = 84
		{"armor_breaks_with_tough", 100, 20, 8, 84},
		// Cap: reduction must never exceed 20.
		{"armor_capped", 1, 1000, 0, 1 * (1 - 20.0/25.0)},
		// Zero / negative damage is a no-op (returns 0, not negative).
		{"zero_damage", 0, 20, 0, 0},
		{"neg_damage", -5, 20, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AfterArmor(tc.dmg, tc.armor, tc.tough)
			if !floatEq(got, tc.want) {
				t.Errorf("AfterArmor(%v,%v,%v)=%v, want %v", tc.dmg, tc.armor, tc.tough, got, tc.want)
			}
		})
	}
}

// TestAfterResistance confirms 20% per level, capped at 0 damage at level >= 5.
func TestAfterResistance(t *testing.T) {
	cases := []struct {
		level int
		want  float64
	}{
		{0, 10},  // no resistance
		{1, 8},   // 10 * 0.8
		{2, 6},   // 10 * 0.6
		{4, 2},   // 10 * 0.2
		{5, 0},   // immune
		{6, 0},   // immune (level > 5)
		{-1, 10}, // defensive: negative level is a no-op
	}
	for _, tc := range cases {
		got := AfterResistance(10, tc.level)
		if !floatEq(got, tc.want) {
			t.Errorf("AfterResistance(10,%d)=%v, want %v", tc.level, got, tc.want)
		}
	}
}

// TestCriticalMultiplier pins the vanilla 1.5x crit factor on the math layer.
// The triggering conditions (falling, on-ground false, etc.) live in the
// attack handler, not here.
func TestCriticalMultiplier(t *testing.T) {
	if got := Critical(10); !floatEq(got, 15) {
		t.Errorf("Critical(10)=%v, want 15", got)
	}
	if got := Critical(0); !floatEq(got, 0) {
		t.Errorf("Critical(0)=%v, want 0", got)
	}
}
