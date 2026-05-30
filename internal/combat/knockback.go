package combat

import (
	"math"

	"livingworld/internal/registry"
)

// Knockback applies vanilla melee knockback to a victim's current velocity and
// returns the new velocity. dirX/dirZ is the horizontal direction from the
// victim to the attacker (attacker.Pos − victim.Pos); the victim is pushed the
// opposite way. strength is the knockback power (≈0.4 for a bare hit, +0.5 per
// Knockback level / sprint); kbResistance is the victim's resistance in [0,1].
// Mirrors LivingEntity.knockback.
func Knockback(vel registry.Vec3, strength, dirX, dirZ, kbResistance float64, onGround bool) registry.Vec3 {
	strength *= 1 - kbResistance
	d := math.Hypot(dirX, dirZ)
	if strength <= 0 || d == 0 {
		return vel
	}
	px, pz := dirX/d*strength, dirZ/d*strength
	out := registry.Vec3{X: vel.X/2 - px, Z: vel.Z/2 - pz}
	if onGround {
		out.Y = math.Min(0.4, vel.Y/2+strength)
	} else {
		out.Y = vel.Y
	}
	return out
}
