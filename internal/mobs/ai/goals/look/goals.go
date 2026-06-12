// Package ailookgoal owns the look-behaviour goals:
// LookAtPlayerGoal and RandomLookAroundGoal. They occupy FlagLook
// and run on the mob's GoalSel alongside (not against) move goals.
package ailookgoal

import (
	"livingworld/internal/mobs"
	context "livingworld/internal/mobs/ai/context"
	movement "livingworld/internal/mobs/ai/movement"
	selector "livingworld/internal/mobs/ai/selector"
)

// LookAtPlayerGoal turns the mob's head toward the nearest player
// within range — vanilla LookAtPlayerGoal. Look-only so it runs
// alongside strolling.
//
// Vanilla behaviour: canUse has a 2% probability gate per tick
// (so the mob doesn't stare 100% of the time); on start, a random
// look duration of 40–80 ticks (2–4 s) is chosen.
type LookAtPlayerGoal struct {
	selector.BaseGoal
	RangeBlocks float64
}

func (LookAtPlayerGoal) Flags() selector.Flag { return selector.FlagLook }

// LookAtPlayerChance is the vanilla per-tick probability that a
// mob begins looking at a nearby player.
const LookAtPlayerChance = 0.02

func (g LookAtPlayerGoal) nearest(m *mobs.Mob, ctx *context.AIContext) *context.PlayerTarget {
	players := ctx.Players()
	var best *context.PlayerTarget
	bestSq := g.RangeBlocks * g.RangeBlocks
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

func (g LookAtPlayerGoal) CanUse(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	if ctx.RNG.Float64() >= LookAtPlayerChance {
		return false
	}
	return g.nearest(m, ctx) != nil
}

func (g LookAtPlayerGoal) CanContinue(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	if m.LookTicks <= 0 {
		return false
	}
	return g.nearest(m, ctx) != nil
}

func (g LookAtPlayerGoal) Start(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	m.LookTicks = 40 + ctx.RNG.Intn(40)
}

func (g LookAtPlayerGoal) Stop(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	m.LookTicks = 0
}

func (g LookAtPlayerGoal) Tick(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	m.LookTicks--
	if p := g.nearest(m, ctx); p != nil {
		movement.LookAt(m, p.X, p.Y+movement.PlayerEyeHeight, p.Z, false)
	}
}

// RandomLookAroundGoal idly drifts the head when nothing else
// looks — vanilla RandomLookAroundGoal. Has a 2% per-tick chance
// to start, then holds a random glance for 20–60 ticks.
type RandomLookAroundGoal struct {
	selector.BaseGoal
}

func (RandomLookAroundGoal) Flags() selector.Flag { return selector.FlagLook }

func (RandomLookAroundGoal) CanUse(body any, ctx *context.AIContext) bool {
	_, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	return ctx.RNG.Float64() < 0.02
}

func (RandomLookAroundGoal) CanContinue(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	return m.LookTicks > 0
}

func (RandomLookAroundGoal) Start(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	m.LookYawTarget = m.Yaw + (ctx.RNG.Float64()-0.5)*150.0
	m.LookTicks = 20 + ctx.RNG.Intn(40)
}

func (RandomLookAroundGoal) Stop(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	m.LookTicks = 0
}

func (RandomLookAroundGoal) Tick(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	m.LookTicks--
	m.HeadYaw = movement.ApproachAngle(m.HeadYaw, m.LookYawTarget, movement.MaxHeadYawTurn)
	m.HeadPitch = movement.ApproachAngle(m.HeadPitch, 0, movement.MaxHeadPitchTurn)
}
