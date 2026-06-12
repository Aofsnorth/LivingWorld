package mobs

import "math"

// speedToBlocksPerTick converts a MobDef's movement-speed attribute
// (WanderSpeed / ChaseSpeed, in vanilla attribute units) into the per-tick
// horizontal displacement the integrator applies. Calibrated against vanilla:
// a zombie's ChaseSpeed 0.35 → 0.217 b/tick (≈ 4.3 b/s), matching the observed
// vanilla zombie pursuit speed. Wander values (0.20–0.30) land at 2.5–3.7 b/s,
// the casual passive amble.
const speedToBlocksPerTick = 0.62

// Head look limits. lookAt eases HeadYaw/HeadPitch toward the target by at most
// these many degrees per tick instead of snapping, so a mob's head tracks a
// player smoothly (vanilla LookControl clamps head rotation the same way).
const (
	maxHeadYawTurn   = 10.0 // vanilla getMaxHeadYRot() default — 10°/tick
	maxHeadPitchTurn = 40.0 // vanilla getMaxHeadXRot() default — 40°/tick
	maxHeadBodyYaw   = 75.0
	// bodyFollowDelay is the number of stationary ticks before the body starts
	// rotating toward the head yaw (vanilla BodyRotationControl threshold).
	bodyFollowDelay = 10
	// mobEyeHeight is the fallback eye height used to compute look pitch when a
	// mob type has no entry in mobEyeHeightFor. Per-type heights live there;
	// using a flat 1.6 for everything made short mobs (pig/chicken) compute a
	// near-level pitch and stare at the player's torso instead of their face.
	mobEyeHeight = 1.6
	// playerEyeHeight is where a look goal aims on the target player, so mobs
	// look at the player's eyes rather than their feet.
	playerEyeHeight = 1.62
)

// mobEyeHeightFor returns the approximate eye height (blocks above feet) for a
// mob type, used as the origin when computing look pitch so the head tilt reads
// correctly across body sizes. Values are vanilla eye heights (≈ 0.85× standing
// height for most mobs). Baby mobs are half height. Unknown types fall back to
// mobEyeHeight.
func mobEyeHeightFor(m *Mob) float64 {
	var h float64
	switch m.Type {
	case "minecraft:chicken":
		h = 0.64
	case "minecraft:pig", "minecraft:sheep":
		h = 0.77
	case "minecraft:cow":
		h = 1.05
	case "minecraft:slime", "minecraft:magma_cube":
		h = 0.7 * float64(maxInt(m.Size, 1))
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
		h = mobEyeHeight
	}
	if m.Baby {
		h *= 0.5
	}
	return h
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Shared movement primitives used by the behaviour goals. These own the
// "translate a desired direction into a physically-valid step" logic that the
// old moveMob folded into its state switch: path following, step-up jumps,
// door breaking, wall climbing, water/baby speed scaling, and stuck recovery.
// Goals decide WHERE to go; these helpers decide HOW the body gets there.

// navigateTo advances the mob one tick toward (tx, ty, tz) at the given
// attribute speed, following a cached A* path that is replanned every 20 ticks
// or when the goal cell drifts. Returns the horizontal distance remaining to
// the goal so callers can decide arrival.
func navigateTo(m *Mob, def MobDef, ctx *AIContext, tx, ty, tz, speed float64) float64 {
	mobCell := PathNode{X: int(math.Floor(m.X)), Y: int(math.Floor(m.Y)), Z: int(math.Floor(m.Z))}
	goalCell := PathNode{X: int(math.Floor(tx)), Y: int(math.Floor(ty)), Z: int(math.Floor(tz))}
	if !m.hasPath() || m.pathGoalStale(goalCell) || m.pathTick == 0 {
		m.replanPath(mobCell, goalCell, *ctx)
	}

	var dx, dz float64
	mx, mz := int(math.Floor(m.X)), int(math.Floor(m.Z))
	if pdx, pdz, py, ok := m.pathDirection(mx, mz); ok {
		dx, dz = pdx, pdz
		// Next waypoint is elevated and we're grounded → jump.
		if py > int(math.Floor(m.Y)) && m.OnGround && m.jumpCooldown == 0 {
			jumpMob(m)
		}
	} else {
		// Path exhausted/empty — beeline.
		dx, dz = tx-m.X, tz-m.Z
	}
	stepHorizontal(m, def, ctx, dx, dz, speed, true)
	return math.Hypot(tx-m.X, tz-m.Z)
}

// fleeFrom advances the mob one tick directly away from (fx, fz) at the given
// speed. No pathing — panic is a straight dash (vanilla PanicGoal beelines and
// re-rolls a target tile, but a direct retreat reads identically at v1 scale).
func fleeFrom(m *Mob, def MobDef, ctx *AIContext, fx, fz, speed float64) {
	dx, dz := m.X-fx, m.Z-fz
	if dx == 0 && dz == 0 {
		dx, dz = math.Sin(m.Yaw*math.Pi/180), math.Cos(m.Yaw*math.Pi/180)
	}
	stepHorizontal(m, def, ctx, dx, dz, speed, true)
}

// stepHorizontal normalises (dx, dz), scales it to a per-tick delta, and tries
// to step. If blocked it attempts (in order) a door-break, a step-up jump, then
// counts the step as stuck (turning 90° after 3 consecutive blocked ticks).
// `face` controls whether the body Yaw is reoriented to the move direction
// (wandering already picked its Yaw and passes false).
func stepHorizontal(m *Mob, def MobDef, ctx *AIContext, dx, dz, speed float64, face bool) {
	if def.Movement == "fly" {
		// Flying mobs glide horizontally without ground collision; their
		// goal sets vertical motion. Apply a simple unobstructed step.
		dl := math.Hypot(dx, dz)
		if dl == 0 {
			return
		}
		step := speed * speedToBlocksPerTick
		m.X += dx / dl * step
		m.Z += dz / dl * step
		if face {
			setBodyYaw(m, math.Atan2(-dx, dz)*180/math.Pi)
		}
		return
	}

	dl := math.Hypot(dx, dz)
	if dl == 0 {
		return
	}
	step := speed * speedToBlocksPerTick

	// Water slow (0.5×) — hostile mobs wade slowly.
	if ctx.WaterAt != nil {
		fx, fy, fz := int(math.Floor(m.X)), int(math.Floor(m.Y)), int(math.Floor(m.Z))
		if ctx.WaterAt(fx, fy, fz) {
			step *= 0.5
		}
	}
	// Baby mobs move 1.5× faster (vanilla).
	if m.Baby {
		step *= 1.5
	}

	nx := m.X + dx/dl*step
	nz := m.Z + dz/dl*step
	if face {
		setBodyYaw(m, math.Atan2(-dx, dz)*180/math.Pi)
	}

	// Spider wall-climb: if the step is into a wall, inject a small upward vy.
	if def.Movement == "climb" {
		cx, cz := int(math.Floor(nx)), int(math.Floor(nz))
		fy := int(math.Floor(m.Y))
		if ctx.SolidAt(cx, fy, cz) || ctx.SolidAt(cx, fy+1, cz) {
			if m.OnGround && m.jumpCooldown == 0 {
				m.vy = 0.32
				m.OnGround = false
			}
		}
	}

	// Collision: try door-break, then jump, then stuck-turn.
	bx, by, bz := int(math.Floor(nx)), int(math.Floor(m.Y)), int(math.Floor(nz))
	if ctx.SolidAt(bx, by, bz) {
		if def.BreaksDoors && ctx.Difficulty == "hard" &&
			ctx.DoorAt != nil && ctx.OnBreakDoor != nil && ctx.DoorAt(bx, by, bz) {
			if ctx.OnBreakDoor(bx, by, bz) {
				m.stuckTicks = 0
				m.X, m.Z = nx, nz
				return
			}
		}
		if m.jumpCooldown == 0 && m.OnGround {
			jumpMob(m)
		} else {
			m.stuckTicks++
			if m.stuckTicks >= 3 {
				setBodyYaw(m, m.Yaw+90)
				m.stuckTicks = 0
			}
		}
		return
	}
	m.stuckTicks = 0
	m.X, m.Z = nx, nz
}

// wanderStep performs one tick of idle random strolling. The mob walks forward
// along its Yaw for a randomised run, occasionally picking a new heading; when
// not walking it lets its head drift. Baby passive mobs are twitchier (shorter
// runs, occasional playful hops), matching vanilla.
func wanderStep(m *Mob, def MobDef, ctx *AIContext) {
	walkChance, walkLen, walkJitter := 0.0125, 80, 160
	if m.Baby && !def.IsHostile {
		walkChance, walkLen, walkJitter = 0.03, 40, 80
		if m.OnGround && m.jumpCooldown == 0 && ctx.RNG.Float64() < 0.01 {
			jumpMob(m)
		}
	}
	if m.walkTicks <= 0 && ctx.RNG.Float64() < walkChance {
		m.walkTicks = walkLen + ctx.RNG.Intn(walkJitter)
		setBodyYaw(m, ctx.RNG.Float64()*360-180)
	}
	// Hop movement (slime/magma cube): force a jump on a size-scaled cadence.
	if def.Movement == "hop" && m.OnGround && m.jumpCooldown == 0 {
		jumpEvery := 6
		switch {
		case m.Size == 2:
			jumpEvery = 10
		case m.Size == 3:
			jumpEvery = 14
		case m.Size >= 4:
			jumpEvery = 20
		}
		if m.ambientCD == 0 {
			m.ambientCD = jumpEvery
			jumpMob(m)
		}
	}
	if m.walkTicks > 0 {
		dx := -math.Sin(m.Yaw * math.Pi / 180)
		dz := math.Cos(m.Yaw * math.Pi / 180)
		m.walkTicks--
		stepHorizontal(m, def, ctx, dx, dz, def.WanderSpeed, false)
	}
	// NOTE: idle head drift is owned by randomLookAroundGoal (a FlagLook goal),
	// not here. wanderStep holds only FlagMove; writing HeadYaw from here too
	// would fight the look goals on the same tick and make the head blink.
}

// lookAt eases the mob's head toward the point (tx, ty, tz), setting both
// HeadYaw and HeadPitch. The rotation is clamped to maxHeadYawTurn /
// maxHeadPitchTurn per tick so the head tracks smoothly instead of snapping
// (vanilla LookControl behaves the same). When turnBody is set the body Yaw
// follows the (already-clamped) head yaw, used by goals that must face a
// target to act on it (melee).
func lookAt(m *Mob, tx, ty, tz float64, turnBody bool) {
	dx, dz := tx-m.X, tz-m.Z
	yaw := math.Atan2(-dx, dz) * 180 / math.Pi
	// Pitch: positive = looking down (Minecraft convention). A target above the
	// mob's eye (dy > 0) yields a negative pitch (look up).
	dy := ty - (m.Y + mobEyeHeightFor(m))
	pitch := -math.Atan2(dy, math.Hypot(dx, dz)) * 180 / math.Pi

	m.HeadYaw = approachAngle(m.HeadYaw, yaw, maxHeadYawTurn)
	m.HeadPitch = approachAngle(m.HeadPitch, pitch, maxHeadPitchTurn)
	if turnBody {
		setBodyYaw(m, m.HeadYaw)
	} else {
		clampHeadYawToBody(m)
	}
}

func setBodyYaw(m *Mob, yaw float64) {
	m.Yaw = normalizeAngle(yaw)
	clampHeadYawToBody(m)
}

func clampHeadYawToBody(m *Mob) {
	m.HeadYaw = normalizeAngle(m.Yaw + clamp(wrapDegrees(m.HeadYaw-m.Yaw), -maxHeadBodyYaw, maxHeadBodyYaw))
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// approachAngle moves cur toward target by at most maxStep degrees, taking the
// shortest way around the 360° wrap.
func approachAngle(cur, target, maxStep float64) float64 {
	d := wrapDegrees(target - cur)
	if d > maxStep {
		d = maxStep
	} else if d < -maxStep {
		d = -maxStep
	}
	return normalizeAngle(cur + d)
}

func normalizeAngle(a float64) float64 {
	return wrapDegrees(a)
}

// wrapDegrees normalises an angle delta to (-180, 180].
func wrapDegrees(d float64) float64 {
	for d > 180 {
		d -= 360
	}
	for d <= -180 {
		d += 360
	}
	return d
}
