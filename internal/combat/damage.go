// Package combat implements LivingWorld's edition-agnostic combat math:
// damage reduction, knockback, and (later) status effects. Functions here are
// pure and independent of the canonical entity model (DESIGN §4), so they can
// land before the shared registry/types exist. See REQUIREMENTS R5.4.
package combat

import "math"

// AfterArmor returns incoming damage reduced by armor using the vanilla Java
// formula. Both armor points and toughness contribute, and high-damage hits
// partially bypass armor:
//
//	reduced = dmg * (1 - min(20, max(armor/5, armor - dmg/(2+toughness/4))) / 25)
func AfterArmor(damage, armorPoints, armorToughness float64) float64 {
	if damage <= 0 {
		return 0
	}
	reduction := math.Min(20, math.Max(armorPoints/5, armorPoints-damage/(2+armorToughness/4)))
	return damage * (1 - reduction/25)
}

// AfterResistance applies the Resistance status effect: each level cuts damage
// by 20%; level >= 5 makes the target immune. Non-positive levels are no-ops.
func AfterResistance(damage float64, level int) float64 {
	if level <= 0 {
		return damage
	}
	if factor := 1 - 0.2*float64(level); factor > 0 {
		return damage * factor
	}
	return 0
}


// Critical applies the vanilla critical-hit multiplier (×1.5) to base attack
// damage. The trigger conditions (attacker falling, not on the ground, not
// sprinting, …) are decided by the attack handler; this is the math.
func Critical(damage float64) float64 { return damage * 1.5 }