package server

import (
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Vanilla survival defaults.
const (
	MaxHealth     float32 = 20
	MaxFood       int32   = 20
	MaxSaturation float32 = 5
)

// buildSetHealthPacket builds the clientbound SetHealth packet. food=20 is a
// full hunger bar; sending 10 (as the old code did) shows only half a bar.
func buildSetHealthPacket(health float32, food int32, saturation float32) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameSetHealth,
		pk.Float(health),
		pk.VarInt(food),
		pk.Float(saturation),
	)
}

// sendHealth pushes the session's current health/food/saturation to the client.
func (s *PlayerSession) sendHealth() error {
	s.mu.Lock()
	h, f, sat := s.Health, s.Food, s.Saturation
	s.mu.Unlock()
	return s.SendPacket(buildSetHealthPacket(h, f, sat))
}
