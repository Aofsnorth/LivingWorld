package mobs

import "math"

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
		step := speed * 0.5
		m.X += dx / dl * step
		m.Z += dz / dl * step
		if face {
			m.Yaw = math.Atan2(-dx, dz) * 180 / math.Pi
		}
		return
	}

	dl := math.Hypot(dx, dz)
	if dl == 0 {
		return
	}
	step := speed * 0.5

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
		m.Yaw = math.Atan2(-dx, dz) * 180 / math.Pi
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
				m.Yaw += 90
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
		m.Yaw = ctx.RNG.Float64()*360 - 180
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
		return
	}
	// Idle head drift on a 30-tick cadence.
	if m.aiTick%30 == 0 {
		m.HeadYaw += (ctx.RNG.Float64() - 0.5) * 30.0
	}
}

// lookAt sets HeadYaw (and optionally body Yaw) toward (tx, tz).
func lookAt(m *Mob, tx, tz float64, turnBody bool) {
	yaw := math.Atan2(-(tx - m.X), tz-m.Z) * 180 / math.Pi
	m.HeadYaw = yaw
	if turnBody {
		m.Yaw = yaw
	}
}
