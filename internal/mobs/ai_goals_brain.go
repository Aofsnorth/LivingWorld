package mobs

import "math"

// Brain-driven target goals — the stateful half of the AI the spec calls out
// (Sensor → Memory → Task). Where nearestAttackableTargetGoal is a stateless
// scan, these goals remember provocation across ticks: an enderman stays angry
// after you stop staring; a neutral mob keeps chasing its attacker for a fixed
// window. The memory lives in the mob's aiBrain (MemAngerTarget/MemAngerTick).

// gazeAngerTicks is how long a provoked enderman stays hostile to its target
// (vanilla endermen remain angry until the target dies or escapes; we cap it
// so a lost target eventually lapses). 30 s at 20 Hz.
const gazeAngerTicks = 600

// gazeConeCos is cos(threshold): a player counts as "staring" when the dot of
// their normalised look vector with the direction to the enderman's head is at
// least this. ~0.985 ≈ 10°, matching vanilla's narrow stare cone.
const gazeConeCos = 0.985

// gazeMaxDist is the range within which a stare can provoke (vanilla 64 b).
const gazeMaxDist = 64.0

// endermanGazeGoal acquires a target only when a player looks at the mob, then
// holds that target via the brain anger memory. Replaces
// nearestAttackableTargetGoal for AggravatedByGaze mobs (enderman).
type endermanGazeGoal struct{ baseGoal }

func (endermanGazeGoal) Flags() GoalFlag { return FlagTarget }

func (g endermanGazeGoal) CanUse(m *Mob, ctx *AIContext) bool {
	// Already angry at a still-valid target → keep it (memory persistence).
	if u, ok := m.brain.getUUID(MemAngerTarget, m.aiTick); ok {
		if findPlayer(ctx.Players(), u) != nil {
			m.target = u
			return true
		}
		m.brain.clear(MemAngerTarget)
	}
	// Otherwise look for a fresh starer.
	if p := g.staringPlayer(m, ctx); p != nil {
		m.target = p.UUID
		m.brain.setFor(MemAngerTarget, p.UUID, m.aiTick, gazeAngerTicks)
		return true
	}
	return false
}

func (g endermanGazeGoal) CanContinue(m *Mob, ctx *AIContext) bool {
	if m.target == zero16() {
		return false
	}
	p := findPlayer(ctx.Players(), m.target)
	if p == nil || p.Gamemode == 1 || p.Gamemode == 3 {
		return false
	}
	// Anger memory keeps the target alive even when the player looks away.
	if m.brain.has(MemAngerTarget, m.aiTick) {
		// Refresh the timer while the player keeps staring.
		if g.staringPlayer(m, ctx) != nil {
			m.brain.setFor(MemAngerTarget, m.target, m.aiTick, gazeAngerTicks)
		}
		return true
	}
	return false
}

func (endermanGazeGoal) Stop(m *Mob, ctx *AIContext) {
	if m.aiTick-m.hurtByTick > hurtByForgetTicks {
		m.target = zero16()
		m.brain.clear(MemAngerTarget)
	}
}

func (endermanGazeGoal) Tick(m *Mob, ctx *AIContext) {}

// staringPlayer returns the nearest player whose view cone is on the mob's head
// with line of sight, or nil. This is the brain's "gaze sensor".
func (g endermanGazeGoal) staringPlayer(m *Mob, ctx *AIContext) *PlayerTarget {
	players := ctx.Players()
	var best *PlayerTarget
	bestDist := gazeMaxDist
	// Aim at the enderman's upper body (feet + 1.5) rather than the very top
	// of the 2.9 b head: a player looking level at a nearby enderman should
	// still register as a stare. The cone (gazeConeCos) does the rest.
	hx, hy, hz := m.X, m.Y+1.5, m.Z
	for i := range players {
		p := &players[i]
		if p.Gamemode == 1 || p.Gamemode == 3 {
			continue
		}
		ex, ey, ez := p.X, p.Y+1.6, p.Z // player eye
		dx, dy, dz := hx-ex, hy-ey, hz-ez
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist == 0 || dist > bestDist {
			continue
		}
		// Player look vector from yaw/pitch (Minecraft convention).
		yaw := p.LookYaw * math.Pi / 180
		pitch := p.LookPitch * math.Pi / 180
		lx := -math.Sin(yaw) * math.Cos(pitch)
		ly := -math.Sin(pitch)
		lz := math.Cos(yaw) * math.Cos(pitch)
		// Dot of the (normalised) eye→head direction with the look vector.
		dot := (dx*lx + dy*ly + dz*lz) / dist
		if dot < gazeConeCos {
			continue
		}
		if !hasLineOfSight(ex, ey, ez, hx, hy, hz, ctx.SolidAt) {
			continue
		}
		best, bestDist = p, dist
	}
	return best
}
