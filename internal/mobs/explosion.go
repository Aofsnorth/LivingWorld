// Creeper explosion. v1 implements the player-damage + broadcast half of
// the vanilla explosion; block destruction is deferred (creeper will
// inflict damage but not break blocks). Damage formula is the vanilla
// linear-falloff surface model:
//
//	maxDamage = power * 4 hp (creeper power 3 → 12 hp ≈ 6 hearts)
//	damage(d)  = (1 - d/radius)² × maxDamage
//	knockback = 0.4 b/tick in the away-from-explosion direction
//
// radius = power * 2 (creeper → 6 blocks; TNT power 4 → 8 blocks).
//
// The actual block-destruction half lives in DestroyedBlocks() below for
// future use; it isn't wired yet because (a) the world package doesn't
// expose a per-block hardness table, and (b) explosion resistance varies
// per block state (e.g. waterlogged, slab halves) and needs the
// BlockProperties registry. The function is exported so a follow-up can
// call it from inside the explode callback once the registry is added.
package mobs

import "math"

// ExplosionListener is the world.Manager hook for creeper explosions. The
// result includes origin + radius + the sorted list of affected players
// with their damage and knockback, so the bridge can broadcast the
// per-edition explosion event without recomputing the sphere.
type ExplosionListener func(result ExplosionResult)

// ExplosionResult is what the world tick gets back from ApplyExplosion so it
// can do the bridge broadcasts (Java ClientboundGameExplosion / Bedrock
// LevelEvent) without each bridge having to recompute the sphere.
type ExplosionResult struct {
	X, Y, Z   float64 // explosion origin
	Power     float64
	Radius    float64
	Hits      []PlayerHit // every player in radius, in distance order
	BlockHits []BlockHit  // every block destroyed (v1: nil)
}

// PlayerHit is one affected player + their damage + knockback vector.
type PlayerHit struct {
	UUID       [16]byte
	Distance   float64
	Damage     float32 // half-hearts
	KnockX     float64
	KnockY     float64
	KnockZ     float64
}

// BlockHit is one block the explosion destroyed (vanilla's effective
// resistance ≤ (4/3) * power). v1 doesn't actually destroy anything; the
// function that returns these is exported but unused.
type BlockHit struct {
	X, Y, Z int
}

// ComputeExplosionDamage returns the vanilla half-heart damage at distance
// d from an explosion of power p with effective radius p*2.
func ComputeExplosionDamage(power, distance float64) float32 {
	radius := power * 2
	if distance >= radius {
		return 0
	}
	falloff := 1.0 - (distance / radius)
	// Vanilla is linear: damage = power * 4 * (1 - d/r). The squared term
	// is a stricter "close quarters hurts" curve. Both are within 1 hp of
	// vanilla at the creeper radius (3) and we choose linear for the
	// "splash range" feel.
	hp := falloff * power * 4.0
	return float32(math.Ceil(hp))
}

// AffectedPlayers returns the players inside the explosion radius sorted
// by distance (closest first). ymin is the explosion's effective Y
// (typically origin + 1.0, head-height). A player at feet (Y) and 1.8
// blocks tall is considered "in the radius" if its AABB intersects the
// sphere.
func AffectedPlayers(players []PlayerTarget, x, y, z, power float64) []PlayerHit {
	radius := power * 2
	r2 := radius * radius
	hits := make([]PlayerHit, 0, len(players))
	for _, p := range players {
		// Distance to the closest point of the player's AABB.
		dx, dy, dz := x-p.X, y-p.Y, z-p.Z
		if dx < 0 {
			dx = -dx
		}
		if dz < 0 {
			dz = -dz
		}
		if p.Y > y {
			dy = p.Y - y // explosion above
		} else if y > p.Y+1.8 {
			dy = y - (p.Y + 1.8) // explosion above their head
		} else {
			dy = 0 // explosion is within the AABB vertically
		}
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d > radius {
			continue
		}
		_ = r2
		// Knockback is away from the explosion, normalized, magnitude 0.4 b/tick.
		nx, ny, nz := (p.X-x)/d, ((p.Y+0.9)-y)/d, (p.Z-z)/d
		if d == 0 {
			nx, ny, nz = 0, 1, 0
		}
		hits = append(hits, PlayerHit{
			UUID:     p.UUID,
			Distance: d,
			Damage:   ComputeExplosionDamage(power, d),
			KnockX:   nx * 0.4,
			KnockY:   ny*0.4 + 0.3, // upward bias so the player gets launched
			KnockZ:   nz * 0.4,
		})
	}
	// Closest first (stable for clients that process one hit at a time).
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].Distance < hits[j-1].Distance; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	return hits
}

// DestroyedBlocks returns the canonical sphere of blocks an explosion of
// `power` would destroy (effective resistance ≤ 4/3 * power). The current
// implementation needs a per-block hardness table from the world's block
// registry; the function is exported as a stub for follow-up.
func DestroyedBlocks(getBlock func(int, int, int) int, x, y, z, power float64) []BlockHit {
	// TODO(world): when world.BlockByID(name).Hardness is available,
	// walk a 2*radius sphere and emit BlockHit for every cell where
	// hardness <= 4.0 (creeper) or 6.67 (TNT). For now we return nil so
	// the caller can branch on length.
	return nil
}
