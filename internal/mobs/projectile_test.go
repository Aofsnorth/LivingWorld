package mobs

import "testing"

// TestM3_SpawnKind_StoresYawPitch verifies the M3 addition of
// Yaw/Pitch fields to Projectile. The bridges read these to set
// the visible orientation of fireballs and potions (arrows and
// tridents ignore them and interpolate from velocity).
func TestM3_SpawnKind_StoresYawPitch(t *testing.T) {
	s := NewProjectileStore()
	// 45° yaw, 30° pitch, no target.
	p := s.SpawnKind(1, [16]byte{}, 0, 0, 0, 45, 30, ProjectileSmallFireball)
	if p.Kind != ProjectileSmallFireball {
		t.Errorf("kind: got %q want %q", p.Kind, ProjectileSmallFireball)
	}
	if p.Yaw != 45 {
		t.Errorf("yaw: got %v want 45", p.Yaw)
	}
	if p.Pitch != 30 {
		t.Errorf("pitch: got %v want 30", p.Pitch)
	}
	// Arrow path also stores yaw/pitch (just unused by bridges).
	a := s.Spawn(1, [16]byte{}, 0, 0, 0, 90, 0)
	if a.Kind != ProjectileArrow {
		t.Errorf("Spawn kind: got %q want %q", a.Kind, ProjectileArrow)
	}
	if a.Yaw != 90 {
		t.Errorf("arrow yaw: got %v want 90", a.Yaw)
	}
}

// TestM3_SpawnKind_PerKindVelocity verifies the M1 velocity
// scaling: small_fireball=0.6, potion=0.75, default=1.6.
func TestM3_SpawnKind_PerKindVelocity(t *testing.T) {
	s := NewProjectileStore()
	// Shoot straight north (yaw=0), level (pitch=0).
	// velocity: vx = -sin(0)*cos(0)*speed = 0,
	//           vy = -sin(0)*speed = 0,
	//           vz =  cos(0)*cos(0)*speed = +speed.
	cases := []struct {
		kind  string
		speed float64
	}{
		{ProjectileArrow, 1.6},
		{ProjectileSmallFireball, 0.6},
		{ProjectilePotion, 0.75},
	}
	for _, c := range cases {
		p := s.SpawnKind(0, [16]byte{}, 0, 0, 0, 0, 0, c.kind)
		if got := p.VZ; got < c.speed-0.001 || got > c.speed+0.001 {
			t.Errorf("kind=%q VZ: got %v want %v", c.kind, got, c.speed)
		}
	}
}
