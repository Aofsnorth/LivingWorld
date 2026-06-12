// Package aicontext owns the cross-package read-only AI surface:
//
//   - AIContext : the per-tick input the AI tick receives (world probes +
//                 side-effect callbacks). It mirrors vanilla's
//                 net.minecraft.world.entity.ai.Brain inputs but is
//                 edition-neutral.
//   - PlayerTarget : the slice of a player's state the AI is allowed to read.
//   - SoundEmit / HitEffect : small struct types the world layer
//                 passes through the AI callbacks.
//
// The package is Mob-free: it owns the AIContext + PlayerTarget
// types and the small zero/find helpers. mobs re-exports the types
// as aliases so existing call sites (mobs.AIContext, mobs.SoundEmit)
// keep working.
package aicontext

import (
	"math"
	"math/rand"
)

// AIContext carries the read-only world probes and the side-effect
// callbacks the AI needs to do its job. It is built once per tick by
// the world layer and passed to aisystems.Tick.
type AIContext struct {
	RNG        *rand.Rand
	SolidAt    func(x, y, z int) bool
	SkyLightAt func(x, y, z int) uint8
	Players    func() []PlayerTarget

	OnMeleeAttack    func(targetUUID [16]byte, attackerID int64, damage float32)
	OnShootArrow     func(shooterID int64, x, y, z, yaw, pitch float64)
	OnExplode        func(attackerID int64, x, y, z float64, power float64)
	OnFireDamage     func(mobID int64, damage float32)
	OnSound          func(emits []SoundEmit)
	OnHitEffect      func(targetUUID [16]byte, attackerID int64, effect HitEffect)
	OnThrow          func(targetUUID [16]byte, attackerID int64, damage float32)
	OnShootProjectile func(ownerID int64, x, y, z, yaw, pitch float64, projectileType string)

	WaterAt      func(x, y, z int) bool
	IsDay        func() bool
	DoorAt       func(x, y, z int) bool
	OnBreakDoor  func(x, y, z int) bool
	Difficulty   string
	HeldItem     func(playerUUID [16]byte) string
	BlockNameAt  func(x, y, z int) string
}

// PlayerTarget is the read-only slice of a player's state the AI is
// allowed to read. The world layer builds this list from
// player.Player and passes it to Tick().
type PlayerTarget struct {
	UUID        [16]byte
	X, Y, Z     float64
	Sneaking    bool
	Invisible   bool
	WearingHead string
	WearingGold bool
	Gamemode    int
	LookYaw     float64
	LookPitch   float64
}

// SoundEmit is one mob sound the AI fans out to bridges each tick.
// The world layer's OnSound callback turns the slice into per-
// edition packets. Sound is the vanilla namespaced id
// ("minecraft:entity.zombie.ambient") — the world layer translates
// it into per-edition packet ids.
type SoundEmit struct {
	MobID  int64
	Sound  string
	Volume float32
	Pitch  float32
}

// HitEffect is the on-hit status effect a melee swing can apply
// (e.g. husk → hunger, cave spider → poison, wither skeleton → wither).
type HitEffect struct {
	Type    string // "hunger", "poison", "wither", "instant_damage", …
	Level   int    // effect amplifier (0 = I, 1 = II, …)
	Seconds int    // duration in ticks (the player layer converts to
	// seconds/20 internally for the effect-bag API)
}

// ZeroUUID returns the zero-valued [16]byte.
func ZeroUUID() [16]byte { return [16]byte{} }

// FindPlayer returns a pointer to the player with the given UUID in
// the slice, or nil.
func FindPlayer(players []PlayerTarget, uuid [16]byte) *PlayerTarget {
	for i := range players {
		if players[i].UUID == uuid {
			return &players[i]
		}
	}
	return nil
}

// HasLineOfSight walks the voxel line from (x0,y0,z0) to
// (x1,y1,z1) one cell at a time; returns false if any solid block
// is hit. Used by the enderman gaze sensor and the
// NearestAttackableTargetGoal detection scan.
//
// Lives in the context package (not systems) so the per-goal
// subpackages can import it without a goals/brain ↔ systems
// import cycle.
func HasLineOfSight(x0, y0, z0, x1, y1, z1 float64, solidAt func(x, y, z int) bool) bool {
	if solidAt == nil {
		return true
	}
	dx, dy, dz := x1-x0, y1-y0, z1-z0
	dist := math.Hypot(dx, math.Hypot(dy, dz))
	if dist == 0 {
		return true
	}
	steps := int(math.Ceil(dist * 2))
	if steps > 200 {
		steps = 200
	}
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x, y, z := x0+dx*t, y0+dy*t, z0+dz*t
		if solidAt(int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z))) {
			return false
		}
	}
	return true
}
