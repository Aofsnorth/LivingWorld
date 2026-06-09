package mobs

import "math"

// Movement / idle / look behaviour goals. These occupy FlagMove and/or
// FlagLook and run on the mob's goalSelector.

// floatGoal keeps the mob's head above water — vanilla FloatGoal. While the
// feet cell is water it forces a jump every tick so the mob bobs to the
// surface instead of sinking. Occupies FlagJump so it doesn't fight move goals.
type floatGoal struct{ baseGoal }

func (floatGoal) Flags() GoalFlag { return FlagJump }

func (floatGoal) CanUse(m *Mob, ctx *AIContext) bool {
	if ctx.WaterAt == nil {
		return false
	}
	fx, fy, fz := int(math.Floor(m.X)), int(math.Floor(m.Y)), int(math.Floor(m.Z))
	return ctx.WaterAt(fx, fy, fz)
}

func (g floatGoal) CanContinue(m *Mob, ctx *AIContext) bool { return g.CanUse(m, ctx) }

func (floatGoal) Tick(m *Mob, ctx *AIContext) {
	// A gentle upward impulse each tick; lighter than a full jump so the mob
	// floats rather than launches.
	if m.vy < 0.1 {
		m.vy += 0.04
	}
	m.OnGround = false
}

// panicGoal makes a passive mob sprint away from its last attacker for a short
// window after being hurt — vanilla PanicGoal. Highest-priority passive move
// goal so panic overrides grazing/strolling.
type panicGoal struct{ baseGoal }

func (panicGoal) Flags() GoalFlag { return FlagMove | FlagLook }

func (panicGoal) CanUse(m *Mob, ctx *AIContext) bool {
	if m.hurtBy == zero16() {
		return false
	}
	return m.aiTick-m.hurtByTick <= panicDurationTicks
}

func (g panicGoal) CanContinue(m *Mob, ctx *AIContext) bool { return g.CanUse(m, ctx) }

// panicDurationTicks is the vanilla passive panic window (2 s).
const panicDurationTicks = 40

// panicSpeedMul is the vanilla PanicGoal speed modifier — a fleeing passive
// sprints noticeably faster than it ambles.
const panicSpeedMul = 2.0

func (panicGoal) Tick(m *Mob, ctx *AIContext) {
	def := defFor(m.Type)
	if p := findPlayer(ctx.Players(), m.hurtBy); p != nil {
		fleeFrom(m, def, ctx, p.X, p.Z, def.WanderSpeed*panicSpeedMul)
	} else {
		// Attacker gone — just keep running forward.
		fleeFrom(m, def, ctx, m.X-math.Sin(m.Yaw*math.Pi/180), m.Z+math.Cos(m.Yaw*math.Pi/180), def.WanderSpeed*panicSpeedMul)
	}
}

func (panicGoal) Stop(m *Mob, ctx *AIContext) {
	// Forget the attacker so the mob settles instead of re-panicking.
	if m.aiTick-m.hurtByTick > panicDurationTicks {
		m.hurtBy = zero16()
	}
}

// temptGoal makes a passive mob follow a nearby player holding its preferred
// food — vanilla TemptGoal. Occupies move+look.
type temptGoal struct{ baseGoal }

func (temptGoal) Flags() GoalFlag { return FlagMove | FlagLook }

// temptRangeSq is the squared distance within which the food lure works (8 b).
const temptRangeSq = 64.0

func (temptGoal) temptingPlayer(m *Mob, ctx *AIContext) *PlayerTarget {
	def := defFor(m.Type)
	if def.FoodItem == "" || ctx.HeldItem == nil {
		return nil
	}
	players := ctx.Players()
	var best *PlayerTarget
	bestSq := temptRangeSq
	for i := range players {
		p := &players[i]
		dx, dz := p.X-m.X, p.Z-m.Z
		sq := dx*dx + dz*dz
		if sq > bestSq {
			continue
		}
		if ctx.HeldItem(p.UUID) == def.FoodItem {
			best, bestSq = p, sq
		}
	}
	return best
}

func (g temptGoal) CanUse(m *Mob, ctx *AIContext) bool { return g.temptingPlayer(m, ctx) != nil }

func (g temptGoal) CanContinue(m *Mob, ctx *AIContext) bool { return g.CanUse(m, ctx) }

func (g temptGoal) Tick(m *Mob, ctx *AIContext) {
	p := g.temptingPlayer(m, ctx)
	if p == nil {
		return
	}
	def := defFor(m.Type)
	dx, dz := p.X-m.X, p.Z-m.Z
	if dx*dx+dz*dz <= 4 { // within 2 b — stop crowding, just watch
		lookAt(m, p.X, p.Y+playerEyeHeight, p.Z, false)
		return
	}
	navigateTo(m, def, ctx, p.X, p.Y, p.Z, def.WanderSpeed)
	lookAt(m, p.X, p.Y+playerEyeHeight, p.Z, false)
}

// randomStrollGoal is the default wander — vanilla RandomStrollGoal /
// WaterAvoidingRandomStrollGoal. Lowest-priority move goal.
type randomStrollGoal struct{ baseGoal }

func (randomStrollGoal) Flags() GoalFlag                       { return FlagMove }
func (randomStrollGoal) CanUse(m *Mob, ctx *AIContext) bool    { return true }
func (randomStrollGoal) CanContinue(m *Mob, ctx *AIContext) bool { return true }

func (randomStrollGoal) Tick(m *Mob, ctx *AIContext) {
	wanderStep(m, defFor(m.Type), ctx)
}

// lookAtPlayerGoal turns the mob's head toward the nearest player within range
// — vanilla LookAtPlayerGoal. Look-only so it runs alongside strolling.
type lookAtPlayerGoal struct {
	baseGoal
	rangeBlocks float64
}

func (lookAtPlayerGoal) Flags() GoalFlag { return FlagLook }

func (g lookAtPlayerGoal) nearest(m *Mob, ctx *AIContext) *PlayerTarget {
	players := ctx.Players()
	var best *PlayerTarget
	bestSq := g.rangeBlocks * g.rangeBlocks
	for i := range players {
		p := &players[i]
		dx, dy, dz := p.X-m.X, p.Y-(m.Y+1.6), p.Z-m.Z
		sq := dx*dx + dy*dy + dz*dz
		if sq <= bestSq {
			best, bestSq = p, sq
		}
	}
	return best
}

func (g lookAtPlayerGoal) CanUse(m *Mob, ctx *AIContext) bool { return g.nearest(m, ctx) != nil }
func (g lookAtPlayerGoal) CanContinue(m *Mob, ctx *AIContext) bool {
	return g.nearest(m, ctx) != nil
}

func (g lookAtPlayerGoal) Tick(m *Mob, ctx *AIContext) {
	if p := g.nearest(m, ctx); p != nil {
		lookAt(m, p.X, p.Y+playerEyeHeight, p.Z, false)
	}
}

// randomLookAroundGoal idly drifts the head when nothing else looks — vanilla
// RandomLookAroundGoal. It only nudges on a cadence so it reads as a glance,
// not a spin. Look-only and lowest priority so lookAtPlayerGoal wins.
type randomLookAroundGoal struct{ baseGoal }

func (randomLookAroundGoal) Flags() GoalFlag                       { return FlagLook }
func (randomLookAroundGoal) CanUse(m *Mob, ctx *AIContext) bool    { return true }
func (randomLookAroundGoal) CanContinue(m *Mob, ctx *AIContext) bool { return true }

func (randomLookAroundGoal) Tick(m *Mob, ctx *AIContext) {
	if m.lookTicks > 0 {
		// Holding a glance: ease the head toward the picked offset.
		m.lookTicks--
		m.HeadYaw = approachAngle(m.HeadYaw, m.lookYawTarget, maxHeadYawTurn)
		return
	}
	// Default: settle the head toward the body heading and level the pitch, so a
	// walking mob looks where it's going instead of staying cocked sideways.
	m.HeadYaw = approachAngle(m.HeadYaw, m.Yaw, maxHeadYawTurn)
	m.HeadPitch = approachAngle(m.HeadPitch, 0, maxHeadPitchTurn)
	// Occasionally pick a brief sideways glance relative to the heading.
	if ctx.RNG.Float64() < 0.02 {
		m.lookYawTarget = m.Yaw + (ctx.RNG.Float64()-0.5)*150.0
		m.lookTicks = 20 + ctx.RNG.Intn(40)
	}
}
