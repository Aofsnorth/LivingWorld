package server

import (
	"livingworld/internal/world"
	"math"
)

// Fall damage. Minecraft is server-authoritative for damage — the client never
// hurts itself — so the server tracks how far a player has fallen and applies
// damage on landing. Vanilla rule: damage = floor(fallDistance) - 3, min 0,
// dealt when the player touches the ground.
const fallSafeBlocks = 3.35

func isFallResetBlock(name string) bool {
	switch name {
	case "minecraft:water", "minecraft:lava", "minecraft:bubble_column",
		"minecraft:cobweb", "minecraft:powder_snow", "minecraft:sweet_berry_bush",
		"minecraft:ladder", "minecraft:vine", "minecraft:scaffolding",
		"minecraft:weeping_vines", "minecraft:weeping_vines_plant",
		"minecraft:twisting_vines", "minecraft:twisting_vines_plant",
		"minecraft:cave_vines", "minecraft:cave_vines_plant", "minecraft:slime_block", "minecraft:honey_block":
		return true
	}
	return false
}

// trackFall accumulates fall distance from successive Y positions and applies
// damage the moment the player lands.
func (s *PlayerSession) trackFall(oldY, newY float64, onGround bool) {
	s.mu.Lock()
	if s.Health <= 0 {
		s.FallDistance = 0
		s.mu.Unlock()
		return
	}
	x, z := s.X, s.Z
	if newY > oldY+0.02 {
		s.FallDistance = 0
	} else if newY < oldY {
		s.FallDistance += oldY - newY
	}
	if s.Bridge != nil && s.Bridge.wm != nil {
		if dw := s.Bridge.wm.GetDefaultWorld(); dw != nil {
			bx, by, bz := int(math.Floor(x)), int(math.Floor(newY)), int(math.Floor(z))
			if isFallResetBlock(world.StateName(dw.GetBlock(bx, by, bz).ID())) ||
				isFallResetBlock(world.StateName(dw.GetBlock(bx, by-1, bz).ID())) {
				s.FallDistance = 0
			}
		}
	}
	landed := onGround && s.FallDistance > 0
	dist := s.FallDistance
	if onGround {
		s.FallDistance = 0
	}
	s.mu.Unlock()

	// Creative (and Spectator) never take fall damage. The session's
	// GameModeVal is the authoritative mode — `/gamemode creative` and
	// /gamemode spectator both set it to a non-survival value. Falling
	// while in creative was the most visible creative-mode bug reported.
	if s.GameMode() != 0 {
		return
	}

	if landed {
		// Vanilla integer fall damage: ceil(fallDistance - 3.35).
		// Walking off 3 blocks (3.0) or jumping off 2 blocks (3.2522) deals 0 damage,
		// while falling 4+ blocks (3.95..4.0) or jumping off 3 blocks (4.2522) deals 1+ damage.
		if raw := dist - fallSafeBlocks; raw > 0 {
			dmg := int(math.Ceil(raw))
			s.mu.Lock()
			x, y, z := s.X, s.Y, s.Z
			s.mu.Unlock()
			fallSound := "minecraft:entity.player.small_fall"
			if dmg > 4 {
				fallSound = "minecraft:entity.player.big_fall"
			}
			if s.Bridge != nil {
				if s.Bridge.sessions != nil {
					s.Bridge.sessions.Broadcast(buildPlayerSoundPacket(fallSound, x, y, z, 1.0, 1.0))
				}
			}
			s.damage(float32(dmg))
		}
	}
}

// damage reduces health by amount (clamped at 0) and syncs it to the client.
func (s *PlayerSession) damage(amount float32) {
	if amount <= 0 {
		return
	}
	s.mu.Lock()
	if s.Health <= 0 {
		s.mu.Unlock()
		return
	}
	s.Health -= amount
	if s.Health < 0 {
		s.Health = 0
	}
	newHealth := s.Health
	x, y, z := s.X, s.Y, s.Z
	s.mu.Unlock()

	if s.Bridge != nil && s.Bridge.pm != nil {
		s.Bridge.pm.SetHealth(s.UUIDVal, newHealth)
		s.Bridge.pm.AddExhaustion(s.UUIDVal, 0.1)
	}

	_ = s.sendHealth()
	hurtPkt := hurtAnimationPacket(s.EntityID())
	_ = s.SendPacket(hurtPkt)
	if s.Bridge != nil {
		if s.Bridge.sessions != nil {
			s.Bridge.sessions.BroadcastExcept(s.UUIDVal, hurtPkt)
			hurtSound := "minecraft:entity.player.hurt"
			if newHealth <= 0 {
				hurtSound = "minecraft:entity.player.death"
			}
			s.Bridge.sessions.Broadcast(buildPlayerSoundPacket(hurtSound, x, y, z, 1.0, 1.0))
		}
	}
}
