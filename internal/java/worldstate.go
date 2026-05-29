package java

import (
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// buildSetDefaultSpawnPositionPacket builds ClientboundSetDefaultSpawnPosition
// (compass target / world spawn). In MC 26.1 the payload is a RespawnData =
// GlobalPos{ Identifier dimension, Position blockPos } + Float yaw + Float
// pitch — NOT the old (Position, Float angle) form, which decodes 13 bytes and
// crashes the client with readerIndex(10)+length(4) exceeds writerIndex(13).
func buildSetDefaultSpawnPositionPacket(dimension string, x, y, z int, yaw, pitch float32) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameSetDefaultSpawnPosition,
		pk.Identifier(dimension),
		pk.Position{X: x, Y: y, Z: z},
		pk.Float(yaw),
		pk.Float(pitch),
	)
}

// sendWorldState sends the default spawn position so the client points its
// compass at spawn. Sky color comes from the dimension_type/biome registry, not
// from a time packet — and MC 26.1 replaced the old ClientboundSetTime
// (2 longs + bool) with a WorldClock map, so we deliberately do NOT send time
// here; an incorrect SetTime would crash the client and time-of-day now lives
// in the separate clock system (a future feature).
func (s *PlayerSession) sendWorldState() {
	sp := s.Bridge.cfg.World.Spawn
	_ = s.SendPacket(buildSetDefaultSpawnPositionPacket("minecraft:overworld", int(sp.X), int(sp.Y), int(sp.Z), sp.Yaw, sp.Pitch))
}
