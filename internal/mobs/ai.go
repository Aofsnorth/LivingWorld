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
	Gamemode    int // 0 survival, 1 creative, 2 adventure, 3 spectator

	// LookYaw / LookPitch are the player's view angles in degrees (Minecraft
	// convention: yaw 0 = +Z south, pitch + = down). Used by the enderman
	// gaze check (Milestone C). Zero values are harmless for mobs that don't
	// read them. The world layer fills these from the player's head rotation.
	LookYaw   float64
	LookPitch float64
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

	// OnHitEffect is called when a melee swing applies a status effect
	// (e.g. husk → hunger, cave spider → poison, wither skeleton →
	// wither). The world layer translates the HitEffect into the
	// per-edition damage / effect packet.
	OnHitEffect func(targetUUID [16]byte, attackerID int64, effect HitEffect)

	// OnThrow is called when a mob (iron golem) picks up a player and throws
	// them. Vanilla: 3+ blocks up + 4 damage. The world layer applies the
	// upward velocity via players.Push.
	OnThrow func(targetUUID [16]byte, attackerID int64, damage float32)

	// OnShootProjectile is the unified ranged-fire hook. The projectileType
	// string tells the bridge which kind of projectile to spawn:
	//
	//   "arrow"               — vanilla arrow, default
	//   "arrow_slowness"      — tipped arrow with slowness (stray)
	//   "arrow_poison"        — tipped arrow with poison (bogged)
	//   "small_fireball"      — blaze: +1 explosion damage
	//   "large_fireball"      — ghast: splash 6 damage, 3 b radius
	//   "trident"             — drowned: thrown trident
	//   "potion"              — witch: throw a splash potion
	//
	// Bridges that haven't wired this yet fall back to OnShootArrow.
	OnShootProjectile func(ownerID int64, x, y, z, yaw, pitch float64, projectileType string)

	// WaterAt is used by WaterSensitive mobs (enderman) to detect water
	// contact. May be nil; the AI degrades gracefully (no water damage).
	WaterAt func(x, y, z int) bool

	// IsDay reports the world time of day. Used by spider (aggressive at
	// night only). May be nil; the AI treats nil as "always night".
	IsDay func() bool

	// DoorAt reports whether the block at (x, y, z) is a door (any wood
	// type). The BreaksDoors zombie variants use this to recognise an
	// obstacle they can punch through. May be nil; the AI degrades to
	// "treat as solid wall".
	DoorAt func(x, y, z int) bool

	// OnBreakDoor removes a door block from the world and publishes the
	// resulting block update. Returns true when a door was actually
	// removed. The AI calls this only after DoorAt reports true.
	OnBreakDoor func(x, y, z int) bool

	// Difficulty is the current world difficulty string ("peaceful",
	// "easy", "normal", "hard"). The AI gates door-breaking by difficulty:
	// vanilla zombies only break wooden doors on Hard.
	Difficulty string

	// HeldItem returns the namespaced item id of the item in the given
	// player's main hand. Used by passive mobs to detect food attraction
	// (cow follows wheat, pig follows carrot, etc). May be nil.
	HeldItem func(playerUUID [16]byte) string

	// BlockNameAt returns the namespaced id of the block at (x, y, z)
	// ("minecraft:lava", "minecraft:water", "minecraft:powder_snow", …).
	// Used by the navigation penalty profiles (Milestone B) to price
	// hazardous cells per mob type. May be nil; navigation degrades to the
	// boolean SolidAt model.
	BlockNameAt func(x, y, z int) string
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
		aiStep(m, &ctx)
		if m.X != before.X || m.Y != before.Y || m.Z != before.Z ||
			m.Yaw != before.Yaw || m.HeadYaw != before.HeadYaw {
			moved = append(moved, *m)
		}
	}
	// UX: pre-compute sound emissions for the tick. The snapshot is taken
	// OUT of the moved slice so we get a single Mob per id (even if a mob
	// didn't move, it might still play an ambient).
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

// aiStep runs one mob's AI for one tick. The rombak replaces the old monolithic
// state-machine switch with a vanilla goal-selector pipeline:
//
//  1. lazy-build the goal/target selectors + brain from the mob's MobDef.
//  2. advance per-mob timers (cooldowns, jump-cooldown, aiTick clock).
//  3. run always-on SYSTEMS that are not goals (gravity + movement-mode
//     integration, sun-burn, water damage, despawn, knockback consume).
//  4. run sensors (populate brain memories for this tick).
//  5. tick the target selector, then the behaviour selector.
//
// Goals own all the *decisions*; systems own the *physics/world-coupling* that
// must happen regardless of which goal is active.
func aiStep(m *Mob, ctx *AIContext) {
	def := defFor(m.Type)

	// 1. Build the AI on first tick (or after a respawn/clear).
	if m.goalSel == nil {
		m.goalSel, m.targetSel = buildAI(def)
		m.brain = newBrain()
		prof := navProfileFor(def)
		m.nav = &prof
	}

	// 2. Timers.
	m.aiTick++
	if m.cooldownTicks > 0 {
		m.cooldownTicks--
	}
	if m.jumpCooldown > 0 {
		m.jumpCooldown--
	}
	if m.ambientCD > 0 {
		m.ambientCD--
	} else {
		// Free-running counter used by idle head-drift cadence + hop timing.
		m.ambientCD = 0
	}
	if m.pathTick > 0 {
		m.pathTick--
	}

	// 3. Systems.
	applyGravity(m, def, ctx)
	sunburnSystem(m, def, ctx)
	waterDamageSystem(m, def, ctx)
	players := ctx.Players()
	despawnSystem(m, def, players)
	knockbackSystem(m)

	// 4. Sensors.
	runSensors(m, ctx, players)

	// 5. Selectors — target acquisition first, then behaviour.
	m.targetSel.tick(m, ctx)
	m.goalSel.tick(m, ctx)
}

// zero16 returns the zero-valued [16]byte. Helper for the "no target" case.
func zero16() [16]byte { return [16]byte{} }

// --- systems --------------------------------------------------------------

// sunburnSystem applies 1 HP/s of fire damage to undead mobs standing in
// direct sky light (vanilla 0.05 HP/tick at 20 Hz). The world layer applies
// the damage and broadcasts the flame overlay via OnFireDamage.
func sunburnSystem(m *Mob, def MobDef, ctx *AIContext) {
	if def.BurnsInDaylight && sunCheck(m, ctx) && ctx.OnFireDamage != nil {
		ctx.OnFireDamage(m.EntityID, 0.05)
	}
}

// waterDamageSystem applies 1 HP/s to WaterSensitive mobs (enderman) in water.
// We approximate with a single feet-cell check; rain is dropped for v1.
func waterDamageSystem(m *Mob, def MobDef, ctx *AIContext) {
	if !def.WaterSensitive || ctx.WaterAt == nil || ctx.OnFireDamage == nil {
		return
	}
	fx, fy, fz := int(math.Floor(m.X)), int(math.Floor(m.Y)), int(math.Floor(m.Z))
	if ctx.WaterAt(fx, fy, fz) {
		ctx.OnFireDamage(m.EntityID, 0.05)
	}
}

// despawnSystem implements the vanilla distance/time despawn for hostile,
// non-persistent mobs:
//   1. >128 blocks from every player → instant despawn.
//   2. ≤128 blocks but no player close for 30 s (600 ticks) → despawn, UNLESS
//      the mob has an active target (engaged mobs don't vanish mid-chase).
// Passive/neutral and PersistenceRequired mobs never despawn here.
func despawnSystem(m *Mob, def MobDef, players []PlayerTarget) {
	if !def.IsHostile || m.PersistenceRequired {
		return
	}
	const despawnDistSq = 128.0 * 128.0
	const timerTicks = 600

	nearestSq := math.MaxFloat64
	for _, p := range players {
		dx, dz := p.X-m.X, p.Z-m.Z
		if sq := dx*dx + dz*dz; sq < nearestSq {
			nearestSq = sq
		}
	}
	if nearestSq > despawnDistSq {
		m.Despawn = true
		return
	}
	if m.target != zero16() {
		m.noPlayerTicks = 0
		return
	}
	m.noPlayerTicks++
	if m.noPlayerTicks >= timerTicks {
		m.Despawn = true
	}
}

// knockbackSystem consumes any pending one-shot knockback impulse stored by
// HurtDirectWithKnockback. The horizontal components slide the mob; the
// vertical bump feeds applyGravity's integrator next tick.
func knockbackSystem(m *Mob) {
	if m.KnockbackVX == 0 && m.KnockbackVZ == 0 && m.KnockbackVY == 0 {
		return
	}
	m.X += m.KnockbackVX
	m.Z += m.KnockbackVZ
	m.vy += m.KnockbackVY
	m.KnockbackVX, m.KnockbackVY, m.KnockbackVZ = 0, 0, 0
}

// applyGravity integrates one tick of vertical motion. The model is the
// standard `vy -= 0.08; m.Y += vy` integrator, with a hard ground-snap when
// the cell below the feet is solid. Jumps inject `vy = 0.42` (the vanilla
// jump-strength unit). Head-block is checked so a mob doesn't bury itself.
//
// Movement-mode dispatch:
//   "fly"   — Phantom, Blaze. No gravity; the mob's own goal drives vertical.
//   "hover" — Ghast. Half-gravity so the bob is gentle.
//   "climb" — Spider. Wall-walk handled in the move primitive.
//   "hop"   — Slime/MagmaCube. Full gravity; jumps forced by the move goal.
//   "walk"  — default, full gravity.
func applyGravity(m *Mob, def MobDef, ctx *AIContext) {
	if def.Movement == "fly" {
		// Flying mobs don't integrate gravity; their goal sets vertical
		// motion directly. Drain residual vy so a leftover jump doesn't
		// drift them upward forever.
		return
	}
	feetX, feetY, feetZ := int(math.Floor(m.X)), int(math.Floor(m.Y)), int(math.Floor(m.Z))

	prevY := m.Y
	m.Y += m.vy
	if def.Movement == "hover" {
		m.vy -= 0.04
	} else {
		m.vy -= 0.08
	}

	// Head-block check: stop upward motion if the mob bonked a ceiling.
	newHeadY := int(math.Floor(m.Y)) + 1
	if m.vy > 0 && newHeadY > feetY+1 && ctx.SolidAt(feetX, newHeadY, feetZ) {
		m.vy = 0
		m.Y = prevY
	}

	// Ground-snap: only when falling and the destination cell is solid.
	if m.vy <= 0 {
		nx, ny, nz := feetX, int(math.Floor(m.Y))-1, feetZ
		if ctx.SolidAt(nx, ny, nz) {
			m.Y = float64(ny + 1)
			m.vy = 0
			m.OnGround = true
			return
		}
	}

	// Below the world: clamp with a sentinel; the world tick handles void dmg.
	if m.Y < float64(minY) {
		m.Y = float64(minY)
		m.vy = 0
		m.OnGround = true
	}
	if m.vy != 0 {
		m.OnGround = false
	}
}

// minY mirrors world.MinWorldHeight without a cross-package import.
const minY = -64

// jumpMob is the vanilla jump-strength injection. Callers decide *when* to
// jump; this is the *how* (one-tick impulse, fixed cooldown).
func jumpMob(m *Mob) {
	if m.OnGround && m.jumpCooldown == 0 {
		m.vy = 0.42
		m.jumpCooldown = 6
	}
}

// sunCheck returns true if the mob's head is in direct sky light. We
// approximate with a single sky-light read just above the mob's head.
func sunCheck(m *Mob, ctx *AIContext) bool {
	if ctx.SkyLightAt == nil {
		return false
	}
	x, y, z := int(math.Floor(m.X)), int(math.Floor(m.Y))+1, int(math.Floor(m.Z))
	return ctx.SkyLightAt(x, y, z) >= 12
}

// hasLineOfSight walks the voxel line from (x0,y0,z0) to (x1,y1,z1) one cell
// at a time; returns false if any solid block is hit. Eye positions are passed
// in by the caller (mob eye ≈ feet+1.6, player eye ≈ feet+1.6).
func hasLineOfSight(x0, y0, z0, x1, y1, z1 float64, solidAt func(x, y, z int) bool) bool {
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

// findPlayer returns a pointer to the player with the given UUID in the slice,
// or nil. Shared by the attack/follow goals.
func findPlayer(players []PlayerTarget, uuid [16]byte) *PlayerTarget {
	for i := range players {
		if players[i].UUID == uuid {
			return &players[i]
		}
	}
	return nil
}
