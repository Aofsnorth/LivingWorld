// Package aisystems_build holds the goal-selector factory. It lives
// in the same package as systems.go so buildAI can wire up the
// per-mob goal lists from a MobDef without crossing another package
// boundary.
package aisystems

import (
	"livingworld/internal/mobs"
	attack "livingworld/internal/mobs/ai/goals/attack"
	brain "livingworld/internal/mobs/ai/goals/brain"
	look "livingworld/internal/mobs/ai/goals/look"
	move "livingworld/internal/mobs/ai/goals/move"
	target "livingworld/internal/mobs/ai/goals/target"
	selector "livingworld/internal/mobs/ai/selector"
	"strings"
)

// buildAI assembles a mob's goal-selector and target-selector from
// its MobDef. Each mob gets a prioritised list of small goals.
// Priorities follow vanilla convention (lower number = more
// important) and match the real entity goal lists as closely as the
// available callbacks allow.
//
// Goal values are stateless and shared safely across all mobs of a
// type (per-mob mutable state lives on *mobs.Mob).
func buildAI(def mobs.MobDef) (goalSel, targetSel *selector.GoalSelector) {
	goalSel = selector.NewGoalSelector()
	targetSel = selector.NewGoalSelector()

	canMelee := def.AttackDamage > 0 || def.ThrowDamage > 0
	canRanged := def.HasRanged
	hasSwell := def.HasExplosion
	isSpider := def.Movement == "climb" || strings.Contains(def.Type, "spider")

	// 0: float to the surface in water (every mob).
	goalSel.Add(0, move.FloatGoal{})

	// 1: passive panic when hurt.
	if !def.IsHostile && def.ThrowDamage == 0 {
		goalSel.Add(1, move.PanicGoal{})
	}

	// 2-4: attack behaviour.
	if hasSwell {
		goalSel.Add(2, attack.SwellGoal{})
		goalSel.Add(3, attack.MeleeAttackGoal{})
	}
	if canRanged {
		goalSel.Add(3, attack.RangedAttackGoal{})
	}
	if canMelee && !hasSwell {
		goalSel.Add(2, attack.MeleeAttackGoal{})
	}
	if isSpider {
		goalSel.Add(4, attack.LeapAtTargetGoal{})
	}

	// 5: food lure.
	if def.FoodItem != "" {
		goalSel.Add(5, move.TemptGoal{})
	}

	// 7: random stroll.
	goalSel.Add(7, move.RandomStrollGoal{})

	// 8: look-at-player + random-look-around.
	lookRange := 6.0
	if def.IsHostile {
		lookRange = 8.0
	}
	goalSel.Add(8, look.LookAtPlayerGoal{RangeBlocks: lookRange})
	goalSel.Add(8, look.RandomLookAroundGoal{})

	// --- target selector ---
	if canMelee || canRanged || hasSwell {
		targetSel.Add(1, target.HurtByTargetGoal{})
	}
	switch {
	case def.AggravatedByGaze:
		targetSel.Add(2, brain.EndermanGazeGoal{})
	case def.IsHostile:
		targetSel.Add(2, target.NearestAttackableTargetGoal{})
	}

	return goalSel, targetSel
}
