package mobs

import "math"

// Target-acquisition goals. These live on the mob's targetSelector and occupy
// FlagTarget. They set/clear m.target; the behaviour goals (melee/ranged/swell)
// read m.target to decide what to do. Splitting acquisition from execution
// mirrors vanilla's Mob.targetSelector vs Mob.goalSelector.

// hurtByTargetGoal makes a mob retaliate against whoever last damaged it.
// Vanilla HurtByTargetGoal: the target is the last attacker, valid for a short
// window after the hit. Highest-priority target goal so a fresh attacker
// overrides the current chase (e.g. a zombie chasing player A switches to
// player B who just hit it).
type hurtByTargetGoal struct {
	baseGoal
}

// hurtByForgetTicks is how long after a hit the retaliation interest lasts
// (vanilla ~100 ticks before the anger memory lapses without a fresh hit).
const hurtByForgetTicks = 100

func (g hurtByTargetGoal) Flags() GoalFlag { return FlagTarget }

func (g hurtByTargetGoal) CanUse(m *Mob, ctx *AIContext) bool {
	if m.hurtBy == zero16() {
		return false
	}
	if m.aiTick-m.hurtByTick > hurtByForgetTicks {
		return false
	}
	// The attacker must still be a live, valid player.
	return findPlayer(ctx.Players(), m.hurtBy) != nil
}

func (g hurtByTargetGoal) CanContinue(m *Mob, ctx *AIContext) bool {
	return g.CanUse(m, ctx)
}

func (g hurtByTargetGoal) Start(m *Mob, ctx *AIContext) {
	m.target = m.hurtBy
	m.brain.set(MemAttackTarget, m.target)
}

func (g hurtByTargetGoal) Stop(m *Mob, ctx *AIContext) {
	// Don't clear m.target here — nearestAttackableTargetGoal may still want
	// it. Just drop the brain note; the behaviour goals re-validate target.
	m.brain.clear(MemHurtBy)
}

func (g hurtByTargetGoal) Tick(m *Mob, ctx *AIContext) {
	// Keep the lock fresh while the attacker is valid.
	m.target = m.hurtBy
}

// nearestAttackableTargetGoal acquires the nearest valid player within the
// mob's (modifier-adjusted) follow range with line-of-sight. This is the
// vanilla NearestAttackableTargetGoal + the detection-range modifiers from the
// pre-rombak pickTarget:
//
//	range = follow_range × sneak(0.8) × invisible(0.07) × head(0.5)
//
// Per-mob gates preserved: AggressiveAtNight (spider neutral by day),
// AggressiveUnlessGold (piglin ignores gold-armored players), gamemode skip
// (creative/spectator never targeted).
type nearestAttackableTargetGoal struct {
	baseGoal
}

func (g nearestAttackableTargetGoal) Flags() GoalFlag { return FlagTarget }

func (g nearestAttackableTargetGoal) CanUse(m *Mob, ctx *AIContext) bool {
	def := defFor(m.Type)
	tgt, ok := detectTarget(m, def, ctx.Players(), ctx)
	if !ok {
		return false
	}
	m.target = tgt
	return true
}

func (g nearestAttackableTargetGoal) CanContinue(m *Mob, ctx *AIContext) bool {
	if m.target == zero16() {
		return false
	}
	def := defFor(m.Type)
	// Keep chasing while the target is alive and within an extended range
	// (vanilla keeps the target until it leaves follow_range or LOS is lost
	// for a sustained period). We re-validate presence + a generous range.
	p := findPlayer(ctx.Players(), m.target)
	if p == nil || p.Gamemode == 1 || p.Gamemode == 3 {
		return false
	}
	dx, dz := p.X-m.X, p.Z-m.Z
	maxSq := (def.FollowRange + 4) * (def.FollowRange + 4)
	return dx*dx+dz*dz <= maxSq
}

func (g nearestAttackableTargetGoal) Stop(m *Mob, ctx *AIContext) {
	// Released the target — let the behaviour goals fall back to wander.
	// Only clear if hurtByTargetGoal isn't holding a fresh attacker.
	if m.aiTick-m.hurtByTick > hurtByForgetTicks {
		m.target = zero16()
		m.brain.clear(MemAttackTarget)
	}
}

func (g nearestAttackableTargetGoal) Tick(m *Mob, ctx *AIContext) {}

// detectTarget is the vanilla detection scan: nearest valid player inside the
// modifier-adjusted follow range with eye-to-eye line of sight. Ported from the
// pre-rombak pickTarget so behaviour is preserved bit-for-bit.
func detectTarget(m *Mob, def MobDef, players []PlayerTarget, ctx *AIContext) ([16]byte, bool) {
	if len(players) == 0 || ctx.SolidAt == nil {
		return zero16(), false
	}
	// Spider daytime gate: no fresh targets during the day (retaliation via
	// hurtByTargetGoal still works).
	if def.AggressiveAtNight && ctx.IsDay != nil && ctx.IsDay() {
		return zero16(), false
	}

	bestDist := math.MaxFloat64
	var best [16]byte
	mx, my, mz := m.X, m.Y+1.0, m.Z // mob eye

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
