// Projectile store for skeleton arrows. M0.5: real arrow physics
// (gravity, segment block-hit, sub-stepping) and vanilla player AABB.
//
// Each arrow integrates `vy -= 0.05` per tick (vanilla gravity for
// projectiles, ~ -1.96 m/s² in block-units), then sub-steps the new
// velocity into 4 sub-segments and checks each segment against
// SolidAt + the AABB of every player. Sub-stepping is what makes
// arrows look "right" — without it, a 1.6 b/tick velocity skips past
// thin walls and into players standing on the far side of a doorway.
//
// Lifetime: 60 ticks (3 s). Vanilla arrows live 1200 ticks (60 s)
// before despawning, but 60 is plenty for the 15-block engagement
// range and avoids "stuck" arrows in the world after combat.
//
// The store is edition-neutral: each bridge subscribes to OnSpawn/
// OnDespawn/OnMove to render the arrow with its own packet.
package mobs

import (
	"math"
	"sync"
	"sync/atomic"
)

// Projectile is one in-flight arrow. OwnerID is the mob that fired it
// (skeleton's EntityID). Target is the player the skeleton was aiming
// at at fire time; the projectile checks against THIS player first
// (priority for vanilla-style "homing") then any other player in the
// segment.
//
// M1: Kind discriminates the projectile type so the bridges can
// route OnSpawn into the right entity spawn packet (Java
// ClientboundSpawnEntity with the right entity id / Bedrock
// AddActor with the right runtime id). Default is "arrow" for
// the legacy Spawn() path.
type Projectile struct {
	EntityID  int64
	OwnerID   int64
	Target    [16]byte // player UUID the skeleton was aiming at
	X, Y, Z   float64
	VX, VY, VZ float64  // velocity (b/tick)
	Lifetime  int       // ticks since spawn; despawns at MaxLifetime
	Kind      string    // M1: "arrow" | "arrow_slowness" | "arrow_poison" | "small_fireball" | "large_fireball" | "trident" | "potion"
	// M3: Yaw / Pitch captured at spawn for fireballs and potions.
	// The bridges use these to set the visible orientation of the
	// projectile entity. Arrows ignore them because the client
	// interpolates orientation from velocity; fireballs and
	// potions are slower and need an explicit yaw to face the
	// target. Stored as Minecraft degrees (0 = +Z south).
	Yaw   float64
	Pitch float64
}

// ProjectileKind string constants (M1). The bridges switch on these.
const (
	ProjectileArrow          = "arrow"
	ProjectileArrowSlowness  = "arrow_slowness"
	ProjectileArrowPoison    = "arrow_poison"
	ProjectileSmallFireball  = "small_fireball"
	ProjectileLargeFireball  = "large_fireball"
	ProjectileTrident        = "trident"
	ProjectilePotion         = "potion"
)

// ArrowMaxLifetime ticks before an arrow in flight is cleaned up.
// Vanilla is 1200 ticks (60 s); 60 is plenty for the 15-block
// engagement range. Raising this is safe (callers should not assume a
// cap).
const ArrowMaxLifetime = 60

// arrowGravity is the per-tick velocity decrement applied to VY. 0.05
// b/tick² maps to ~1.96 m/s² in the real world (g) and matches what
// drops.Store.TickPhysics uses for falling items.
const arrowGravity = 0.05

// arrowSpeed is the initial velocity magnitude in b/tick. Vanilla
// arrows fly at 1.6 b/tick when fired at 0° pitch; we use the same
// constant so the skeleton's bow damage (which scales with launch
// speed in vanilla) reads correctly.
const arrowSpeed = 1.6

// arrowSubSteps is how many sub-segments each per-tick motion is
// broken into. 4 is a good default: at 1.6 b/tick the sub-step is
// 0.4 b, smaller than the smallest hitbox (the player's 0.6 wide
// AABB) but large enough to keep the loop cheap. Vanilla uses 1
// sub-step on the client but does AABB expansion (0.3) to compensate.
const arrowSubSteps = 4

// ProjectileStore holds in-flight arrows.
type ProjectileStore struct {
	mu      sync.RWMutex
	projs   map[int64]*Projectile
	nextID  atomic.Int64
	onSpawn []func(Projectile)
	onMove  []func(Projectile)
	onDesp  []func(id int64)
}

// NewProjectileStore returns an empty store. Arrow entity ids start at
// 1<<24 to stay clear of mob ids (1<<22) and player/drop ids.
func NewProjectileStore() *ProjectileStore {
	s := &ProjectileStore{projs: make(map[int64]*Projectile)}
	s.nextID.Store(1 << 24)
	return s
}

// OnSpawn / OnMove / OnDespawn are the bridge subscription points.
func (s *ProjectileStore) OnSpawn(fn func(Projectile)) {
	s.mu.Lock()
	s.onSpawn = append(s.onSpawn, fn)
	s.mu.Unlock()
}
func (s *ProjectileStore) OnMove(fn func(Projectile)) {
	s.mu.Lock()
	s.onMove = append(s.onMove, fn)
	s.mu.Unlock()
}
func (s *ProjectileStore) OnDespawn(fn func(id int64)) {
	s.mu.Lock()
	s.onDesp = append(s.onDesp, fn)
	s.mu.Unlock()
}

// Spawn fires an arrow in the direction (yaw, pitch) from (x, y, z).
// The projectile store notifies bridges via OnSpawn.
//
//	yaw / pitch in degrees
//	x / y / z in world blocks (y is the eye/launch height, typically
//	1.6 above the skeleton's feet)
func (s *ProjectileStore) Spawn(ownerID int64, target [16]byte, x, y, z, yaw, pitch float64) Projectile {
	return s.SpawnKind(ownerID, target, x, y, z, yaw, pitch, ProjectileArrow)
}

// SpawnKind is Spawn with an explicit Kind (M1). The Kind is
// carried in the Projectile and is read by bridges on OnSpawn to
// pick the right entity-spawn packet (arrow vs tipped arrow vs
// fireball vs potion vs trident). v1: only the Kind field is
// carried; the per-kind entity id / packet dispatch is in the
// bridges (M1.6).
//
// Velocity tuning per kind:
//   - arrow*: 1.6 b/tick (vanilla arrow speed)
//   - small_fireball: 0.6 b/tick (vanilla blaze)
//   - large_fireball: 1.6 b/tick (vanilla ghast)
//   - trident: 1.6 b/tick (vanilla drowned)
//   - potion: 0.75 b/tick (vanilla witch, half of arrow)
func (s *ProjectileStore) SpawnKind(ownerID int64, target [16]byte, x, y, z, yaw, pitch float64, kind string) Projectile {
	yawR := yaw * math.Pi / 180
	pitchR := pitch * math.Pi / 180
	speed := arrowSpeed
	switch kind {
	case ProjectileSmallFireball:
		speed = 0.6
	case ProjectilePotion:
		speed = 0.75
	}
	p := Projectile{
		EntityID: s.nextID.Add(1),
		OwnerID:  ownerID,
		Target:   target,
		X: x, Y: y, Z: z,
		VX: -math.Sin(yawR) * math.Cos(pitchR) * speed,
		VY: -math.Sin(pitchR) * speed,
		VZ: math.Cos(yawR) * math.Cos(pitchR) * speed,
		Kind:   kind,
		Yaw:    yaw,
		Pitch:  pitch,
	}
	s.mu.Lock()
	s.projs[p.EntityID] = &p
	cbs := append([]func(Projectile){}, s.onSpawn...)
	s.mu.Unlock()
	for _, cb := range cbs {
		cb(p)
	}
	return p
}

// ProjectileTickContext is the world tick's input to ProjectileStore.Tick.
type ProjectileTickContext struct {
	SolidAt     func(x, y, z int) bool
	Players     func() []ProjectileTarget
	OnHitPlayer func(projectile Projectile, targetUUID [16]byte)
}

// ProjectileTarget is the slice of a player an arrow needs for hit
// detection: UUID + position. The "feet" y is used with the
// inflated AABB.
type ProjectileTarget struct {
	UUID    [16]byte
	X, Y, Z float64 // feet
}

// playerAABB is the hitbox the arrow checks against. Vanilla is
// 0.6 × 1.8; we inflate by 0.3 to match the arrow's tolerance so
// "near-misses" still hit. The X/Z are full-width 0.6 + 2*0.3 = 1.2;
// Y is the player's full height (feet to head) plus 0.3 either side.
const (
	playerAABBHalfWidth = 0.6 // (0.3 entity + 0.3 tolerance) on each side = 0.6 half-width
	playerAABBInflateY  = 0.3 // extra ±Y tolerance for arrow-trail hits
	playerHeight        = 1.8
)

// Tick integrates one game tick of arrow flight. Each arrow:
//  1. Decays VY by arrowGravity (vanilla g).
//  2. Sub-steps the new velocity into 4 sub-segments and checks
//     each against SolidAt + every player's inflated AABB.
//  3. Despawns on block hit, player hit, lifetime expiry, or OOB.
//
// Sub-stepping means the arrow's per-tick "1.6 b" motion becomes
// four 0.4 b steps. If any of those steps crosses a block boundary
// that SolidAt reports as solid, the arrow snaps to the surface
// and despawns. If a player's AABB overlaps the step, the arrow
// calls OnHitPlayer with the player's UUID and despawns.
func (s *ProjectileStore) Tick(ctx ProjectileTickContext) {
	type remove struct {
		id  int64
		hit bool
		who [16]byte
	}
	var toRemove []remove
	var moved []Projectile

	s.mu.Lock()
	for _, p := range s.projs {
		p.Lifetime++
		// M0.5: real gravity.
		p.VY -= arrowGravity

		dx := p.VX / arrowSubSteps
		dy := p.VY / arrowSubSteps
		dz := p.VZ / arrowSubSteps

		removed := false
		for step := 0; step < arrowSubSteps; step++ {
			nx := p.X + dx
			ny := p.Y + dy
			nz := p.Z + dz

			// Block hit: vanilla's hit cell is the head of the
			// segment (math.Floor of the destination). Inflating
			// by 0 here would make arrows vanish on top of slabs
			// / carpets, so we keep it tight.
			if ctx.SolidAt != nil &&
				ctx.SolidAt(int(math.Floor(nx)), int(math.Floor(ny)), int(math.Floor(nz))) {
				toRemove = append(toRemove, remove{id: p.EntityID})
				removed = true
				break
			}
			// Player hit: AABB cylinder, target first (vanilla
			// "homing").
			hit := false
			var hitWho [16]byte
			if ctx.Players != nil {
				players := ctx.Players()
				// Pass 1: the targeted player.
				for _, pl := range players {
					if pl.UUID != p.Target {
						continue
					}
					if segmentHitsAABB(p.X, p.Y, p.Z, nx, ny, nz, pl) {
						hit, hitWho = true, pl.UUID
						break
					}
				}
				// Pass 2: any other player in range.
				if !hit {
					for _, pl := range players {
						if pl.UUID == p.Target {
							continue
						}
						if segmentHitsAABB(p.X, p.Y, p.Z, nx, ny, nz, pl) {
							hit, hitWho = true, pl.UUID
							break
						}
					}
				}
			}
			if hit {
				toRemove = append(toRemove, remove{id: p.EntityID, hit: true, who: hitWho})
				removed = true
				break
			}
			// Lifetime / OOB.
			if p.Lifetime > ArrowMaxLifetime || ny < -64 || ny > 320 {
				toRemove = append(toRemove, remove{id: p.EntityID})
				removed = true
				break
			}
			p.X, p.Y, p.Z = nx, ny, nz
		}
		if !removed {
			moved = append(moved, *p)
		}
	}
	moveCbs := append([]func(Projectile){}, s.onMove...)
	s.mu.Unlock()

	for _, p := range moved {
		for _, cb := range moveCbs {
			cb(p)
		}
	}
	for _, r := range toRemove {
		s.Remove(r.id)
		if r.hit && ctx.OnHitPlayer != nil {
			ctx.OnHitPlayer(Projectile{EntityID: r.id}, r.who)
		}
	}
}

// segmentHitsAABB returns true when the segment from (x0,y0,z0) to
// (x1,y1,z1) crosses the AABB of `pl`. The AABB is centered on
// (pl.X, pl.Y + 0.9, pl.Z) with half-extents
// (playerAABBHalfWidth, playerHeight/2 + playerAABBInflateY,
// playerAABBHalfWidth).
//
// Vanilla uses Slab method. We use a simple AABB-vs-segment test:
// the segment hits if every axis has at least one endpoint on the
// "near" side AND the projected parameter t ∈ [0, 1] when the
// segment is clamped to the slab. Cheaper and good enough for a
// small step (0.4 b) and a 1.2×2.4 b AABB.
func segmentHitsAABB(x0, y0, z0, x1, y1, z1 float64, pl ProjectileTarget) bool {
	cx, cy, cz := pl.X, pl.Y+0.9, pl.Z
	ex, ey, ez := playerAABBHalfWidth, playerHeight/2+playerAABBInflateY, playerAABBHalfWidth

	// Inside AABB? Snap to segment.
	if math.Abs(x0-cx) <= ex && math.Abs(y0-cy) <= ey && math.Abs(z0-cz) <= ez {
		return true
	}

	// Slab method.
	dx, dy, dz := x1-x0, y1-y0, z1-z0
	tMin, tMax := 0.0, 1.0
	if !slab(dx, x0-cx, ex, &tMin, &tMax) {
		return false
	}
	if !slab(dy, y0-cy, ey, &tMin, &tMax) {
		return false
	}
	if !slab(dz, z0-cz, ez, &tMin, &tMax) {
		return false
	}
	return true
}

func slab(dOrigin, originDelta, halfExtent float64, tMin, tMax *float64) bool {
	if math.Abs(dOrigin) < 1e-9 {
		// Parallel: either inside the slab (hit) or outside (miss).
		if math.Abs(originDelta) > halfExtent {
			return false
		}
		return true
	}
	t1 := (-originDelta - halfExtent) / dOrigin
	t2 := (-originDelta + halfExtent) / dOrigin
	if t1 > t2 {
		t1, t2 = t2, t1
	}
	if t1 > *tMin {
		*tMin = t1
	}
	if t2 < *tMax {
		*tMax = t2
	}
	return *tMin <= *tMax
}

// Remove deletes an arrow by id and fires OnDespawn. Returns false if
// the arrow was already gone.
func (s *ProjectileStore) Remove(id int64) bool {
	s.mu.Lock()
	_, ok := s.projs[id]
	if ok {
		delete(s.projs, id)
	}
	cbs := append([]func(int64){}, s.onDesp...)
	s.mu.Unlock()
	if !ok {
		return false
	}
	for _, cb := range cbs {
		cb(id)
	}
	return true
}

// All returns a snapshot of every active projectile.
func (s *ProjectileStore) All() []Projectile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Projectile, 0, len(s.projs))
	for _, m := range s.projs {
		out = append(out, *m)
	}
	return out
}
