package anticheat

import (
	"fmt"
	"math"
)

// Vanilla movement/combat tolerances. These are deliberately generous starting
// points; the Engine adds lag/TPS compensation on top, and real tuning is gated
// by the false-positive regression corpus (R15).
const (
	walkSpeed   = 4.3 // blocks/sec
	sprintSpeed = 5.6 // blocks/sec
	speedTol    = 1.3 // jump/ice/slime headroom
	reachMax    = 3.0 // survival attack reach
	reachTol    = 0.5
)

// SpeedCheck flags horizontal movement faster than vanilla permits (movement
// family, DESIGN §9).
type SpeedCheck struct{}

func (SpeedCheck) Name() string { return "Speed" }

func (SpeedCheck) Inspect(ctx *PlayerCtx, ev Event) CheckResult {
	m, ok := ev.(*MoveEvent)
	if !ok || m.DT <= 0 {
		return CheckResult{}
	}
	speed := math.Hypot(m.To.X-m.From.X, m.To.Z-m.From.Z) / m.DT
	max := walkSpeed
	if ctx.Sprinting {
		max = sprintSpeed
	}
	if max *= speedTol; speed <= max {
		return CheckResult{}
	}
	return CheckResult{Vio: speed - max, Reason: fmt.Sprintf("hspeed %.2f>%.2f b/s", speed, max), Mitigate: MitigateSetback}
}

// ReachCheck flags melee attacks beyond survival reach (combat family).
type ReachCheck struct{}

func (ReachCheck) Name() string { return "Reach" }

func (ReachCheck) Inspect(ctx *PlayerCtx, ev Event) CheckResult {
	a, ok := ev.(*AttackEvent)
	if !ok {
		return CheckResult{}
	}
	dx := a.TargetPos.X - ctx.Pos.X
	dy := a.TargetPos.Y - (ctx.Pos.Y + ctx.EyeHeight)
	dz := a.TargetPos.Z - ctx.Pos.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if max := reachMax + reachTol; dist > max {
		return CheckResult{Vio: dist - max, Reason: fmt.Sprintf("reach %.2f>%.2f", dist, max), Mitigate: MitigateCancel}
	}
	return CheckResult{}
}


const (
	timerMaxRatio = 1.2  // client-time / real-time speedup tolerated (jitter/burst)
	maxCPS        = 20.0 // attacks/sec before suspicion
	maxAimAngle   = 80.0 // degrees off-crosshair an attack may still land
	deg2rad       = math.Pi / 180
	rad2deg       = 180 / math.Pi
)

// TimerCheck flags clients whose movement time outruns wall-clock time
// (packet/timing family) — the classic "timer" speed hack.
type TimerCheck struct{}

func (TimerCheck) Name() string { return "Timer" }

func (TimerCheck) Inspect(_ *PlayerCtx, ev Event) CheckResult {
	m, ok := ev.(*MoveEvent)
	if !ok || m.DT <= 0 || m.RealDT <= 0 {
		return CheckResult{}
	}
	if ratio := m.DT / m.RealDT; ratio > timerMaxRatio {
		return CheckResult{Vio: ratio - timerMaxRatio, Reason: fmt.Sprintf("timer %.2fx", ratio)}
	}
	return CheckResult{}
}

// AutoclickerCheck flags inhuman click rates (combat family). Click-interval
// regularity/variance needs history and is a follow-up.
type AutoclickerCheck struct{}

func (AutoclickerCheck) Name() string { return "Autoclicker" }

func (AutoclickerCheck) Inspect(_ *PlayerCtx, ev Event) CheckResult {
	a, ok := ev.(*AttackEvent)
	if !ok || a.Interval <= 0 {
		return CheckResult{}
	}
	if cps := 1 / a.Interval; cps > maxCPS {
		return CheckResult{Vio: cps - maxCPS, Reason: fmt.Sprintf("cps %.1f", cps)}
	}
	return CheckResult{}
}

// KillAuraCheck flags attacks landed well outside the player's view direction
// (combat/aim family), e.g. hitting a target behind them. Snap/GCD aim analysis
// needs rotation history and is a follow-up.
type KillAuraCheck struct{}

func (KillAuraCheck) Name() string { return "KillAura" }

func (KillAuraCheck) Inspect(ctx *PlayerCtx, ev Event) CheckResult {
	a, ok := ev.(*AttackEvent)
	if !ok {
		return CheckResult{}
	}
	dx := a.TargetPos.X - ctx.Pos.X
	dy := a.TargetPos.Y - (ctx.Pos.Y + ctx.EyeHeight)
	dz := a.TargetPos.Z - ctx.Pos.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if dist == 0 {
		return CheckResult{}
	}
	yaw, pitch := ctx.Yaw*deg2rad, ctx.Pitch*deg2rad
	lx := -math.Cos(pitch) * math.Sin(yaw) // look unit vector (MC yaw/pitch)
	ly := -math.Sin(pitch)
	lz := math.Cos(pitch) * math.Cos(yaw)
	dot := (lx*dx + ly*dy + lz*dz) / dist // cos(angle); look is unit length
	if dot > 1 {
		dot = 1
	} else if dot < -1 {
		dot = -1
	}
	if angle := math.Acos(dot) * rad2deg; angle > maxAimAngle {
		return CheckResult{Vio: (angle - maxAimAngle) / 30, Reason: fmt.Sprintf("aim %.0f deg", angle), Mitigate: MitigateCancel}
	}
	return CheckResult{}
}
