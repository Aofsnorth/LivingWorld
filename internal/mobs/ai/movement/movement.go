// Package aimovement owns the shared movement primitives used by the
// behaviour goals. These own the "translate a desired direction into
// a physically-valid step" logic that the old monolithic state
// machine folded in: path following, step-up jumps, door breaking,
// wall climbing, water/baby speed scaling, and stuck recovery.
//
// Goals decide WHERE to go; the helpers here decide HOW the body
// gets there. The package also owns the head/body rotation math
// (lookAt, approachAngle, setBodyYaw) and the per-mob eye-height
// table used to compute look pitch.
package aimovement

import (
	"livingworld/internal/mobs"
	// Alias the context import to `context` so the function
	// signatures read as `*context.AIContext`.
	context "livingworld/internal/mobs/ai/context"
	"math"
)

// Head look limits. lookAt eases HeadYaw/HeadPitch toward the target
// by at most these many degrees per tick instead of snapping, so a
// mob's head tracks a player smoothly (vanilla LookControl clamps
// head rotation the same way).
const (
	MaxHeadYawTurn   = 10.0 // vanilla getMaxHeadYRot() default — 10°/tick
	MaxHeadPitchTurn = 40.0 // vanilla getMaxHeadXRot() default — 40°/tick
	MaxHeadBodyYaw   = 75.0
	// BodyFollowDelay is the number of stationary ticks before the
	// body starts rotating toward the head yaw (vanilla
	// BodyRotationControl threshold).
	BodyFollowDelay = 10
	// MobEyeHeight is the fallback eye height used to compute look
	// pitch when a mob type has no entry in MobEyeHeightFor.
	MobEyeHeight = 1.6
	// PlayerEyeHeight is where a look goal aims on the target player.
	PlayerEyeHeight = 1.62
)

// SpeedToBlocksPerTick converts a MobDef's movement-speed attribute
// (WanderSpeed / ChaseSpeed, in vanilla attribute units) into the
// per-tick horizontal displacement the integrator applies.
const SpeedToBlocksPerTick = 0.62

// MobEyeHeightFor returns the approximate eye height (blocks above
// feet) for a mob type, used as the origin when computing look pitch.
func MobEyeHeightFor(m *mobs.Mob) float64 {
	var h float64
	switch m.Type {
	case "minecraft:chicken":
		h = 0.64
	case "minecraft:pig", "minecraft:sheep":
		h = 0.77
	case "minecraft:cow":
		h = 1.05
	case "minecraft:slime", "minecraft:magma_cube":
		h = 0.7 * float64(MaxInt(m.Size, 1))
	case "minecraft:spider", "minecraft:cave_spider":
		h = 0.65
	case "minecraft:enderman":
		h = 2.55
	case "minecraft:iron_golem":
		h = 2.3
	case "minecraft:zombie", "minecraft:husk", "minecraft:zombie_villager",
		"minecraft:drowned", "minecraft:skeleton", "minecraft:stray",
		"minecraft:bogged", "minecraft:wither_skeleton", "minecraft:creeper",
		"minecraft:piglin", "minecraft:witch":
		h = 1.74
	default:
		h = MobEyeHeight
	}
	if m.Baby {
		h *= 0.5
	}
	return h
}

// MaxInt returns the larger of a and b.
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// JumpMob is the vanilla jump-strength injection. Callers decide
// *when* to jump; this is the *how* (one-tick impulse, fixed
// cooldown).
func JumpMob(m *mobs.Mob) {
	if m.OnGround && m.JumpCooldown == 0 {
		m.Vy = 0.42
		m.JumpCooldown = 6
	}
}

// NavigateTo advances the mob one tick toward (tx, ty, tz) at the
// given attribute speed, following a cached A* path that is
// replanned every 20 ticks or when the goal cell drifts. Returns
// the horizontal distance remaining to the goal so callers can
// decide arrival.
func NavigateTo(m *mobs.Mob, def mobs.MobDef, ctx *context.AIContext, tx, ty, tz, speed float64) float64 {
	mobCell := mobs.PathNode{X: int(math.Floor(m.X)), Y: int(math.Floor(m.Y)), Z: int(math.Floor(m.Z))}
	goalCell := mobs.PathNode{X: int(math.Floor(tx)), Y: int(math.Floor(ty)), Z: int(math.Floor(tz))}
	if !m.HasPath() || m.PathGoalStale(goalCell) || m.PathTick == 0 {
		m.ReplanPath(mobCell, goalCell, *ctx)
	}
	var dx, dz float64
	mx, mz := int(math.Floor(m.X)), int(math.Floor(m.Z))
	if pdx, pdz, py, ok := m.PathDirection(mx, mz); ok {
		dx, dz = pdx, pdz
		// Next waypoint is elevated and we're grounded → jump.
		if py > int(math.Floor(m.Y)) && m.OnGround && m.JumpCooldown == 0 {
			JumpMob(m)
		}
	} else {
		dx, dz = tx-m.X, tz-m.Z
	}
	StepHorizontal(m, def, ctx, dx, dz, speed, true)
	return math.Hypot(tx-m.X, tz-m.Z)
}

// FleeFrom advances the mob one tick directly away from (fx, fz) at
// the given speed. No pathing — panic is a straight dash (vanilla
// PanicGoal beelines and re-rolls a target tile, but a direct
// retreat reads identically at v1 scale).
func FleeFrom(m *mobs.Mob, def mobs.MobDef, ctx *context.AIContext, fx, fz, speed float64) {
	dx, dz := m.X-fx, m.Z-fz
	if dx == 0 && dz == 0 {
		dx, dz = math.Sin(m.Yaw*math.Pi/180), math.Cos(m.Yaw*math.Pi/180)
	}
	StepHorizontal(m, def, ctx, dx, dz, speed, true)
}

// StepHorizontal normalises (dx, dz), scales it to a per-tick
// delta, and tries to step. If blocked it attempts (in order) a
// door-break, a step-up jump, then counts the step as stuck
// (turning 90° after 3 consecutive blocked ticks).
func StepHorizontal(m *mobs.Mob, def mobs.MobDef, ctx *context.AIContext, dx, dz, speed float64, face bool) {
	if def.Movement == "fly" {
		dl := math.Hypot(dx, dz)
		if dl == 0 {
			return
		}
		step := speed * SpeedToBlocksPerTick
		m.X += dx / dl * step
		m.Z += dz / dl * step
		if face {
			SetBodyYaw(m, math.Atan2(-dx, dz)*180/math.Pi)
		}
		return
	}
	dl := math.Hypot(dx, dz)
	if dl == 0 {
		return
	}
	step := speed * SpeedToBlocksPerTick
	if ctx.WaterAt != nil {
		fx, fy, fz := int(math.Floor(m.X)), int(math.Floor(m.Y)), int(math.Floor(m.Z))
		if ctx.WaterAt(fx, fy, fz) {
			step *= 0.5
		}
	}
	if m.Baby {
		step *= 1.5
	}
	nx := m.X + dx/dl*step
	nz := m.Z + dz/dl*step
	if face {
		SetBodyYaw(m, math.Atan2(-dx, dz)*180/math.Pi)
	}
	if def.Movement == "climb" {
		cx, cz := int(math.Floor(nx)), int(math.Floor(nz))
		fy := int(math.Floor(m.Y))
		if ctx.SolidAt(cx, fy, cz) || ctx.SolidAt(cx, fy+1, cz) {
			if m.OnGround && m.JumpCooldown == 0 {
				m.Vy = 0.32
				m.OnGround = false
			}
		}
	}
	bx, by, bz := int(math.Floor(nx)), int(math.Floor(m.Y)), int(math.Floor(nz))
	if ctx.SolidAt(bx, by, bz) {
		if def.BreaksDoors && ctx.Difficulty == "hard" &&
			ctx.DoorAt != nil && ctx.OnBreakDoor != nil && ctx.DoorAt(bx, by, bz) {
			if ctx.OnBreakDoor(bx, by, bz) {
				m.StuckTicks = 0
				m.X, m.Z = nx, nz
				return
			}
		}
		if m.JumpCooldown == 0 && m.OnGround {
			JumpMob(m)
		} else {
			m.StuckTicks++
			if m.StuckTicks >= 3 {
				SetBodyYaw(m, m.Yaw+90)
				m.StuckTicks = 0
			}
		}
		return
	}
	m.StuckTicks = 0
	m.X, m.Z = nx, nz
}

// WanderStep performs one tick of idle random strolling. The mob
// walks forward along its Yaw for a randomised run, occasionally
// picking a new heading; when not walking it lets its head drift.
// Baby passive mobs are twitchier (shorter runs, occasional playful
// hops), matching vanilla.
func WanderStep(m *mobs.Mob, def mobs.MobDef, ctx *context.AIContext) {
	walkChance, walkLen, walkJitter := 0.0125, 80, 160
	if m.Baby && !def.IsHostile {
		walkChance, walkLen, walkJitter = 0.03, 40, 80
		if m.OnGround && m.JumpCooldown == 0 && ctx.RNG.Float64() < 0.01 {
			JumpMob(m)
		}
	}
	if m.WalkTicks <= 0 && ctx.RNG.Float64() < walkChance {
		m.WalkTicks = walkLen + ctx.RNG.Intn(walkJitter)
		SetBodyYaw(m, ctx.RNG.Float64()*360-180)
	}
	if def.Movement == "hop" && m.OnGround && m.JumpCooldown == 0 {
		jumpEvery := 6
		switch {
		case m.Size == 2:
			jumpEvery = 10
		case m.Size == 3:
			jumpEvery = 14
		case m.Size >= 4:
			jumpEvery = 20
		}
		if m.AmbientCD == 0 {
			m.AmbientCD = jumpEvery
			JumpMob(m)
		}
	}
	if m.WalkTicks > 0 {
		dx := -math.Sin(m.Yaw * math.Pi / 180)
		dz := math.Cos(m.Yaw * math.Pi / 180)
		m.WalkTicks--
		StepHorizontal(m, def, ctx, dx, dz, def.WanderSpeed, false)
	}
}

// LookAt eases the mob's head toward the point (tx, ty, tz),
// setting both HeadYaw and HeadPitch. The rotation is clamped to
// MaxHeadYawTurn / MaxHeadPitchTurn per tick so the head tracks
// smoothly instead of snapping.
func LookAt(m *mobs.Mob, tx, ty, tz float64, turnBody bool) {
	dx, dz := tx-m.X, tz-m.Z
	yaw := math.Atan2(-dx, dz)*180/math.Pi + 180
	dy := ty - (m.Y + MobEyeHeightFor(m))
	pitch := -math.Atan2(dy, math.Hypot(dx, dz)) * 180 / math.Pi
	m.HeadYaw = ApproachAngle(m.HeadYaw, yaw, MaxHeadYawTurn)
	m.HeadPitch = ApproachAngle(m.HeadPitch, pitch, MaxHeadPitchTurn)
	if turnBody {
		SetBodyYaw(m, m.HeadYaw)
	} else {
		ClampHeadYawToBody(m)
	}
}

// SetBodyYaw sets the body yaw and re-clamps the head.
func SetBodyYaw(m *mobs.Mob, yaw float64) {
	m.Yaw = NormalizeAngle(yaw)
	ClampHeadYawToBody(m)
}

// ClampHeadYawToBody keeps the head within ±MaxHeadBodyYaw of the
// body.
func ClampHeadYawToBody(m *mobs.Mob) {
	m.HeadYaw = NormalizeAngle(m.Yaw + Clamp(WrapDegrees(m.HeadYaw-m.Yaw), -MaxHeadBodyYaw, MaxHeadBodyYaw))
}

// Clamp clamps v to [min, max].
func Clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// ApproachAngle moves cur toward target by at most maxStep degrees,
// taking the shortest way around the 360° wrap.
func ApproachAngle(cur, target, maxStep float64) float64 {
	d := WrapDegrees(target - cur)
	if d > maxStep {
		d = maxStep
	} else if d < -maxStep {
		d = -maxStep
	}
	return NormalizeAngle(cur + d)
}

// NormalizeAngle wraps a into (-180, 180].
func NormalizeAngle(a float64) float64 { return WrapDegrees(a) }

// WrapDegrees normalises an angle delta to (-180, 180].
func WrapDegrees(d float64) float64 {
	for d > 180 {
		d -= 360
	}
	for d <= -180 {
		d += 360
	}
	return d
}

// _ is a placeholder so the package compiles even if the
// pathfind import is not currently used (the entity/pathfind
// package provides the A* under the hood; the mob path helpers
// in mobs/path.go wrap it).
var _ = context.ZeroUUID
