// Package aibraingoal owns the brain-driven target goals — the
// stateful half of the AI (Sensor → Memory → Task). Where
// NearestAttackableTargetGoal is a stateless scan, these goals
// remember provocation across ticks.
package aibraingoal

import (
	"livingworld/internal/mobs"
	brain "livingworld/internal/mobs/ai/brain"
	context "livingworld/internal/mobs/ai/context"
	selector "livingworld/internal/mobs/ai/selector"
	"math"
)

// GazeAngerTicks is how long a provoked enderman stays hostile
// to its target. 30 s at 20 Hz.
const GazeAngerTicks = 600

// GazeConeCos is cos(threshold): a player counts as "staring"
// when the dot of their normalised look vector with the
// direction to the enderman's head is at least this. ~0.985 ≈
// 10°, matching vanilla's narrow stare cone.
const GazeConeCos = 0.985

// GazeMaxDist is the range within which a stare can provoke.
const GazeMaxDist = 64.0

// EndermanGazeGoal acquires a target only when a player looks at
// the mob, then holds that target via the brain anger memory.
type EndermanGazeGoal struct {
	selector.BaseGoal
}

func (EndermanGazeGoal) Flags() selector.Flag { return selector.FlagTarget }

func (EndermanGazeGoal) CanUse(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	if u, ok := m.Brain.GetUUID(brain.MemAngerTarget, m.AITick); ok {
		if context.FindPlayer(ctx.Players(), u) != nil {
			m.Target = u
			return true
		}
		m.Brain.Clear(brain.MemAngerTarget)
	}
	if p := StaringPlayer(m, ctx); p != nil {
		m.Target = p.UUID
		m.Brain.SetFor(brain.MemAngerTarget, p.UUID, m.AITick, GazeAngerTicks)
		return true
	}
	return false
}

func (EndermanGazeGoal) CanContinue(body any, ctx *context.AIContext) bool {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return false
	}
	if m.Target == context.ZeroUUID() {
		return false
	}
	p := context.FindPlayer(ctx.Players(), m.Target)
	if p == nil || p.Gamemode == 1 || p.Gamemode == 3 {
		return false
	}
	if m.Brain.Has(brain.MemAngerTarget, m.AITick) {
		if StaringPlayer(m, ctx) != nil {
			m.Brain.SetFor(brain.MemAngerTarget, m.Target, m.AITick, GazeAngerTicks)
		}
		return true
	}
	return false
}

func (EndermanGazeGoal) Stop(body any, ctx *context.AIContext) {
	m, ok := body.(*mobs.Mob)
	if !ok {
		return
	}
	if m.AITick-m.HurtByTick > 100 {
		m.Target = context.ZeroUUID()
		m.Brain.Clear(brain.MemAngerTarget)
	}
}

func (EndermanGazeGoal) Tick(body any, ctx *context.AIContext) {}

// StaringPlayer returns the nearest player whose view cone is on
// the mob's head with line of sight, or nil. This is the brain's
// "gaze sensor".
func StaringPlayer(m *mobs.Mob, ctx *context.AIContext) *context.PlayerTarget {
	players := ctx.Players()
	var best *context.PlayerTarget
	bestDist := GazeMaxDist
	hx, hy, hz := m.X, m.Y+1.5, m.Z
	for i := range players {
		p := &players[i]
		if p.Gamemode == 1 || p.Gamemode == 3 {
			continue
		}
		ex, ey, ez := p.X, p.Y+1.6, p.Z
		dx, dy, dz := hx-ex, hy-ey, hz-ez
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if dist == 0 || dist > bestDist {
			continue
		}
		yaw := p.LookYaw * math.Pi / 180
		pitch := p.LookPitch * math.Pi / 180
		lx := -math.Sin(yaw) * math.Cos(pitch)
		ly := -math.Sin(pitch)
		lz := math.Cos(yaw) * math.Cos(pitch)
		dot := (dx*lx + dy*ly + dz*lz) / dist
		if dot < GazeConeCos {
			continue
		}
		if !context.HasLineOfSight(ex, ey, ez, hx, hy, hz, ctx.SolidAt) {
			continue
		}
		best, bestDist = p, dist
	}
	return best
}
