// Package aitargetgoal owns the target-acquisition goals. They
// live on the mob's TargetSel and occupy FlagTarget. They
// set/clear m.Target; the behaviour goals (melee/ranged/swell)
// read m.Target to decide what to do.
package aitargetgoal

import (
	"livingworld/internal/mobs"
	brain "livingworld/internal/mobs/ai/brain"
	context "livingworld/internal/mobs/ai/context"
	selector "livingworld/internal/mobs/ai/selector"
	"math"
)

// HurtByTargetGoal makes a mob retaliate against whoever last
// damaged it. Vanilla HurtByTargetGoal: the target is the last
// attacker, valid for a short window after the hit.
type HurtByTargetGoal struct {
	selector.BaseGoal
}

// HurtByForgetTicks is how long after a hit the retaliation
// interest lasts.
const HurtByForgetTicks = 100

func (HurtByTargetGoal) Flags() selector.Flag { return selector.FlagTarget }

func (HurtByTargetGoal) CanUse(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	if m.HurtBy == context.ZeroUUID() {
		return false
	}
	if m.AITick-m.HurtByTick > HurtByForgetTicks {
		return false
	}
	return context.FindPlayer(ctx.Players(), m.HurtBy) != nil
}

func (HurtByTargetGoal) CanContinue(body any, ctx *context.AIContext) bool {
	return HurtByTargetGoal{}.CanUse(body, ctx)
}

func (HurtByTargetGoal) Start(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	m.Target = m.HurtBy
	m.Brain.Set(brain.MemAttackTarget, m.Target)
}

func (HurtByTargetGoal) Stop(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	if m.AITick-m.HurtByTick > HurtByForgetTicks {
		m.Target = context.ZeroUUID()
	}
	m.Brain.Clear(brain.MemHurtBy)
}

func (HurtByTargetGoal) Tick(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	m.Target = m.HurtBy
}

// NearestAttackableTargetGoal acquires the nearest valid player
// within the mob's (modifier-adjusted) follow range with
// line-of-sight. This is the vanilla NearestAttackableTargetGoal +
// the detection-range modifiers:
//
//	range = follow_range × sneak(0.8) × invisible(0.07) × head(0.5)
type NearestAttackableTargetGoal struct {
	selector.BaseGoal
}

func (NearestAttackableTargetGoal) Flags() selector.Flag { return selector.FlagTarget }

func (NearestAttackableTargetGoal) CanUse(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	def := mobs.DefFor(m.Type)
	tgt, ok := DetectTarget(m, def, ctx.Players(), ctx)
	if !ok {
		return false
	}
	m.Target = tgt
	return true
}

func (NearestAttackableTargetGoal) CanContinue(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	if m.Target == context.ZeroUUID() {
		return false
	}
	def := mobs.DefFor(m.Type)
	p := context.FindPlayer(ctx.Players(), m.Target)
	if p == nil || p.Gamemode == 1 || p.Gamemode == 3 {
		return false
	}
	dx, dz := p.X-m.X, p.Z-m.Z
	maxSq := (def.FollowRange + 4) * (def.FollowRange + 4)
	return dx*dx+dz*dz <= maxSq
}

func (NearestAttackableTargetGoal) Stop(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	if m.AITick-m.HurtByTick > HurtByForgetTicks {
		m.Target = context.ZeroUUID()
		m.Brain.Clear(brain.MemAttackTarget)
	}
}

func (NearestAttackableTargetGoal) Tick(body any, ctx *context.AIContext) {}

// DetectTarget is the vanilla detection scan: nearest valid player
// inside the modifier-adjusted follow range with eye-to-eye line
// of sight.
func DetectTarget(m *mobs.Mob, def mobs.MobDef, players []context.PlayerTarget, ctx *context.AIContext) ([16]byte, bool) {
	if len(players) == 0 || ctx.SolidAt == nil {
		return context.ZeroUUID(), false
	}
	if def.AggressiveAtNight && ctx.IsDay != nil && ctx.IsDay() {
		return context.ZeroUUID(), false
	}
	bestDist := math.MaxFloat64
	var best [16]byte
	mx, my, mz := m.X, m.Y+1.0, m.Z
	for i := range players {
		p := &players[i]
		if p.Gamemode == 1 || p.Gamemode == 3 {
			continue
		}
		if def.AggressiveUnlessGold && p.WearingGold {
			continue
		}
		mod := 1.0
		if p.Sneaking {
			mod *= 0.8
		}
		if p.Invisible {
			mod *= 0.07
		}
		switch p.WearingHead {
		case "minecraft:zombie", "minecraft:skeleton", "minecraft:creeper":
			mod *= 0.5
		}
		dist := math.Hypot(p.X-mx, math.Hypot(p.Y-my, p.Z-mz))
		if dist > def.FollowRange*mod {
			continue
		}
		if !context.HasLineOfSight(mx, my, mz, p.X, p.Y+1.0, p.Z, ctx.SolidAt) {
			continue
		}
		if dist < bestDist {
			bestDist = dist
			best = p.UUID
		}
	}
	return best, best != context.ZeroUUID()
}
