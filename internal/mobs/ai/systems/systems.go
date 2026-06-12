// Package aisystems owns the per-tick AI orchestrator. It is the
// single entry point driven from world.Manager.runOneTick; the
// world layer calls Tick(store, ctx) once per 20 Hz tick and the
// systems package drives every mob through:
//
//  1. Sensors → refresh brain memories for this tick.
//  2. Systems → gravity, sun-burn, water damage, despawn,
//     knockback consume, body rotation. These are NOT goals —
//     they run regardless of which goal is active.
//  3. Selectors → target acquisition first (FlagTarget), then
//     behaviour (FlagMove / FlagLook / FlagJump).
//
// The package owns:
//   - the public Tick function (replaces *mobs.Store.Tick from the
//     pre-rombak world layer)
//   - the aiStep helper (per-mob one-tick pipeline)
//   - buildAI (goal-selector factory from a MobDef)
//   - the per-tick system functions (ApplyGravity, SunburnSystem,
//     WaterDamageSystem, DespawnSystem, KnockbackSystem,
//     BodyRotationSystem, SunCheck, HasLineOfSight)
//
// The package is in the ai/ subfolder because the import graph
// flows mobs ← ai_systems, not the other way. Tick is exported so
// the world layer can call it without importing the per-goal
// subpackages.
package aisystems

import (
	"livingworld/internal/mobs"
	brain "livingworld/internal/mobs/ai/brain"
	context "livingworld/internal/mobs/ai/context"
	movement "livingworld/internal/mobs/ai/movement"
	navigation "livingworld/internal/mobs/ai/navigation"
	"math"
)

// Tick runs one AI pass over every mob. It is the single entry
// point driven from world.Manager.runOneTick. Callbacks fire
// OUTSIDE the store lock, mirroring drops.Store.TickPhysics.
//
// OnFireDamage is the one side-effect callback the world layer
// wires back to THIS store (HurtFire), which would re-lock s.mu
// while Tick holds it — a self-deadlock. Defer those events:
// collect them during the locked pass and replay them after the
// unlock, where HurtFire can take the lock safely.
func Tick(s *mobs.Store, ctx context.AIContext) {
	realFire := ctx.OnFireDamage
	var fireEvents []struct {
		id  int64
		dmg float32
	}
	if realFire != nil {
		ctx.OnFireDamage = func(id int64, dmg float32) {
			fireEvents = append(fireEvents, struct {
				id  int64
				dmg float32
			}{id, dmg})
		}
	}
	s.MuLock()
	mobMap := s.Mobs()
	moved := make([]mobs.Mob, 0, len(mobMap))
	for _, m := range mobMap {
		before := *m
		aiStep(m, &ctx)
		if m.X != before.X || m.Y != before.Y || m.Z != before.Z ||
			m.Yaw != before.Yaw || m.HeadYaw != before.HeadYaw || m.HeadPitch != before.HeadPitch {
			moved = append(moved, *m)
		}
	}
	snapshot := make([]mobs.Mob, 0, len(mobMap))
	for _, m := range mobMap {
		snapshot = append(snapshot, *m)
	}
	s.MuUnlock()
	cbs := s.OnMoveCBs()
	for _, e := range fireEvents {
		realFire(e.id, e.dmg)
	}
	for _, m := range moved {
		for _, cb := range cbs {
			cb(m)
		}
	}
	if ctx.OnSound != nil {
		ctx.OnSound(mobs.EmitSoundsFromSnapshot(snapshot, ctx.RNG))
	}
}

// aiStep runs one mob's AI for one tick. The rombak replaces the
// old monolithic state-machine switch with a vanilla goal-selector
// pipeline:
//
//  1. lazy-build the goal/target selectors + brain from the mob's
//     MobDef.
//  2. advance per-mob timers (cooldowns, jump-cooldown, aiTick
//     clock).
//  3. run always-on SYSTEMS that are not goals (gravity +
//     movement-mode integration, sun-burn, water damage, despawn,
//     knockback consume).
//  4. run sensors (populate brain memories for this tick).
//  5. tick the target selector, then the behaviour selector.
//
// Goals own all the *decisions*; systems own the *physics/world-
// coupling* that must happen regardless of which goal is active.
func aiStep(m *mobs.Mob, ctx *context.AIContext) {
	def := mobs.DefFor(m.Type)

	// 1. Build the AI on first tick (or after a respawn/clear).
	if m.GoalSel == nil {
		m.GoalSel, m.TargetSel = buildAI(def)
		m.Brain = brain.NewBrain()
		prof := navigation.NavProfileFor(def.Type, def.Movement)
		m.Nav = &prof
	}

	// 2. Timers.
	m.AITick++
	if m.CooldownTicks > 0 {
		m.CooldownTicks--
	}
	if m.JumpCooldown > 0 {
		m.JumpCooldown--
	}
	if m.AmbientCD > 0 {
		m.AmbientCD--
	} else {
		m.AmbientCD = 0
	}
	if m.PathTick > 0 {
		m.PathTick--
	}

	// 3. Systems.
	ApplyGravity(m, def, ctx)
	SunburnSystem(m, def, ctx)
	WaterDamageSystem(m, def, ctx)
	players := ctx.Players()
	DespawnSystem(m, def, players)
	KnockbackSystem(m)

	// 4. Sensors.
	RunSensors(m, ctx, players)

	// 5. Selectors.
	m.TargetSel.Tick(m, ctx)
	m.GoalSel.Tick(m, ctx)

	// 6. Body rotation control.
	BodyRotationSystem(m)
}

// ApplyGravity integrates one tick of vertical motion.
func ApplyGravity(m *mobs.Mob, def mobs.MobDef, ctx *context.AIContext) {
	if def.Movement == "fly" {
		// Flying mobs don't integrate gravity; their goal sets
		// vertical motion directly. Drain residual vy so a
		// leftover jump doesn't drift them upward forever.
		return
	}
	feetX, feetY, feetZ := int(math.Floor(m.X)), int(math.Floor(m.Y)), int(math.Floor(m.Z))
	prevY := m.Y
	m.Y += m.Vy
	if def.Movement == "hover" {
		m.Vy -= 0.04
	} else {
		m.Vy -= 0.08
	}
	newHeadY := int(math.Floor(m.Y)) + 1
	if m.Vy > 0 && newHeadY > feetY+1 && ctx.SolidAt(feetX, newHeadY, feetZ) {
		m.Vy = 0
		m.Y = prevY
	}
	if m.Vy <= 0 {
		nx, ny, nz := feetX, int(math.Floor(m.Y))-1, feetZ
		if ctx.SolidAt(nx, ny, nz) {
			m.Y = float64(ny + 1)
			m.Vy = 0
			m.OnGround = true
			return
		}
	}
	if m.Y < float64(mobs.MinWorldHeightForMobAI) {
		m.Y = float64(mobs.MinWorldHeightForMobAI)
		m.Vy = 0
		m.OnGround = true
	}
	if m.Vy != 0 {
		m.OnGround = false
	}
}

// SunburnSystem applies 1 HP/s of fire damage to undead mobs
// standing in direct sky light.
func SunburnSystem(m *mobs.Mob, def mobs.MobDef, ctx *context.AIContext) {
	if def.BurnsInDaylight && SunCheck(m, ctx) && ctx.OnFireDamage != nil {
		ctx.OnFireDamage(m.EntityID, 0.05)
	}
}

// WaterDamageSystem applies 1 HP/s to WaterSensitive mobs
// (enderman) in water.
func WaterDamageSystem(m *mobs.Mob, def mobs.MobDef, ctx *context.AIContext) {
	if !def.WaterSensitive || ctx.WaterAt == nil || ctx.OnFireDamage == nil {
		return
	}
	fx, fy, fz := int(math.Floor(m.X)), int(math.Floor(m.Y)), int(math.Floor(m.Z))
	if ctx.WaterAt(fx, fy, fz) {
		ctx.OnFireDamage(m.EntityID, 0.05)
	}
}

// DespawnSystem implements the vanilla distance/time despawn.
func DespawnSystem(m *mobs.Mob, def mobs.MobDef, players []context.PlayerTarget) {
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
	if m.Target != context.ZeroUUID() {
		m.NoPlayerTicks = 0
		return
	}
	m.NoPlayerTicks++
	if m.NoPlayerTicks >= timerTicks {
		m.Despawn = true
	}
}

// RunSensors refreshes the small memory set used by the goal layer.
func RunSensors(m *mobs.Mob, ctx *context.AIContext, players []context.PlayerTarget) {
	if m.Brain == nil {
		return
	}
	if len(players) > 0 {
		bestIdx := -1
		bestSq := math.MaxFloat64
		for i := range players {
			dx, dy, dz := players[i].X-m.X, players[i].Y-m.Y, players[i].Z-m.Z
			if sq := dx*dx + dy*dy + dz*dz; sq < bestSq {
				bestIdx, bestSq = i, sq
			}
		}
		if bestIdx >= 0 {
			m.Brain.Set(brain.MemNearestPlayer, players[bestIdx])
		}
	}
	if m.Target != context.ZeroUUID() {
		m.Brain.Set(brain.MemAttackTarget, m.Target)
	}
	if m.HurtBy != context.ZeroUUID() {
		m.Brain.Set(brain.MemHurtBy, m.HurtBy)
		m.Brain.Set(brain.MemHurtByTick, m.HurtByTick)
	}
}

// KnockbackSystem consumes any pending one-shot knockback impulse.
func KnockbackSystem(m *mobs.Mob) {
	if m.KnockbackVX == 0 && m.KnockbackVZ == 0 && m.KnockbackVY == 0 {
		return
	}
	m.X += m.KnockbackVX
	m.Z += m.KnockbackVZ
	m.Vy += m.KnockbackVY
	m.KnockbackVX, m.KnockbackVY, m.KnockbackVZ = 0, 0, 0
}

// SunCheck returns true if the mob's head is in direct sky light.
func SunCheck(m *mobs.Mob, ctx *context.AIContext) bool {
	if ctx.SkyLightAt == nil {
		return false
	}
	x, y, z := int(math.Floor(m.X)), int(math.Floor(m.Y))+1, int(math.Floor(m.Z))
	return ctx.SkyLightAt(x, y, z) >= 12
}

// HasLineOfSight is provided here as a re-export of the version in
// the aicontext package. Kept for callers that already import
// aisystems; the canonical implementation lives in aicontext
// (it is goal-free so it doesn't pull the goals subpackages into
// the import graph).
func HasLineOfSight(x0, y0, z0, x1, y1, z1 float64, solidAt func(x, y, z int) bool) bool {
	return context.HasLineOfSight(x0, y0, z0, x1, y1, z1, solidAt)
}

// BodyRotationSystem mirrors vanilla BodyRotationControl.
func BodyRotationSystem(m *mobs.Mob) {
	if m.WalkTicks > 0 {
		m.HeadStillTicks = 0
		if !m.GoalSel.LookGoalActive() {
			m.HeadYaw = movement.ApproachAngle(m.HeadYaw, m.Yaw, movement.MaxHeadYawTurn)
			m.HeadPitch = movement.ApproachAngle(m.HeadPitch, 0, movement.MaxHeadPitchTurn)
		}
		return
	}
	m.HeadStillTicks++
	if m.HeadStillTicks > movement.BodyFollowDelay {
		diff := movement.WrapDegrees(m.HeadYaw - m.Yaw)
		step := diff * 0.167
		if step > 0 && step < 1 {
			step = 1
		} else if step < 0 && step > -1 {
			step = -1
		}
		m.Yaw = movement.NormalizeAngle(m.Yaw + step)
	}
	movement.ClampHeadYawToBody(m)
	if !m.GoalSel.LookGoalActive() {
		m.HeadYaw = movement.ApproachAngle(m.HeadYaw, m.Yaw, movement.MaxHeadYawTurn)
		m.HeadPitch = movement.ApproachAngle(m.HeadPitch, 0, movement.MaxHeadPitchTurn)
	}
}
