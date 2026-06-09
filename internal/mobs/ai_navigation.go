package mobs

// Navigation profiles — the vanilla "pathfinding malus" model (spec §
// Replikasi Pathfinding Dinamis). Every mob shares one A* core (path.go), but
// each type carries its own table of per-block penalties so the same search
// produces species-appropriate routes: a Strider strolls over lava, a Blaze is
// reluctant on it, a land mob refuses water, an enderman avoids it entirely.
//
// A profile is looked up by mob type and combined with the world's
// AIContext.BlockNameAt probe to build the costFn handed to PathFind. Profiles
// are immutable and shared across all mobs of a type.

// navType classifies how a mob traverses the world. It mirrors the Bedrock
// minecraft:navigation.* component family and selects which media the mob may
// enter. (The A* neighbour model is shared; navType currently tunes only the
// malus defaults — fly/swim relax the land hazards.)
type navType int

const (
	navWalk  navType = iota // ground mobs (zombie, cow, …)
	navClimb                // spiders — walk + wall climb
	navHop                  // slime / magma cube — jump locomotion
	navFly                  // phantom / blaze — airborne
	navSwim                 // drowned — water-capable
)

// navProfile is a mob type's traversal capabilities + hazard penalties.
type navProfile struct {
	typ   navType
	malus map[string]float64 // block id → penalty (MalusBlocked = impassable)
}

// Block ids referenced by the malus tables. Centralised so a rename is a
// one-line change.
const (
	blkLava        = "minecraft:lava"
	blkWater       = "minecraft:water"
	blkPowderSnow  = "minecraft:powder_snow"
	blkFire        = "minecraft:fire"
	blkMagmaBlock  = "minecraft:magma_block"
	blkSweetBerry  = "minecraft:sweet_berry_bush"
	blkCactus      = "minecraft:cactus"
)

// landMalus is the default penalty table for ground mobs: lava and powder snow
// are lethal (impassable), open water and damaging blocks are strongly avoided
// but technically pathable so a cornered mob still has a route.
func landMalus() map[string]float64 {
	return map[string]float64{
		blkLava:       MalusBlocked,
		blkPowderSnow: MalusBlocked,
		blkWater:      8,
		blkFire:       MalusBlocked,
		blkMagmaBlock: 8,
		blkCactus:     MalusBlocked,
		blkSweetBerry: 4,
	}
}

// navProfileFor builds the immutable profile for a mob type. Special cases
// follow the spec's worked examples; everything else falls back to the land
// defaults derived from def.Movement.
func navProfileFor(def MobDef) navProfile {
	switch def.Type {
	case "minecraft:strider":
		// Lives on the lava sea: lava neutral, water blocked.
		return navProfile{typ: navWalk, malus: map[string]float64{
			blkLava: 0, blkWater: MalusBlocked, blkPowderSnow: MalusBlocked,
		}}
	case "minecraft:blaze":
		// Nether native: reluctant on lava (8), flies, water blocked.
		return navProfile{typ: navFly, malus: map[string]float64{
			blkLava: 8, blkWater: MalusBlocked,
		}}
	case "minecraft:magma_cube", "minecraft:wither_skeleton", "minecraft:zombified_piglin":
		// Fire-immune Nether mobs: tolerate lava at a cost, avoid water.
		return navProfile{typ: navTypeFor(def), malus: map[string]float64{
			blkLava: 8, blkWater: 8,
		}}
	case "minecraft:drowned":
		// Amphibious: water neutral, lava lethal.
		return navProfile{typ: navSwim, malus: map[string]float64{
			blkLava: MalusBlocked, blkPowderSnow: MalusBlocked,
		}}
	case "minecraft:enderman":
		// Water damages endermen — avoid entirely.
		mal := landMalus()
		mal[blkWater] = MalusBlocked
		return navProfile{typ: navWalk, malus: mal}
	}

	// Generic fall-through keyed on movement mode.
	switch def.Movement {
	case "fly", "hover":
		// Flyers ignore ground hazards but still won't dive into water.
		return navProfile{typ: navFly, malus: map[string]float64{blkWater: MalusBlocked}}
	case "climb":
		return navProfile{typ: navClimb, malus: landMalus()}
	case "hop":
		return navProfile{typ: navHop, malus: landMalus()}
	default:
		return navProfile{typ: navWalk, malus: landMalus()}
	}
}

// navTypeFor maps a movement mode to a navType (used when the special-case
// branch already fixed the malus but still wants the right locomotion class).
func navTypeFor(def MobDef) navType {
	switch def.Movement {
	case "fly", "hover":
		return navFly
	case "climb":
		return navClimb
	case "hop":
		return navHop
	default:
		return navWalk
	}
}

// cost returns the costFn for this profile bound to the world probe, or nil if
// the profile has no penalties or no probe is available (PathFind then runs
// uniform-cost). The probe reads the block at the mob's *feet* cell — hazards
// like lava/water are non-solid, so they live in the cell the mob would occupy.
func (p navProfile) cost(ctx *AIContext) costFn {
	if ctx.BlockNameAt == nil || len(p.malus) == 0 {
		return nil
	}
	malus := p.malus
	probe := ctx.BlockNameAt
	return func(x, y, z int) float64 {
		if m, ok := malus[probe(x, y, z)]; ok {
			return m
		}
		return 0
	}
}
