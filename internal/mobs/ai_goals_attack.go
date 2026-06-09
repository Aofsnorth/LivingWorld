package mobs

import "math"

// Combat behaviour goals. These read m.target (set by the target selector) and
// drive the mob into attack range, then fire the appropriate AIContext combat
// callback. They occupy FlagMove (+FlagLook) so only one runs at a time.

// hasTarget reports whether m.target is set and the player still exists.
func targetPlayer(m *Mob, ctx *AIContext) *PlayerTarget {
	if m.target == zero16() {
		return nil
	}
	return findPlayer(ctx.Players(), m.target)
}

// meleeAttackGoal pathfinds to the target and swings on cooldown — vanilla
// MeleeAttackGoal. Iron golem throws instead of swinging (ThrowDamage>0); the
// on-hit status effect (husk hunger, wither skeleton wither, …) fires too.
// Zombie-family door-breaking happens inside navigateTo's step collision.
type meleeAttackGoal struct{ baseGoal }

func (meleeAttackGoal) Flags() GoalFlag { return FlagMove | FlagLook }

func (meleeAttackGoal) CanUse(m *Mob, ctx *AIContext) bool { return targetPlayer(m, ctx) != nil }
func (meleeAttackGoal) CanContinue(m *Mob, ctx *AIContext) bool {
	return targetPlayer(m, ctx) != nil
}

func (meleeAttackGoal) Tick(m *Mob, ctx *AIContext) {
	p := targetPlayer(m, ctx)
	if p == nil {
		return
	}
	def := defFor(m.Type)
	dx, dz := p.X-m.X, p.Z-m.Z
	distSq := dx*dx + dz*dz
	lookAt(m, p.X, p.Z, true)

	if distSq > def.AttackRange {
		// Out of range — close the distance.
		m.state = StatePursue
		navigateTo(m, def, ctx, p.X, p.Y, p.Z, def.ChaseSpeed)
		return
	}
	// In range — swing on cooldown. Mobs with no melee/throw damage (creeper)
	// only close the distance; the detonation is owned by swellGoal.
	m.state = StateMelee
	if def.AttackDamage <= 0 && def.ThrowDamage <= 0 {
		return
	}
	if m.cooldownTicks > 0 {
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
	m.cooldownTicks = def.AttackCooldown
}

func (meleeAttackGoal) Stop(m *Mob, ctx *AIContext) {
	if m.state == StateMelee || m.state == StatePursue {
		m.state = StateIdle
	}
}

// rangedAttackGoal kites the target: keeps inside firing range but outside a
// minimum stand-off, warms up for RangedWarmupTicks, then fires the mob's
// projectile via OnShootProjectile — vanilla RangedAttackGoal /
// RangedBowAttackGoal. Used by skeleton/stray/bogged/blaze/ghast/witch/drowned.
type rangedAttackGoal struct{ baseGoal }

func (rangedAttackGoal) Flags() GoalFlag { return FlagMove | FlagLook }

func (rangedAttackGoal) CanUse(m *Mob, ctx *AIContext) bool { return targetPlayer(m, ctx) != nil }
func (rangedAttackGoal) CanContinue(m *Mob, ctx *AIContext) bool {
	return targetPlayer(m, ctx) != nil
}

func (rangedAttackGoal) Tick(m *Mob, ctx *AIContext) {
	p := targetPlayer(m, ctx)
	if p == nil {
		return
	}
	def := defFor(m.Type)
	// def.AttackRange holds the SQUARED engagement range for ranged mobs
	// (e.g. skeleton 225 = 15², ghast 1600 = 40²).
	maxRange := math.Sqrt(def.AttackRange)
	minRange := 4.0
	if def.Movement == "fly" || def.Movement == "hover" {
		minRange = 6.0
	}
	dx, dz := p.X-m.X, p.Z-m.Z
	dist := math.Hypot(dx, dz)
	yaw := math.Atan2(-dx, dz) * 180 / math.Pi
	m.HeadYaw = yaw

	if dist > maxRange {
		// Too far — approach.
		m.state = StatePursue
		m.drawTicks = 0
		navigateTo(m, def, ctx, p.X, p.Y, p.Z, def.ChaseSpeed)
		return
	}
	if dist < minRange {
		// Too close — back away (kite).
		m.state = StatePursue
		m.drawTicks = 0
		fleeFrom(m, def, ctx, p.X, p.Z, def.WanderSpeed)
		return
	}
	// In the firing band: stand, aim, warm up, fire.
	m.state = StateShoot
	if m.drawTicks == 0 && m.cooldownTicks == 0 {
		m.drawTicks = def.RangedWarmupTicks
		if m.drawTicks == 0 {
			m.drawTicks = 20
		}
	}
	if m.drawTicks > 0 {
		m.drawTicks--
		if m.drawTicks > 0 {
			return
		}
	}
	if m.cooldownTicks > 0 {
		return
	}
	// Fire.
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
	m.Yaw = yaw
	m.cooldownTicks = def.AttackCooldown
}

func (rangedAttackGoal) Stop(m *Mob, ctx *AIContext) {
	m.drawTicks = 0
	if m.state == StateShoot || m.state == StatePursue {
		m.state = StateIdle
	}
}

// leapAtTargetGoal makes a spider leap toward its target when close and
// grounded — vanilla LeapAtTargetGoal. Jump-only so it composes with the melee
// move goal.
type leapAtTargetGoal struct{ baseGoal }

func (leapAtTargetGoal) Flags() GoalFlag { return FlagJump }

func (leapAtTargetGoal) CanUse(m *Mob, ctx *AIContext) bool {
	p := targetPlayer(m, ctx)
	if p == nil || !m.OnGround || m.jumpCooldown != 0 {
		return false
	}
	dx, dz := p.X-m.X, p.Z-m.Z
	d := dx*dx + dz*dz
	return d >= 4 && d <= 16 // 2–4 blocks
}

func (leapAtTargetGoal) CanContinue(m *Mob, ctx *AIContext) bool { return false }

func (leapAtTargetGoal) Start(m *Mob, ctx *AIContext) {
	p := targetPlayer(m, ctx)
	if p == nil {
		return
	}
	dx, dz := p.X-m.X, p.Z-m.Z
	d := math.Hypot(dx, dz)
	if d == 0 {
		return
	}
	m.vy = 0.42
	m.X += dx / d * 0.4
	m.Z += dz / d * 0.4
	m.OnGround = false
	m.jumpCooldown = 6
}

func (leapAtTargetGoal) Tick(m *Mob, ctx *AIContext) {}

// swellGoal is the creeper fuse — vanilla SwellGoal. Once the target is in
// explosion range the fuse arms and counts down; leaving range disarms it
// (vanilla 1.20: the swell reverses if the player escapes, unlike the legacy
// "committed fuse" approximation). At zero it detonates via OnExplode.
type swellGoal struct{ baseGoal }

func (swellGoal) Flags() GoalFlag { return FlagMove }

func (swellGoal) CanUse(m *Mob, ctx *AIContext) bool {
	p := targetPlayer(m, ctx)
	if p == nil {
		return false
	}
	def := defFor(m.Type)
	dx, dz := p.X-m.X, p.Z-m.Z
	return math.Hypot(dx, dz) < def.ExplosionRadius
}

func (swellGoal) CanContinue(m *Mob, ctx *AIContext) bool {
	// Keep swelling while we still have a target; if it leaves range the
	// fuse winds back down (handled in Tick).
	return targetPlayer(m, ctx) != nil
}

func (g swellGoal) Start(m *Mob, ctx *AIContext) {
	def := defFor(m.Type)
	m.fuseTicks = def.FuseTicks
	m.state = StateFuse
}

func (swellGoal) Tick(m *Mob, ctx *AIContext) {
	def := defFor(m.Type)
	p := targetPlayer(m, ctx)
	inRange := false
	if p != nil {
		dx, dz := p.X-m.X, p.Z-m.Z
		inRange = math.Hypot(dx, dz) < def.ExplosionRadius
		if dx != 0 || dz != 0 {
			m.Yaw = math.Atan2(-dx, dz) * 180 / math.Pi
		}
	}
	if inRange {
		m.state = StateFuse
		m.fuseTicks--
		if m.fuseTicks <= 0 {
			if ctx.OnExplode != nil {
				ctx.OnExplode(m.EntityID, m.X, m.Y+1.0, m.Z, float64(def.ExplosionPower))
			}
			m.Despawn = true
		}
		return
	}
	// Out of range — wind the fuse back up (vanilla reversible swell).
	m.fuseTicks++
	if m.fuseTicks >= def.FuseTicks {
		m.fuseTicks = def.FuseTicks
		m.state = StateIdle
	}
}

func (swellGoal) Stop(m *Mob, ctx *AIContext) {
	def := defFor(m.Type)
	m.fuseTicks = def.FuseTicks
	if m.state == StateFuse {
		m.state = StateIdle
	}
}
