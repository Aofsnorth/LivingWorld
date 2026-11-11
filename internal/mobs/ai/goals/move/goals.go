// Package aimovegoal owns the vanilla move and idle behaviour
// goals: float-in-water, panic-on-hurt, follow-food, and the
// default wander. They occupy FlagMove and/or FlagLook and run on
// the mob's GoalSel.
package aimovegoal

import (
	"livingworld/internal/mobs"
	context "livingworld/internal/mobs/ai/context"
	movement "livingworld/internal/mobs/ai/movement"
	selector "livingworld/internal/mobs/ai/selector"
	"math"
)

// FloatGoal keeps the mob's head above water — vanilla FloatGoal.
// While the feet cell is water it forces a jump every tick so
// the mob bobs to the surface instead of sinking. Occupies
// FlagJump so it doesn't fight move goals.
type FloatGoal struct {
	selector.BaseGoal
}

func (FloatGoal) Flags() selector.Flag { return selector.FlagJump }

func (FloatGoal) CanUse(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	if ctx.WaterAt == nil {
		return false
	}
	// Vanilla FloatGoal checks fluidHeight > getFluidJumpThreshold() (~0.4 for
	// normal mobs, ~0.25 for short/baby mobs) so the mob's body remains submerged
	// in the top water block with its head above the surface instead of standing
	// on top of the water surface block.
	submergeOffset := 0.55
	if movement.MobEyeHeightFor(m) < 0.9 {
		submergeOffset = 0.42
	}
	fx, fz := int(math.Floor(m.X)), int(math.Floor(m.Z))
	return ctx.WaterAt(fx, int(math.Floor(m.Y+submergeOffset)), fz)
}

func (FloatGoal) CanContinue(body any, ctx *context.AIContext) bool {
	return FloatGoal{}.CanUse(body, ctx)
}

func (FloatGoal) Tick(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	if m.Vy < 0.04 {
		m.Vy += 0.035
		if m.Vy > 0.04 {
			m.Vy = 0.04
		}
	}
	m.OnGround = false
}

// PanicGoal makes a passive mob sprint to random nearby waypoints when hurt —
// matching vanilla PanicGoal (100 ticks hurtByTimestamp window, 1.25x-1.4x speed,
// LandRandomPos(5, 4) waypoint selection).
type PanicGoal struct {
	selector.BaseGoal
}

func (PanicGoal) Flags() selector.Flag { return selector.FlagMove | selector.FlagLook }

func (PanicGoal) CanUse(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	if m.FireTicks > 0 {
		return true
	}
	if m.HurtBy == context.ZeroUUID() {
		return false
	}
	return m.AITick-m.HurtByTick <= PanicDurationTicks
}

func (PanicGoal) CanContinue(body any, ctx *context.AIContext) bool {
	return PanicGoal{}.CanUse(body, ctx)
}

// PanicDurationTicks is the vanilla passive panic window (100 ticks = 5 s).
const PanicDurationTicks = 100

// PanicSpeedMul is the vanilla PanicGoal speed modifier (1.35x wander speed).
const PanicSpeedMul = 1.35

func (PanicGoal) Start(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	m.WalkTicks = 0
}

func (PanicGoal) Tick(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	def := mobs.DefFor(m.Type)
	if p := context.FindPlayer(ctx.Players(), m.HurtBy); p != nil {
		movement.FleeFrom(m, def, ctx, p.X, p.Z, def.WanderSpeed*PanicSpeedMul)
	} else {
		movement.FleeFrom(m, def, ctx, m.X-math.Sin(m.Yaw*math.Pi/180), m.Z+math.Cos(m.Yaw*math.Pi/180), def.WanderSpeed*PanicSpeedMul)
	}
}

func (PanicGoal) Stop(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	if m.AITick-m.HurtByTick > PanicDurationTicks {
		m.HurtBy = context.ZeroUUID()
	}
}

// TemptGoal makes a passive mob follow a nearby player holding
// its preferred food — vanilla TemptGoal. Occupies move+look.
type TemptGoal struct {
	selector.BaseGoal
}

func (TemptGoal) Flags() selector.Flag { return selector.FlagMove | selector.FlagLook }

// TemptRangeSq is the squared distance within which the food lure
// works (8 b).
const TemptRangeSq = 64.0

func (TemptGoal) temptingPlayer(m *mobs.Mob, ctx *context.AIContext) *context.PlayerTarget {
	def := mobs.DefFor(m.Type)
	if def.FoodItem == "" || ctx.HeldItem == nil {
		return nil
	}
	players := ctx.Players()
	var best *context.PlayerTarget
	bestSq := TemptRangeSq
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

func (TemptGoal) CanUse(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	return TemptGoal{}.temptingPlayer(m, ctx) != nil
}

func (TemptGoal) CanContinue(body any, ctx *context.AIContext) bool {
	return TemptGoal{}.CanUse(body, ctx)
}

func (TemptGoal) Tick(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	p := TemptGoal{}.temptingPlayer(m, ctx)
	if p == nil {
		return
	}
	def := mobs.DefFor(m.Type)
	dx, dz := p.X-m.X, p.Z-m.Z
	if dx*dx+dz*dz <= 4 {
		movement.LookAt(m, p.X, p.Y+movement.PlayerEyeHeight, p.Z, false)
		return
	}
	movement.NavigateTo(m, def, ctx, p.X, p.Y, p.Z, def.WanderSpeed)
	movement.LookAt(m, p.X, p.Y+movement.PlayerEyeHeight, p.Z, false)
}

// RandomStrollGoal is the default wander — vanilla
// RandomStrollGoal / WaterAvoidingRandomStrollGoal. Lowest-
// priority move goal.
type RandomStrollGoal struct {
	selector.BaseGoal
}

func (RandomStrollGoal) Flags() selector.Flag { return selector.FlagMove }
func (RandomStrollGoal) CanUse(body any, ctx *context.AIContext) bool {
	_, ok := body.(*mobs.Mob)
	return ok
}
func (RandomStrollGoal) CanContinue(body any, ctx *context.AIContext) bool {
	return true
}

func (RandomStrollGoal) Tick(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	movement.WanderStep(m, mobs.DefFor(m.Type), ctx)
}
