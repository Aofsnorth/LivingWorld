// Package aiattack owns the four vanilla combat-behaviour goals:
// melee, ranged, leap-at-target, and the creeper swell (fuse).
// They read m.Target (set by the target selector) and drive the
// mob into attack range, then fire the appropriate AIContext
// combat callback. They occupy FlagMove (+FlagLook) so only one
// runs at a time.
package aiattack

import (
	"livingworld/internal/mobs"
	context "livingworld/internal/mobs/ai/context"
	movement "livingworld/internal/mobs/ai/movement"
	selector "livingworld/internal/mobs/ai/selector"
	"math"
)

// TargetPlayer reports whether m.Target is set and the player still
// exists. Used by melee, ranged, leap, and swell goals.
func TargetPlayer(m *mobs.Mob, ctx *context.AIContext) *context.PlayerTarget {
	if m.Target == context.ZeroUUID() {
		return nil
	}
	p := context.FindPlayer(ctx.Players(), m.Target)
	if p == nil || p.Dead || p.Gamemode == 1 || p.Gamemode == 3 {
		return nil
	}
	return p
}

// MeleeAttackGoal pathfinds to the target and swings on cooldown —
// vanilla MeleeAttackGoal. Iron golem throws instead of swinging
// (ThrowDamage>0); the on-hit status effect (husk hunger, wither
// skeleton wither, …) fires too. Zombie-family door-breaking
// happens inside NavigateTo's step collision.
type MeleeAttackGoal struct {
	selector.BaseGoal
}

func (MeleeAttackGoal) Flags() selector.Flag { return selector.FlagMove | selector.FlagLook }

func (MeleeAttackGoal) CanUse(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	return TargetPlayer(m, ctx) != nil
}
func (MeleeAttackGoal) CanContinue(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	return TargetPlayer(m, ctx) != nil
}

func (MeleeAttackGoal) Tick(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	p := TargetPlayer(m, ctx)
	if p == nil {
		return
	}
	def := mobs.DefFor(m.Type)
	dx, dz := p.X-m.X, p.Z-m.Z
	distSq := dx*dx + dz*dz
	movement.LookAt(m, p.X, p.Y+movement.PlayerEyeHeight, p.Z, true)

	hasLOS := ctx.SolidAt == nil || context.HasLineOfSight(m.X, m.Y+1.0, m.Z, p.X, p.Y+1.0, p.Z, ctx.SolidAt)
	if distSq > def.AttackRange || math.Abs(p.Y-m.Y) > 2.2 || !hasLOS {
		m.State = mobs.StatePursue
		movement.NavigateTo(m, def, ctx, p.X, p.Y, p.Z, def.ChaseSpeed)
		return
	}
	m.State = mobs.StateMelee
	if def.AttackDamage <= 0 && def.ThrowDamage <= 0 {
		return
	}
	if m.CooldownTicks > 0 {
		return
	}
	switch {
	case def.ThrowDamage > 0 && ctx.OnThrow != nil:
		ctx.OnThrow(p.UUID, m.EntityID, def.ThrowDamage)
	case ctx.OnMeleeAttack != nil:
		ctx.OnMeleeAttack(p.UUID, m.EntityID, float32(def.AttackDamage))
	}
	if def.OnHit.Type != "" && ctx.OnHitEffect != nil {
		ctx.OnHitEffect(p.UUID, m.EntityID, def.OnHit)
	}
	m.CooldownTicks = def.AttackCooldown
}

func (MeleeAttackGoal) Stop(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	if m.State == mobs.StateMelee || m.State == mobs.StatePursue {
		m.State = mobs.StateIdle
	}
}

// RangedAttackGoal kites the target: keeps inside firing range but
// outside a minimum stand-off, warms up for RangedWarmupTicks,
// then fires the mob's projectile via OnShootProjectile — vanilla
// RangedAttackGoal / RangedBowAttackGoal. Used by
// skeleton/stray/bogged/blaze/ghast/witch/drowned.
type RangedAttackGoal struct {
	selector.BaseGoal
}

func (RangedAttackGoal) Flags() selector.Flag { return selector.FlagMove | selector.FlagLook }

func (RangedAttackGoal) CanUse(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	return TargetPlayer(m, ctx) != nil
}
func (RangedAttackGoal) CanContinue(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	return TargetPlayer(m, ctx) != nil
}

func (RangedAttackGoal) Tick(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	p := TargetPlayer(m, ctx)
	if p == nil {
		return
	}
	def := mobs.DefFor(m.Type)
	maxRange := math.Sqrt(def.AttackRange)
	minRange := 4.0
	if def.Movement == "fly" || def.Movement == "hover" {
		minRange = 6.0
	}
	dx, dz := p.X-m.X, p.Z-m.Z
	dist := math.Hypot(dx, dz)
	yaw := math.Atan2(-dx, dz)*180/math.Pi + 180
	movement.LookAt(m, p.X, p.Y+movement.PlayerEyeHeight, p.Z, false)

	hasLOS := ctx.SolidAt == nil || context.HasLineOfSight(m.X, m.Y+1.6, m.Z, p.X, p.Y+1.0, p.Z, ctx.SolidAt)
	if dist > maxRange || !hasLOS {
		m.State = mobs.StatePursue
		m.DrawTicks = 0
		movement.NavigateTo(m, def, ctx, p.X, p.Y, p.Z, def.ChaseSpeed)
		return
	}
	if dist < minRange {
		m.State = mobs.StatePursue
		m.DrawTicks = 0
		movement.FleeFrom(m, def, ctx, p.X, p.Z, def.WanderSpeed)
		return
	}
	m.State = mobs.StateShoot
	if m.DrawTicks == 0 && m.CooldownTicks == 0 {
		m.DrawTicks = def.RangedWarmupTicks
		if m.DrawTicks == 0 {
			m.DrawTicks = 20
		}
	}
	if m.DrawTicks > 0 {
		m.DrawTicks--
		if m.DrawTicks > 0 {
			return
		}
	}
	if m.CooldownTicks > 0 {
		return
	}
	dy := (p.Y + 1.0) - (m.Y + 1.6)
	pitch := math.Atan2(dy+0.2*dist, dist) * 180 / math.Pi
	kind := def.RangedProjectile
	if kind == "" {
		kind = "arrow"
	}
	switch {
	case ctx.OnShootProjectile != nil:
		ctx.OnShootProjectile(m.EntityID, m.X, m.Y+1.6, m.Z, yaw, pitch, kind)
	case ctx.OnShootArrow != nil:
		ctx.OnShootArrow(m.EntityID, m.X, m.Y+1.6, m.Z, yaw, pitch)
	}
	movement.SetBodyYaw(m, yaw)
	m.CooldownTicks = def.AttackCooldown
}

func (RangedAttackGoal) Stop(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	m.DrawTicks = 0
	if m.State == mobs.StateShoot || m.State == mobs.StatePursue {
		m.State = mobs.StateIdle
	}
}

// LeapAtTargetGoal makes a spider leap toward its target when
// close and grounded — vanilla LeapAtTargetGoal. Jump-only so it
// composes with the melee move goal.
type LeapAtTargetGoal struct {
	selector.BaseGoal
}

func (LeapAtTargetGoal) Flags() selector.Flag { return selector.FlagJump }

func (LeapAtTargetGoal) CanUse(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	p := TargetPlayer(m, ctx)
	if p == nil || !m.OnGround || m.JumpCooldown != 0 {
		return false
	}
	dx, dz := p.X-m.X, p.Z-m.Z
	d := dx*dx + dz*dz
	return d >= 4 && d <= 16
}

func (LeapAtTargetGoal) CanContinue(body any, ctx *context.AIContext) bool { return false }

func (LeapAtTargetGoal) Start(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	p := TargetPlayer(m, ctx)
	if p == nil {
		return
	}
	dx, dz := p.X-m.X, p.Z-m.Z
	d := math.Hypot(dx, dz)
	if d == 0 {
		return
	}
	m.Vy = 0.42
	m.X += dx / d * 0.4
	m.Z += dz / d * 0.4
	m.OnGround = false
	m.JumpCooldown = 6
}

func (LeapAtTargetGoal) Tick(body any, ctx *context.AIContext) {}

// SwellGoal is the creeper fuse — vanilla SwellGoal. Once the
// target is in explosion range the fuse arms and counts down;
// leaving range disarms it (vanilla 1.20: the swell reverses if
// the player escapes, unlike the legacy "committed fuse"
// approximation). At zero it detonates via OnExplode.
type SwellGoal struct {
	selector.BaseGoal
}

func (SwellGoal) Flags() selector.Flag { return selector.FlagMove | selector.FlagLook }

func (SwellGoal) CanUse(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	p := TargetPlayer(m, ctx)
	if p == nil {
		return false
	}
	dx, dz := p.X-m.X, p.Z-m.Z
	if math.Hypot(dx, dz) > 3.0 || math.Abs(p.Y-m.Y) > 3.0 {
		return false
	}
	return ctx.SolidAt == nil || context.HasLineOfSight(m.X, m.Y+1.0, m.Z, p.X, p.Y+1.0, p.Z, ctx.SolidAt)
}

func (SwellGoal) CanContinue(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	p := TargetPlayer(m, ctx)
	if p == nil {
		return false
	}
	dx, dy, dz := p.X-m.X, p.Y-m.Y, p.Z-m.Z
	// Vanilla SwellGoal: once armed within 3 blocks, creeper stays in SwellGoal
	// until the target moves > 7 blocks away (3D distance > 7.0 -> 49.0 sq) or
	// line-of-sight is broken and the fuse fully unwinds.
	if dx*dx+dy*dy+dz*dz > 49.0 {
		return false
	}
	def := mobs.DefFor(m.Type)
	hasLOS := ctx.SolidAt == nil || context.HasLineOfSight(m.X, m.Y+1.0, m.Z, p.X, p.Y+1.0, p.Z, ctx.SolidAt)
	if !hasLOS && m.FuseTicks >= def.FuseTicks {
		return false
	}
	return true
}

func (SwellGoal) Start(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	def := mobs.DefFor(m.Type)
	m.FuseTicks = def.FuseTicks
	m.State = mobs.StateFuse
	m.Path = nil
	m.WalkTicks = 0
}

func (SwellGoal) Tick(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	def := mobs.DefFor(m.Type)
	p := TargetPlayer(m, ctx)
	inRange := false
	if p != nil {
		dx, dy, dz := p.X-m.X, p.Y-m.Y, p.Z-m.Z
		hasLOS := ctx.SolidAt == nil || context.HasLineOfSight(m.X, m.Y+1.0, m.Z, p.X, p.Y+1.0, p.Z, ctx.SolidAt)
		// Vanilla: fuse counts down as long as the player has LOS and remains within 7 blocks (49 sq).
		inRange = hasLOS && (dx*dx+dy*dy+dz*dz <= 49.0)
		movement.LookAt(m, p.X, p.Y+movement.PlayerEyeHeight, p.Z, true)
	}
	if inRange {
		m.State = mobs.StateFuse
		m.FuseTicks--
		if m.FuseTicks <= 0 {
			if ctx.OnExplode != nil {
				ctx.OnExplode(m.EntityID, m.X, m.Y+1.0, m.Z, float64(def.ExplosionPower))
			}
			m.Despawn = true
		}
		return
	}
	m.FuseTicks++
	if m.FuseTicks >= def.FuseTicks {
		m.FuseTicks = def.FuseTicks
		m.State = mobs.StateIdle
	}
}

func (SwellGoal) Stop(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	def := mobs.DefFor(m.Type)
	m.FuseTicks = def.FuseTicks
	if m.State == mobs.StateFuse {
		m.State = mobs.StateIdle
	}
}
