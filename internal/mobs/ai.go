package mobs

import (
	"math"
	"math/rand"
)

// PlayerTarget is the slice of a player's state the mob AI is allowed to
// read. The world layer builds this list from player.Player and passes it
// to Tick(); the mobs package never imports player (avoids the import cycle
// between world and player). Keeping the surface tiny makes it cheap to
// fill in and easy to mock in tests.
type PlayerTarget struct {
	UUID        [16]byte
	X, Y, Z     float64
	Sneaking    bool
	Invisible   bool
	WearingHead string // mob-type head id, e.g. "minecraft:zombie" (empty = none)
	// WearingGold is true when the player has at least one
	// piece of gold armor equipped. Piglins treat
	// gold-armored players as neutral (vanilla rule).
	WearingGold bool
	Gamemode    int    // 0 survival, 1 creative, 2 adventure, 3 spectator
}

// AIContext carries the read-only world and the side-effect callbacks the AI
// needs to do its job. It is built once per tick by the world layer and passed
// to Store.Tick. Anything the AI wants to mutate outside its own Mob struct
// has to go through one of these callbacks.
type AIContext struct {
	RNG        *rand.Rand
	SolidAt    func(x, y, z int) bool
	SkyLightAt func(x, y, z int) uint8

	// Players returns the live player list (built fresh each tick from
	// player.Manager, under the player package's own RLock).
	Players func() []PlayerTarget

	// OnMeleeAttack is called when a hostile mob lands a melee hit. Damage is
	// in half-hearts (1 hp = 0.5 hearts). The bridge handles the actual
	// player-side health/death packet.
	OnMeleeAttack func(targetUUID [16]byte, attackerID int64, damage float32)

	// OnShootArrow is called when a skeleton fires. yaw/pitch are in degrees.
	// The arrow entity + its physics + impact damage are managed by
	// projectile.go; the callback just spawns one.
	OnShootArrow func(shooterID int64, x, y, z, yaw, pitch float64)

	// OnExplode is called when a creeper's fuse hits zero. The world layer
	// handles block damage, particle/sound broadcast and player damage via
	// explosion.go.
	OnExplode func(attackerID int64, x, y, z float64, power float64)

	// OnFireDamage is called once per tick that a sunlight-sensitive
	// mob is exposed to direct sky light. Vanilla burns 1 HP per
	// second (so 0.05 HP/tick at 20 Hz); the world layer applies
	// the actual damage. The mob id is passed so the bridge can
	// broadcast the fire overlay / kill-credit metadata.
	OnFireDamage func(mobID int64, damage float32)

	// OnSound is the UX bridge for mob sounds. The world layer
	// pre-computes the sounds for this tick (see EmitSoundsFromSnapshot
	// in sound.go) and passes the list here; bridges fan out the
	// per-edition packet (Java ClientboundGameSoundEntity, Bedrock
	// LevelSoundEvent). Volume + pitch are [0, 1] floats; SoundID
	// is the namespaced id ("minecraft:entity.zombie.ambient").
	OnSound func(emits []SoundEmit)

	// M1: OnHitEffect is called when a melee swing applies a
	// status effect (e.g. husk → hunger, cave spider → poison,
	// wither skeleton → wither). The world layer translates the
	// HitEffect into the per-edition damage / effect packet (M5
	// adds the proper effect icons / particles; for M1 we apply
	// the underlying damage).
	OnHitEffect func(targetUUID [16]byte, attackerID int64, effect HitEffect)

	// M1: OnThrow is called when a mob (iron golem) picks up a
	// player and throws them. Vanilla: 3+ blocks up + 4 damage (the
	// throw-damage is applied on landing). The world layer applies
	// the upward velocity via players.Push.
	OnThrow func(targetUUID [16]byte, attackerID int64, damage float32)

	// M1: OnShootProjectile is the unified ranged-fire hook. The
	// projectileType string tells the bridge which kind of
	// projectile to spawn:
	//
	//   "arrow"               — vanilla arrow, default
	//   "arrow_slowness"      — tipped arrow with slowness (stray)
	//   "arrow_poison"        — tipped arrow with poison (bogged)
	//   "small_fireball"      — blaze: +1 explosion damage
	//   "large_fireball"      — ghast: splash 6 damage, 3 b radius
	//   "trident"             — drowned: thrown trident
	//   "potion"              — witch: throw a splash potion
	//
	// Bridges that haven't wired this yet fall back to OnShootArrow
	// (see shootTick).
	OnShootProjectile func(ownerID int64, x, y, z, yaw, pitch float64, projectileType string)

	// M1: WaterAt is used by WaterSensitive mobs (enderman) to
	// detect water contact. May be nil; the AI degrades
	// gracefully (no water damage). The world layer provides a
	// closure that calls block.WaterAt for the chunk the mob is
	// in.
	WaterAt func(x, y, z int) bool

	// M7.10: IsDay reports the world time of day. Used by
	// spider (aggressive at night only). May be nil; the AI
	// treats nil as "always night" so spiders stay hostile.
	IsDay func() bool

	// M7.10: DoorAt reports whether the block at (x, y, z)
	// is a door (any wood type). The BreaksDoors zombie
	// variants use this to recognise an obstacle they can
	// punch through. The world layer's closure checks the
	// block name (oak_door, spruce_door, …) so the mobs
	// package doesn't need to know the per-type block ids.
	// May be nil; the AI degrades to "treat as solid wall"
	// (the mob can't break it).
	DoorAt func(x, y, z int) bool

	// OnBreakDoor removes a door block from the world and publishes
	// the resulting block update. It returns true when a door was
	// actually removed. The AI calls this only after DoorAt reports
	// true, keeping the block-name logic in the world package.
	OnBreakDoor func(x, y, z int) bool

	// M7.10: Difficulty is the current world difficulty string
	// ("peaceful", "easy", "normal", "hard"). The AI gates
	// door-breaking by difficulty: vanilla zombies only break
	// wooden doors on Hard. The world layer provides this so
	// the AI doesn't need to know the difficulty constants.
	Difficulty string
}

// Tick runs one AI pass over every mob. It is the single entry point driven
// from world.Manager.runOneTick (Phase 4). Callbacks fire OUTSIDE the store
// lock, mirroring drops.Store.TickPhysics. SolidAt is required; the other
// fields may be nil (the AI degrades gracefully — no detection, no attack).
func (s *Store) Tick(ctx AIContext) {
	s.mu.Lock()
	moved := make([]Mob, 0, len(s.mobs))
	for _, m := range s.mobs {
		before := *m
		aiStep(m, ctx)
		if m.X != before.X || m.Y != before.Y || m.Z != before.Z ||
			m.Yaw != before.Yaw || m.HeadYaw != before.HeadYaw {
			moved = append(moved, *m)
		}
	}
	// UX: pre-compute sound emissions for the tick. The snapshot is
	// taken OUT of the moved slice so we get a single Mob per id
	// (even if a mob didn't move, it might still play an ambient).
	snapshot := make([]Mob, 0, len(s.mobs))
	for _, m := range s.mobs {
		snapshot = append(snapshot, *m)
	}
	cbs := append([]func(Mob){}, s.onMove...)
	s.mu.Unlock()

	for _, m := range moved {
		for _, cb := range cbs {
			cb(m)
		}
	}
	if ctx.OnSound != nil {
		ctx.OnSound(EmitSoundsFromSnapshot(snapshot, ctx.RNG))
	}
}

// aiStep runs one mob's AI for one tick. Heavy lifting is delegated to
// behaviour helpers (wanderAI, pursueAI, etc.) keyed on def + state.
func aiStep(m *Mob, ctx AIContext) {
	def := defFor(m.Type)

	// --- cooldowns & state timers ------------------------------------------
	if m.cooldownTicks > 0 {
		m.cooldownTicks--
	}
	if m.jumpCooldown > 0 {
		m.jumpCooldown--
	}
	if m.ambientCD > 0 {
		m.ambientCD--
	}
	if m.panicTicks > 0 {
		m.panicTicks--
		if m.panicTicks == 0 && !def.IsHostile {
			m.state = StateIdle
		}
	}
	// --- gravity -----------------------------------------------------------
	applyGravity(m, def, ctx)

	// --- M1: WaterSensitive (enderman) --------------------------------------
	// Vanilla: endermen take 1 HP/s of damage while in contact
	// with water (or rain, with no block above). We approximate
	// with a single cell check at the mob's feet (the enderman
	// is 2.9 b tall so the head cell check would be expensive).
	// We rely on the world layer's WaterAt to detect water —
	// it returns true for any waterlogged block. Rain detection
	// is dropped for v1.
	if def.WaterSensitive && ctx.WaterAt != nil {
		feetX, feetY, feetZ := int(math.Floor(m.X)), int(math.Floor(m.Y)), int(math.Floor(m.Z))
		if ctx.WaterAt(feetX, feetY, feetZ) {
			if ctx.OnFireDamage != nil {
				// Re-use the OnFireDamage hook: it's
				// 1 HP/s of damage, and the mob's HP
				// will tick down. The bridge broadcasts
				// the death on zero HP.
				ctx.OnFireDamage(m.EntityID, 0.05)
			}
		}
	}

	// --- M0.8 despawn logic ------------------------------------------------
	// Vanilla rules (simplified for v1):
	//  1. Hostile mobs >128 blocks from ANY player despawn instantly.
	//  2. Hostile mobs with no player within 128 blocks for 30s (600 ticks)
	//     despawn gradually. This applies to all hostile states.
	//  3. Hostile mobs with an active target (chasing player) get a
	//     temporary 'persistence' boost — they won't despawn while
	//     actively chasing, preventing frustrating "mobs vanishing
	//     around corners" scenarios.
	//  4. Passive/neutral mobs and PersistenceRequired mobs never despawn.
	players := ctx.Players()
	if def.IsHostile && !m.PersistenceRequired {
		despawnDistSq := 128.0 * 128.0 // 128 blocks squared
		timerTicks := 600               // 30 seconds at 20 Hz

		// Find the nearest player distance squared.
		nearestDistSq := math.MaxFloat64
		for _, p := range players {
			dx, dz := p.X-m.X, p.Z-m.Z
			distSq := dx*dx + dz*dz
			if distSq < nearestDistSq {
				nearestDistSq = distSq
			}
		}

		// Rule 1: Instant despawn if completely out of range (>128 b).
		if nearestDistSq > despawnDistSq {
			m.Despawn = true
		} else if nearestDistSq <= despawnDistSq {
			// Rule 2 & 3: Within 128 blocks but not close enough for the
			// 30s timer to reset. Reset when <=128 blocks, increment when
			// no player is close. If currently chasing a target, we
			// pause the timer (simulate "engaged" persistence).
			hasTarget := m.target != zero16()
			if hasTarget {
				// Reset timer while engaged to prevent despawning mid-chase.
				m.noPlayerTicks = 0
			} else {
				m.noPlayerTicks++
				if m.noPlayerTicks >= timerTicks {
					m.Despawn = true
				}
			}
		}
	}

	// --- sun check (zombie / skeleton in daylight) -------------------------
	// M0.6: hostile undead mobs burn in direct sunlight. Vanilla
	// applies 1 HP/s of fire damage (so 0.05 HP/tick at 20 Hz) and
	// sets the mob on fire (visual). We do the same here: if the
	// mob is in the sun, call OnFireDamage with the per-tick damage
	// and let the world layer apply it. The mob's HP is tracked
	// outside the AI (in the world layer / a future combat store),
	// so we just notify.
	if def.BurnsInDaylight && sunCheck(m, ctx) {
		if ctx.OnFireDamage != nil {
			ctx.OnFireDamage(m.EntityID, 0.05)
		}
	}

	// --- detection (hostile only) -----------------------------------------
	if def.IsHostile {
		if tgt, ok := pickTarget(m, def, players, ctx); ok {
			m.target = tgt
			if m.state != StateFuse && m.state != StateMelee {
				m.state = StatePursue
			}
		} else {
			// Lost the target — revert to wander. Creeper in fuse keeps
			// ticking down (it's already committed).
			if m.state != StateFuse {
				m.target = zero16()
				m.state = StateIdle
			}
		}
	}

	// M0.3: tick the path-replan counter. Replan every 20 ticks or when
	// the target moves >2 cells. Tick count is 0 when no path yet.
	if m.pathTick > 0 {
		m.pathTick--
	}

	// --- behaviour dispatch -----------------------------------------------
	switch m.state {
	case StateIdle, StatePursue, StateFlee:
		moveMob(m, def, players, ctx)
		lookAtNearestPlayer(m, def, players, 6.0)
	case StateMelee:
		// Stand in range; swing on cooldown; lose interest if target runs.
		meleeTick(m, def, players, ctx)
		faceTarget(m, players)
	case StateShoot:
		shootTick(m, def, players, ctx)
		faceTarget(m, players)
	case StateFuse:
		fuseTick(m, def, players, ctx)
	}
}

// zero16 returns the zero-valued [16]byte. Helper for the "no target" case.
func zero16() [16]byte { return [16]byte{} }

// applyGravity integrates one tick of vertical motion. The model is the
// standard `vy -= 0.08; m.Y += vy` integrator, with a hard ground-snap when
// the cell below the feet is solid. Jumps inject `vy = 0.42` (the vanilla
// jump-strength unit, applied by mob's `jump_strength` attribute); the
// 0.08 gravity is also the canonical constant — it produces a 1.25-block
// max jump height for 0.42 impulse. Head-block is checked: if the cell at
// `feet.y + 1` is solid we kill vy mid-ascent so the mob doesn't bury
// itself.
//
// Landing rule: only snap to floor when `vy <= 0` and the destination cell
// is solid (true landing, not a mid-jump cell we passed through). This
// stops the floor-snap from cancelling a jump on the first tick.
//
// M1: Movement-mode dispatch.
//   "fly"   — Phantom, Blaze, Ghast. No gravity, no ground-snap. The
//             mob handles its own vertical motion in moveMob (sine
//             wave for ghast, hover-into-attack for blaze/phantom).
//   "hover" — Ghast (gentle bob). Half-gravity + a sine wave; we
//             integrate vy but with a 0.5x multiplier so the mob
//             slowly returns to a "rest altitude" set on spawn.
//   "climb" — Spider. Wall-walk: if the cell at sideY+1 is solid,
//             flip to climbing (we set m.vy = 0.2 every tick the
//             mob is against a wall). The actual climb motion is
//             handled in moveMob.
//   "hop"   — Slime/MagmaCube. We use the same gravity model but
//             jumpMob is forced every few ticks in moveMob; apply
//             gravity normally.
//   "walk"  — default, full gravity.
func applyGravity(m *Mob, def MobDef, ctx AIContext) {
	// Flying mobs: do not integrate gravity. The mob's own
	// vertical motion (set in moveMob / state transitions) is
	// authoritative.
	if def.Movement == "fly" {
		// Drain any residual vy so the mob doesn't keep drifting
		// up after a jump, but allow it to be set by moveMob.
		return
	}
	feetX, feetY, feetZ := int(math.Floor(m.X)), int(math.Floor(m.Y)), int(math.Floor(m.Z))

	// Falling or jumping: integrate one step.
	prevY := m.Y
	m.Y += m.vy
	if def.Movement == "hover" {
		// Ghast: half-gravity. Keeps the bob gentle.
		m.vy -= 0.04
	} else {
		m.vy -= 0.08
	}

	// Head-block check: if the cell at the new head is solid, stop the
	// upward motion (the mob bonked the ceiling).
	newHeadY := int(math.Floor(m.Y)) + 1
	if m.vy > 0 && newHeadY > feetY+1 && ctx.SolidAt(feetX, newHeadY, feetZ) {
		m.vy = 0
		m.Y = prevY
	}

	// Ground-snap: only when falling (vy <= 0) and the destination is solid.
	if m.vy <= 0 {
		nx, ny, nz := feetX, int(math.Floor(m.Y))-1, feetZ
		if ctx.SolidAt(nx, ny, nz) {
			m.Y = float64(ny+1) // sit on top of the floor block
			m.vy = 0
			m.OnGround = true
			return
		}
	}

	// Below the world (vanilla: void damage) — clamp to 0 with a sentinel.
	// The world tick will pick up the low Y and damage the mob.
	if m.Y < float64(minY) {
		m.Y = float64(minY)
		m.vy = 0
		m.OnGround = true
	}
	if m.vy != 0 {
		m.OnGround = false
	}
}

// minY mirrors world.MinWorldHeight without forcing a cross-package import
// on the mobs package. Vanilla mob despawn on Y = -64 (the bottom of the
// world). Kept as a private constant so a future M1 change to
// configurable per-dimension bounds has one place to thread through.
const minY = -64

// jumpMob is the vanilla jump-strength injection. Callers (path.go, melee
// leap) decide *when* to jump; this is the *how* (one-tick impulse, no
// inertia, lands the same way every time).
func jumpMob(m *Mob) {
	if m.OnGround && m.jumpCooldown == 0 {
		m.vy = 0.42
		m.jumpCooldown = 6
	}
}

// sunCheck returns true if the mob's head is in direct sunlight. Vanilla
// uses a 1×1 column above the mob (must all be transparent and have sky
// light >= 12 with no rain). We approximate with a single sky-light read at
// the mob's Y; close enough for burn-tick purposes.
func sunCheck(m *Mob, ctx AIContext) bool {
	if ctx.SkyLightAt == nil {
		return false
	}
	x, y, z := int(math.Floor(m.X)), int(math.Floor(m.Y))+1, int(math.Floor(m.Z))
	return ctx.SkyLightAt(x, y, z) >= 12
}

// pickTarget returns the UUID of the best target for this mob, or false if
// no player is in range + line-of-sight. Implements the vanilla detection
// formula:
//
//	range = follow_range * sneak_mod * head_mod * invisible_mod
//
// All multipliers are 1 by default. We re-use the same idea as the wiki:
//
//	sneaking   → 0.8
//	head       → 0.5 (zombie/skeleton/creeper head only)
//	invisible  → 0.07
//	spectator  → 0
//
// M7.10 added per-mob gates:
//   - AggressiveAtNight: spider. The mob returns "no target"
//     during the day, so the AI falls back to StateIdle (the
//     spider walks around but doesn't chase).
//   - AggressiveUnlessGold: piglin. Gold-armored players are
//     skipped entirely (vanilla treats them as neutral).
//   - Enderman gaze (deferred to M7.x): vanilla enderman
//     only attack players who have looked at them within the
//     last 5 s. v1 doesn't track per-player gaze timers, so
//     endermen fall back to the standard hostile detection
//     (hostile to nearest player in range).
func pickTarget(m *Mob, def MobDef, players []PlayerTarget, ctx AIContext) ([16]byte, bool) {
	if len(players) == 0 || ctx.SolidAt == nil {
		return zero16(), false
	}
	// Spider daytime gate: don't pick any target during the
	// day. The mob falls back to wander; retaliates only
	// after being hit (hurtBy path).
	if def.AggressiveAtNight && ctx.IsDay != nil && ctx.IsDay() {
		return zero16(), false
	}
	bestDist := math.MaxFloat64
	var best [16]byte
	mx, my, mz := m.X, m.Y+1.0, m.Z // eye position
	for _, p := range players {
		if p.Gamemode == 1 || p.Gamemode == 3 { // creative + spectator are ignored
			continue
		}
		// Piglin gold armor gate: gold-armored players are
		// neutral; skip them entirely.
		if def.AggressiveUnlessGold && p.WearingGold {
			continue
		}
		// Detection-range modifiers.
		mod := 1.0
		if p.Sneaking {
			mod *= 0.8
		}
		if p.Invisible {
			mod *= 0.07
		}
		if p.WearingHead != "" {
			switch p.WearingHead {
			case "minecraft:zombie", "minecraft:skeleton", "minecraft:creeper":
				mod *= 0.5
			}
		}
		dist := math.Hypot(p.X-mx, math.Hypot(p.Y-my, p.Z-mz))
		if dist > def.FollowRange*mod {
			continue
		}
		// Line-of-sight: simple 3D DDA from mob-eye to player-eye. Vanilla
		// uses 8 ray samples; for v1 a single straight-line DDA is enough.
		if !hasLineOfSight(mx, my, mz, p.X, p.Y+1.0, p.Z, ctx.SolidAt) {
			continue
		}
		if dist < bestDist {
			bestDist = dist
			best = p.UUID
		}
	}
	return best, best != zero16()
}

// hasLineOfSight walks the voxel line from (x0,y0,z0) to (x1,y1,z1) stepping
// one cell at a time. Returns false if any non-air block is hit. The eye
// positions used by pickTarget are 1.6m above the feet, so the ray starts
// at +1.6 of the feet and aims at the player's head.
func hasLineOfSight(x0, y0, z0, x1, y1, z1 float64, solidAt func(x, y, z int) bool) bool {
	dx, dy, dz := x1-x0, y1-y0, z1-z0
	dist := math.Hypot(dx, math.Hypot(dy, dz))
	if dist == 0 {
		return true
	}
	steps := int(math.Ceil(dist * 2)) // 2 samples per block (mid-cell)
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

// moveMob is the per-tick integrator. It picks a direction (toward target if
// pursuing, away from attacker if panicking, otherwise along Yaw) and tries
// to step forward; if the step is blocked it tries to jump (hostile +
// pursue + cooldown ok); if still blocked it turns.
//
// M0.3: when StatePursue, the direction comes from the A* path rather
// than the raw line to the target. The path is replanned every 20 ticks
// or when the target moves >2 cells from the path's last cell.
//
// M1: Movement-mode dispatch. "fly" skips the horizontal step (the
// mob's own state drives motion), "hover" is identical to "walk"
// but uses half-gravity (handled in applyGravity), "climb" adds a
// wall-walk check before the step, "hop" forces a jump every few
// ticks so the slime/cube actually leaps.
func moveMob(m *Mob, def MobDef, players []PlayerTarget, ctx AIContext) {
	speed := def.WanderSpeed
	target := zero16()

	// Pick a direction.
	var dx, dz float64
	switch m.state {
	case StatePursue:
		// Find the target player; if missing, fall back to wander.
		var tgt *PlayerTarget
		for i, p := range players {
			if p.UUID == m.target {
				tgt = &players[i]
				break
			}
		}
		if tgt == nil {
			m.state = StateIdle
			m.path = nil
			m.pathIdx = 0
			return
		}
		speed = def.ChaseSpeed
		target = m.target
		// M0.3: build / refresh path.
		mobCell := PathNode{X: int(math.Floor(m.X)), Y: int(math.Floor(m.Y)), Z: int(math.Floor(m.Z))}
		goalCell := PathNode{X: int(math.Floor(tgt.X)), Y: int(math.Floor(tgt.Y)), Z: int(math.Floor(tgt.Z))}
		if !m.hasPath() || m.pathGoalStale(goalCell) || m.pathTick == 0 {
			m.replanPath(mobCell, goalCell, ctx)
		}
		// Walk toward the next waypoint.
		mx, mz := int(math.Floor(m.X)), int(math.Floor(m.Z))
		if ptdx, ptdz, pty, ok := m.pathDirection(mx, mz); ok {
			dx, dz = ptdx, ptdz
			// If the next waypoint is elevated and we're grounded, jump.
			if pty > int(math.Floor(m.Y)) && m.OnGround && m.jumpCooldown == 0 {
				jumpMob(m)
			}
		} else {
			// Path exhausted — fall back to direct-line chase.
			dx, dz = tgt.X-m.X, tgt.Z-m.Z
		}
	case StateFlee:
		// Run opposite of hurtBy for panicTicks.
		for _, p := range players {
			if p.UUID == m.hurtBy {
				dx, dz = m.X-p.X, m.Z-p.Z
				break
			}
		}
		if dx == 0 && dz == 0 {
			dx, dz = math.Sin(m.Yaw*math.Pi/180), math.Cos(m.Yaw*math.Pi/180)
		}
		// Flee ~1.5× wander speed.
		speed = def.WanderSpeed * 1.5
	default:
		// Idle wander: head along Yaw for `walkTicks` ticks, then idle.
		if m.walkTicks <= 0 && ctx.RNG.Float64() < 0.0125 {
			m.walkTicks = 80 + ctx.RNG.Intn(160) // 4-12s
			m.Yaw = ctx.RNG.Float64()*360 - 180
		}
		if m.walkTicks > 0 {
			dx, dz = -math.Sin(m.Yaw*math.Pi/180), math.Cos(m.Yaw*math.Pi/180)
			m.walkTicks--
		} else {
			// M7.10: idle head drift. Vanilla mobs
			// standing still slowly turn their head in
			// small increments. We nudge HeadYaw by
			// ±0.5°/tick on a 30-tick cadence; the
			// delta is small enough to look like an
			// "ear twitch" rather than a panicked
			// turn-around. Yaw stays put so the body
			// doesn't reorient. The lookAt helper
			// overwrites HeadYaw when a player is in
			// range, so a player passing by wins over
			// the drift.
			if m.ambientCD%30 == 0 {
				m.HeadYaw += (ctx.RNG.Float64() - 0.5) * 30.0
			}
			return // idle
		}
	}

	// M1: fly / hover / hop. The slime's hop motion is the same
	// step + jump, but we force a jump on cooldown timer to match
	// vanilla's bouncing cadence (size 1 = 6-tick jump cooldown,
	// size 2 = 10, size 3 = 14, size 4 = 20).
	if def.Movement == "hop" {
		// Slime/MagmaCube: forced jump on a cadence. Size scaling
		// matches the larger cubes jumping less often.
		jumpEvery := 6
		if m.Size == 2 {
			jumpEvery = 10
		} else if m.Size == 3 {
			jumpEvery = 14
		} else if m.Size >= 4 {
			jumpEvery = 20
		}
		if m.OnGround && m.jumpCooldown == 0 {
			// The actual cadence is jittered slightly with
			// m.jumpCooldown so we don't tick a perfect 6
			// every time. Set a hop-cadence anchor.
			if m.ambientCD <= 0 {
				m.ambientCD = jumpEvery
				jumpMob(m)
			}
		}
	}

	// M1: fly. The mob's own state drives motion; we still need to
	// run the StatePursue pathing for chase but we skip the
	// horizontal step (applyGravity already short-circuited).
	if def.Movement == "fly" {
		// Phantom dive / Blaze hover: handled by their own
		// transition logic (see transition()). For "fly" the
		// mob only does a horizontal step; vertical is set
		// by the transition.
	} else {
		// Normalize and scale to the per-tick delta.
		dl := math.Hypot(dx, dz)
		if dl == 0 {
			return
		}
		// Vanilla: zombie attribute 0.23 → 2.5 m/s walk = 0.125 b/tick.
		// 0.23 * 0.5 = 0.115 b/tick = 2.3 m/s. The 0.5 multiplier is the
		// terminal-velocity approximation of vanilla's
		//   deltaV += attribute * 0.1
		//   deltaV *= 0.92 friction (land) or 0.91 / 0.98 / 0.91 (general)
		// limit, simplified to attribute * 0.5. Player physics is in
		// player/manager.go and uses the 43.17 m/s-per-attribute-unit constant
		// because the player's movement model is different (food, sprint, etc).
		step := speed * 0.5

		// M7.10: water slow. Vanilla: hostile mobs move at
		// 0.5× their on-land speed in water; the mob also
		// sinks unless it has a WaterSensitive flag (only
		// enderman takes damage). We slow the step here.
		// WaterAt is provided by the world layer; nil-safe
		// for tests.
		if ctx.WaterAt != nil {
			feetX, feetY, feetZ := int(math.Floor(m.X)), int(math.Floor(m.Y)), int(math.Floor(m.Z))
			if ctx.WaterAt(feetX, feetY, feetZ) {
				step *= 0.5
			}
		}

		// M7.10: baby zombie 1.5× speed (vanilla).
		if m.Baby {
			step *= 1.5
		}

		nx := m.X + (dx/dl)*step
		nz := m.Z + (dz/dl)*step
		// Turn to face the move direction (only for pursuing/fleeing; wander
		// already turned when it picked a new Yaw).
		if m.state != StateIdle {
			m.Yaw = math.Atan2(-dx, dz) * 180 / math.Pi
		}

		// M1: climb. Spider wall-walks: if a horizontal step is
		// blocked by a wall, climb up by injecting a small vy.
		if def.Movement == "climb" {
			climbX, climbZ := int(math.Floor(nx)), int(math.Floor(nz))
			feetY := int(math.Floor(m.Y))
			if ctx.SolidAt(climbX, feetY, climbZ) || ctx.SolidAt(climbX, feetY+1, climbZ) {
				// Wall in the way — climb up.
				if m.OnGround && m.jumpCooldown == 0 {
					m.vy = 0.32 // lower than walk jump (0.42)
					m.OnGround = false
				}
			}
		}

		// Try to step; if blocked, try to break a door (zombie variants),
		// then jump, then finally count as stuck.
		blockX, blockY, blockZ := int(math.Floor(nx)), int(math.Floor(m.Y)), int(math.Floor(nz))
		if ctx.SolidAt(blockX, blockY, blockZ) {
			// M7.10: zombies break wooden doors instead of grinding
			// forever against them. Vanilla takes multiple crack stages;
			// v1 removes the door when the zombie reaches it. The world
			// callback does the block update / publish, so AI stays block-id
			// agnostic. Door breaking is gated to Hard difficulty only,
			// matching vanilla behavior.
			difficultyAllowsBreak := ctx.Difficulty == "hard"
			if def.BreaksDoors && difficultyAllowsBreak && ctx.DoorAt != nil && ctx.OnBreakDoor != nil && ctx.DoorAt(blockX, blockY, blockZ) {
				if ctx.OnBreakDoor(blockX, blockY, blockZ) {
					m.stuckTicks = 0
					m.X, m.Z = nx, nz
					return
				}
			}
			if m.jumpCooldown == 0 && m.OnGround {
				// Use the real jump integrator (M0.2): vy = 0.42, the gravity
				// step in applyGravity will damp it. The old code did
				// `m.Y += 0.42` which was instantly cancelled by the next
				// tick's ground-snap.
				jumpMob(m)
			} else {
				// M7.10: stuck detection. Vanilla mobs
				// turn 90° after grinding on the same wall
				// for ~3 ticks. We count consecutive blocked
				// steps; on the 3rd we turn and reset, so the
				// mob visibly backs off rather than marching
				// in place.
				m.stuckTicks++
				if m.stuckTicks >= 3 {
					m.Yaw += 90
					m.stuckTicks = 0
				}
			}
			return
		}
		// Stepped cleanly — reset the stuck counter.
		m.stuckTicks = 0
		m.X, m.Z = nx, nz
	}

	// After moving, hostile mobs decide if they should be in a combat
	// state this tick. Skeleton engages at 15+ blocks, zombie/creeper
	// in melee range.
	if m.state == StatePursue && def.IsHostile {
		dist := math.Hypot(dx, dz)
		if def.HasRanged && dist <= 15 && dist >= 4 {
			m.state = StateShoot
			// M0.4: bow draw for 20 ticks before the arrow fires.
			// During the draw the mob stands still and aims; vanilla
			// also lowers the draw time when re-entering quickly but
			// we keep it constant for v1.
			m.drawTicks = 20
		} else if !def.HasRanged && dist <= def.AttackRange {
			m.state = StateMelee
		} else if def.HasExplosion && dist <= def.ExplosionRadius {
			m.state = StateFuse
			m.wasInRange = false // reset fuse trigger
		}
	}
	_ = target

	// M7: consume any pending knockback velocity. The mob's
	// HurtDirect stored a one-shot impulse in KnockbackVX/VY/VZ
	// when a player melee swing landed. We apply it on the
	// same tick the hit happened (next tick in practice —
	// aiStep runs at most once per world tick), zero the
	// components, and let applyGravity integrate the vertical
	// bump. The horizontal impulse integrates linearly so the
	// mob visibly slides a few blocks before friction / path
	// decisions resume. The hit flash and hurtBy are already
	// stored; this just adds the visible "stagger".
	if m.KnockbackVX != 0 || m.KnockbackVZ != 0 || m.KnockbackVY != 0 {
		m.X += m.KnockbackVX
		m.Z += m.KnockbackVZ
		m.vy += m.KnockbackVY
		m.KnockbackVX, m.KnockbackVY, m.KnockbackVZ = 0, 0, 0
		// Force a position update so the bridge sees the
		// stagger even on a StateIdle / no-path tick. The
		// OnMove broadcast happens on the caller's next
		// OnMove iteration; the integrator already runs
		// the mob's normal move-and-broadcast loop.
	}
}

// meleeTick fires the swing when the cooldown is up. If the player ran out
// of range we revert to StatePursue so the mob chases again.
//
// M1: applies the mob's OnHit effect (e.g. husk → hunger, wither
// skeleton → wither, cave spider → poison). The world layer
// translates the effect into the per-edition damage packet; the
// AI just signals what should happen.
func meleeTick(m *Mob, def MobDef, players []PlayerTarget, ctx AIContext) {
	var target *PlayerTarget
	for i, p := range players {
		if p.UUID == m.target {
			target = &players[i]
			break
		}
	}
	if target == nil {
		m.target = zero16()
		m.state = StateIdle
		return
	}
	dx, _, dz := target.X-m.X, 0.0, target.Z-m.Z
	dist := math.Hypot(dx, dz)
	if dist > def.AttackRange {
		m.state = StatePursue
		return
	}
	if m.cooldownTicks > 0 {
		return
	}
	// Iron golem: instead of a normal swing, it THROWS the player
	// up into the air. Def.ThrowDamage is the throw damage; the
	// world layer applies the upward velocity.
	if def.ThrowDamage > 0 && ctx.OnThrow != nil {
		ctx.OnThrow(target.UUID, m.EntityID, def.ThrowDamage)
	} else if ctx.OnMeleeAttack != nil {
		ctx.OnMeleeAttack(target.UUID, m.EntityID, float32(def.AttackDamage))
	}
	// M1: apply the mob's on-hit effect.
	if def.OnHit.Type != "" && ctx.OnHitEffect != nil {
		ctx.OnHitEffect(target.UUID, m.EntityID, def.OnHit)
	}
	m.cooldownTicks = def.AttackCooldown
}

// shootTick is the ranged-attack loop. M0.4: counts down
// `drawTicks` while the mob aims, then fires. M1: extended to
// support non-arrow projectiles (blaze small fireball, ghast
// large fireball, witch potion, drowned trident) by routing
// through OnShootProjectile with the mob's RangedProjectile type.
//
// M1: per-mob range check is now configurable (def.RangedWarmupTicks,
// 15-block skeleton range, 40-block ghast range) instead of
// hard-coded.
func shootTick(m *Mob, def MobDef, players []PlayerTarget, ctx AIContext) {
	// Per-mob engagement range. Skeleton = 15, ghast = 40, witch = 15,
	// blaze = 16, drowned = 15, stray = 15, bogged = 15. We
	// approximate by using the squared attack range (vanilla uses
	// horizontal distance; 15 b² = 225, 40 b² = 1600).
	maxRange := math.Sqrt(def.AttackRange) // attack range is squared
	minRange := 4.0
	if def.Movement == "fly" || def.Movement == "hover" {
		// Flying mobs keep distance — lower min range so they
		// don't divebomb their target.
		minRange = 6.0
	}
	for _, p := range players {
		if p.UUID != m.target {
			continue
		}
		dx, dz := p.X-m.X, p.Z-m.Z
		dist := math.Hypot(dx, dz)
		if dist > maxRange || dist < minRange {
			m.state = StatePursue
			m.drawTicks = 0
			return
		}
		dy := (p.Y + 1.0) - (m.Y + 1.6)
		pitch := math.Atan2(dy+0.2*dist, dist) * 180 / math.Pi
		yaw := math.Atan2(-dx, dz) * 180 / math.Pi
		m.HeadYaw = yaw
		if m.drawTicks > 0 {
			m.drawTicks--
			return
		}
		// Fire!
		projectileType := def.RangedProjectile
		if projectileType == "" {
			projectileType = "arrow" // default for vanilla skeleton
		}
		if ctx.OnShootProjectile != nil {
			ctx.OnShootProjectile(m.EntityID, m.X, m.Y+1.6, m.Z, yaw, pitch, projectileType)
		} else if ctx.OnShootArrow != nil {
			// Fallback to the old OnShootArrow if the bridge
			// hasn't wired the new hook yet.
			ctx.OnShootArrow(m.EntityID, m.X, m.Y+1.6, m.Z, yaw, pitch)
		}
		m.Yaw, m.HeadYaw = yaw, yaw
		m.cooldownTicks = def.AttackCooldown
		m.state = StatePursue
		return
	}
	// Target gone.
	m.target = zero16()
	m.state = StateIdle
}

// fuseTick is the creeper's "hissing" loop. M7.10: the fuse
// keeps ticking down even if the target leaves range. The
// creeper stands in place, swells, and explodes where it
// stood (vanilla behaviour: the creeper's fuse is committed
// once it lights — running away doesn't cancel the boom,
// only a charged-creeper-vs-shorter-fuse difference). The
// "fuse resets" is kept: a fresh in-range target re-arms the
// fuse to the full duration (def.FuseTicks), but a target
// that leaves while the fuse is running lets the timer
// continue to count down.
//
// M7.10: creeper also stops moving while hissing (vanilla
// freezes the creeper the moment it starts the fuse). It
// faces the last known target direction (or its current
// Yaw if no target is visible).
func fuseTick(m *Mob, def MobDef, players []PlayerTarget, ctx AIContext) {
	// Is the original target still visible?
	targetVisible := false
	for _, p := range players {
		if p.UUID == m.target {
			dist := math.Hypot(p.X-m.X, p.Z-m.Z)
			if dist < def.ExplosionRadius {
				targetVisible = true
			}
			break
		}
	}
	// Re-arm the fuse if a fresh target entered range
	// (m.wasInRange = false → first frame of a new fuse).
	if targetVisible && !m.wasInRange {
		m.fuseTicks = def.FuseTicks
	}
	m.wasInRange = targetVisible
	// Tick the fuse down regardless of target visibility —
	// the creeper is committed.
	m.fuseTicks--
	// Face the last known target (or current Yaw if
	// missing) — vanilla creeper doesn't move while
	// hissing.
	if m.target != zero16() {
		for _, p := range players {
			if p.UUID == m.target {
				dx, dz := p.X-m.X, p.Z-m.Z
				if dx != 0 || dz != 0 {
					m.Yaw = math.Atan2(-dx, dz) * 180 / math.Pi
				}
				break
			}
		}
	}
	if m.fuseTicks <= 0 {
		if ctx.OnExplode != nil {
			ctx.OnExplode(m.EntityID, m.X, m.Y+1.0, m.Z, float64(def.ExplosionPower))
		}
		m.Despawn = true // world tick will collect and Remove()
	}
}

// lookAtNearestPlayer sets HeadYaw toward the nearest player within `range`
// blocks. Body Yaw stays where it was; only the head turns. Called every
// tick from the behaviour dispatch so the AI stays cheap.
//
// M7.10: cadence. Running the 6-cell range check on every
// mob every tick is the dominant cost in a crowded room.
// We rate-limit to every 5 ticks (0.25 s) for non-hostile
// mobs and every tick for hostile ones (they need to keep
// the player in view for the line-of-sight refresh in
// pickTarget). 5 ticks × 0.5 m/s mob = 2.5 cm of head lag
// at worst — visually invisible, but a 5× CPU saving.
func lookAtNearestPlayer(m *Mob, def MobDef, players []PlayerTarget, range_ float64) {
	// Cadence gate: hostile mobs (def.IsHostile) re-aim
	// every tick; everyone else every 5 ticks. ambientCD
	// is a free-running counter (1/tick, 0..5 cycle).
	if !def.IsHostile {
		if m.ambientCD%5 != 0 {
			return
		}
	}
	mx, my, mz := m.X, m.Y+1.6, m.Z
	best := math.MaxFloat64
	var bx, bz float64
	found := false
	for _, p := range players {
		d := math.Hypot(p.X-mx, math.Hypot(p.Y-my, p.Z-mz))
		if d > range_ || d >= best {
			continue
		}
		best, bx, bz, found = d, p.X, p.Z, true
	}
	if !found {
		return
	}
	m.HeadYaw = math.Atan2(-(bx - mx), bz-mz) * 180 / math.Pi
}

// faceTarget is a stricter version of lookAtNearestPlayer used by hostile
// mobs in combat — both body AND head track the target.
func faceTarget(m *Mob, players []PlayerTarget) {
	for _, p := range players {
		if p.UUID != m.target {
			continue
		}
		yaw := math.Atan2(-(p.X - m.X), p.Z-m.Z) * 180 / math.Pi
		m.Yaw, m.HeadYaw = yaw, yaw
		return
	}
}
