package mobs

import "strings"

// buildAI assembles a mob's goal-selector and target-selector from its MobDef.
// It is the rombak's replacement for the old aiStep switch: instead of one big
// switch keyed on AIState, each mob gets a prioritised list of small goals.
// Priorities follow vanilla convention (lower number = more important) and
// match the real entity goal lists as closely as the available callbacks allow.
//
// Goal values are stateless and shared safely across all mobs of a type (per-
// mob mutable state lives on *Mob), so allocating them per mob here is cheap.
func buildAI(def MobDef) (goalSel, targetSel *goalSelector) {
	goalSel = newGoalSelector()
	targetSel = newGoalSelector()

	canMelee := def.AttackDamage > 0 || def.ThrowDamage > 0
	canRanged := def.HasRanged
	hasSwell := def.HasExplosion
	isSpider := def.Movement == "climb" || strings.Contains(def.Type, "spider")

	// --- goal selector (behaviour) ---------------------------------------

	// 0: float to the surface in water (every mob).
	goalSel.add(0, floatGoal{})

	// 1: passive panic when hurt.
	if !def.IsHostile && def.ThrowDamage == 0 {
		goalSel.add(1, panicGoal{})
	}

	// 2–4: attack behaviour. A mob may have several (creeper = swell + melee
	// approach; spider = leap + melee).
	if hasSwell {
		goalSel.add(2, swellGoal{})
		goalSel.add(3, meleeAttackGoal{}) // approach only (no damage → no swing)
	}
	if canRanged {
		goalSel.add(3, rangedAttackGoal{})
	}
	if canMelee && !hasSwell {
		goalSel.add(2, meleeAttackGoal{})
	}
	if isSpider {
		goalSel.add(4, leapAtTargetGoal{})
	}

	// 5: food lure (passive mobs with a FoodItem).
	if def.FoodItem != "" {
		goalSel.add(5, temptGoal{})
	}

	// 7: random stroll (default wander).
	goalSel.add(7, randomStrollGoal{})

	// 8: look at nearby players, then idle head drift. Hostile mobs watch
	// from farther (8 b) than passives (6 b).
	lookRange := 6.0
	if def.IsHostile {
		lookRange = 8.0
	}
	goalSel.add(8, lookAtPlayerGoal{rangeBlocks: lookRange})
	goalSel.add(8, randomLookAroundGoal{})

	// --- target selector (acquisition) -----------------------------------

	if canMelee || canRanged || hasSwell {
		// Retaliate against the last attacker (players + iron golem).
		targetSel.add(1, hurtByTargetGoal{})
	}
	switch {
	case def.AggravatedByGaze:
		// Enderman: neutral until stared at; brain-backed anger memory.
		targetSel.add(2, endermanGazeGoal{})
	case def.IsHostile:
		// Hostiles auto-aggro the nearest valid player. Neutral mobs
		// (iron golem) only retaliate via hurtByTargetGoal above.
		targetSel.add(2, nearestAttackableTargetGoal{})
	}

	return goalSel, targetSel
}
