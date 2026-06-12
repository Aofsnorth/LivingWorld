// Package ainavigation is the vanilla "pathfinding malus" model
// (spec § Replikasi Pathfinding Dinamis). Every mob shares one A*
// core (mobs.PathFind), but each type carries its own table of
// per-block penalties so the same search produces
// species-appropriate routes: a Strider strolls over lava, a Blaze
// is reluctant on it, a land mob refuses water, an enderman avoids
// it entirely.
//
// A profile is looked up by mob type and combined with the world's
// AIContext.BlockNameAt probe to build the costFn handed to
// PathFind. Profiles are immutable and shared across all mobs of a
// type.
//
// The package is Mob-free: it takes (mobType, movementMode) strings
// instead of a mobs.MobDef struct to avoid an import cycle (mobs
// imports ainavigation for the NavProfile type, ainavigation must
// not import mobs back). The cost sentinel (MalusBlocked) lives
// here as well.
package ainavigation

import (
	// Alias the package to `context` so the method signatures
	// read as `*context.AIContext` (matching the rest of the ai
	// subpackages). The stdlib `context` is not used in this
	// file.
	context "livingworld/internal/mobs/ai/context"
)

// MalusBlocked is the cost sentinel for "this cell is impassable".
// The path package defines the same value; the two constants are
// kept in sync manually because each package owns its own
// (cross-package consts would re-introduce the import cycle this
// package avoids).
const MalusBlocked = 1e6

// NavType classifies how a mob traverses the world. It mirrors the
// Bedrock minecraft:navigation.* component family and selects which
// media the mob may enter.
type NavType int

const (
	NavWalk NavType = iota // ground mobs (zombie, cow, …)
	NavClimb               // spiders — walk + wall climb
	NavHop                 // slime / magma cube — jump locomotion
	NavFly                 // phantom / blaze — airborne
	NavSwim                // drowned — water-capable
)

// NavProfile is a mob type's traversal capabilities + hazard
// penalties.
type NavProfile struct {
	Type  NavType
	Malus map[string]float64 // block id → penalty (MalusBlocked = impassable)
}

// Block ids referenced by the malus tables. Centralised so a rename
// is a one-line change.
const (
	BlkLava       = "minecraft:lava"
	BlkWater      = "minecraft:water"
	BlkPowderSnow = "minecraft:powder_snow"
	BlkFire       = "minecraft:fire"
	BlkMagmaBlock = "minecraft:magma_block"
	BlkSweetBerry = "minecraft:sweet_berry_bush"
	BlkCactus     = "minecraft:cactus"
)

// LandMalus is the default penalty table for ground mobs: lava and
// powder snow are lethal (impassable), open water and damaging
// blocks are strongly avoided but technically pathable so a
// cornered mob still has a route.
func LandMalus() map[string]float64 {
	return map[string]float64{
		BlkLava:       MalusBlocked,
		BlkPowderSnow: MalusBlocked,
		BlkWater:      8,
		BlkFire:       MalusBlocked,
		BlkMagmaBlock: 8,
		BlkCactus:     MalusBlocked,
		BlkSweetBerry: 4,
	}
}

// NavProfileFor builds the immutable profile for a mob type.
// mobType is the namespaced mob id ("minecraft:zombie"); movement
// is the locomotion mode ("walk" / "fly" / "hover" / "climb" /
// "hop"). Special cases follow the spec's worked examples; the
// rest fall back to the land defaults.
func NavProfileFor(mobType, movement string) NavProfile {
	switch mobType {
	case "minecraft:strider":
		// Lives on the lava sea: lava neutral, water blocked.
		return NavProfile{Type: NavWalk, Malus: map[string]float64{
			BlkLava: 0, BlkWater: MalusBlocked, BlkPowderSnow: MalusBlocked,
		}}
	case "minecraft:blaze":
		// Nether native: reluctant on lava (8), flies, water blocked.
		return NavProfile{Type: NavFly, Malus: map[string]float64{
			BlkLava: 8, BlkWater: MalusBlocked,
		}}
	case "minecraft:magma_cube", "minecraft:wither_skeleton", "minecraft:zombified_piglin":
		// Fire-immune Nether mobs: tolerate lava at a cost, avoid water.
		return NavProfile{Type: NavTypeFor(movement), Malus: map[string]float64{
			BlkLava: 8, BlkWater: 8,
		}}
	case "minecraft:drowned":
		// Amphibious: water neutral, lava lethal.
		return NavProfile{Type: NavSwim, Malus: map[string]float64{
			BlkLava: MalusBlocked, BlkPowderSnow: MalusBlocked,
		}}
	case "minecraft:enderman":
		// Water damages endermen — avoid entirely.
		mal := LandMalus()
		mal[BlkWater] = MalusBlocked
		return NavProfile{Type: NavWalk, Malus: mal}
	}
	switch movement {
	case "fly", "hover":
		return NavProfile{Type: NavFly, Malus: map[string]float64{BlkWater: MalusBlocked}}
	case "climb":
		return NavProfile{Type: NavClimb, Malus: LandMalus()}
	case "hop":
		return NavProfile{Type: NavHop, Malus: LandMalus()}
	default:
		return NavProfile{Type: NavWalk, Malus: LandMalus()}
	}
}

// NavTypeFor maps a movement mode to a NavType (used when the
// special-case branch already fixed the malus but still wants the
// right locomotion class).
func NavTypeFor(movement string) NavType {
	switch movement {
	case "fly", "hover":
		return NavFly
	case "climb":
		return NavClimb
	case "hop":
		return NavHop
	default:
		return NavWalk
	}
}

// Cost returns the costFn for this profile bound to the world
// probe, or nil if the profile has no penalties or no probe is
// available (PathFind then runs uniform-cost).
func (p NavProfile) Cost(ctx *context.AIContext) mobs_CostFn {
	if ctx.BlockNameAt == nil || len(p.Malus) == 0 {
		return nil
	}
	malus := p.Malus
	probe := ctx.BlockNameAt
	return func(x, y, z int) float64 {
		if m, ok := malus[probe(x, y, z)]; ok {
			return m
		}
		return 0
	}
}

// mobs_CostFn is a forward declaration of mobs.CostFn to break the
// import cycle. The path package's CostFn type and the navigation
// package's Cost() return type are the same shape (a function
// pointer), and Go's structural typing lets the path package
// accept the value the navigation package produces.
type mobs_CostFn = func(x, y, z int) float64
