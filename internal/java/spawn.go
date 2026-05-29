package java

import (
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// buildPlayerPositionPacket builds the clientbound synchronize-player-position
// packet (teleport). The Y must sit on top of the terrain — for the superflat
// world the grass surface is y=3, so feet rest at y=4. Spawning at y=64 makes
// the player free-fall to the ground on join.
func buildPlayerPositionPacket(teleportID int32, x, y, z float64, yaw, pitch float32) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGamePlayerPosition,
		pk.VarInt(teleportID),
		pk.Double(x), pk.Double(y), pk.Double(z),
		pk.Double(0), pk.Double(0), pk.Double(0), // delta movement
		pk.Float(yaw), pk.Float(pitch),
		pk.Int(0), // relative flags (all absolute)
	)
}

// sendSpawnPosition teleports the player to the session's spawn coordinates.
func (s *PlayerSession) sendSpawnPosition() error {
	s.mu.Lock()
	x, y, z, yaw, pitch := s.X, s.Y, s.Z, s.Yaw, s.Pitch
	s.mu.Unlock()
	return s.SendPacket(buildPlayerPositionPacket(0, x, y, z, yaw, pitch))
}
