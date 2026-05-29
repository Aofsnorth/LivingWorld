package server

// Fall damage. Minecraft is server-authoritative for damage — the client never
// hurts itself — so the server tracks how far a player has fallen and applies
// damage on landing. Vanilla rule: damage = floor(fallDistance) - 3, min 0,
// dealt when the player touches the ground.
const fallSafeBlocks = 3.0

// trackFall accumulates fall distance from successive Y positions and applies
// damage the moment the player lands.
func (s *PlayerSession) trackFall(oldY, newY float64, onGround bool) {
	s.mu.Lock()
	if newY < oldY {
		s.FallDistance += oldY - newY
	}
	landed := onGround && s.FallDistance > 0
	dist := s.FallDistance
	if onGround {
		s.FallDistance = 0
	}
	s.mu.Unlock()

	if landed {
		if dmg := dist - fallSafeBlocks; dmg > 0 {
			s.damage(float32(dmg))
		}
	}
}

// damage reduces health by amount (clamped at 0) and syncs it to the client.
func (s *PlayerSession) damage(amount float32) {
	s.mu.Lock()
	s.Health -= amount
	if s.Health < 0 {
		s.Health = 0
	}
	s.mu.Unlock()
	_ = s.sendHealth()
}
