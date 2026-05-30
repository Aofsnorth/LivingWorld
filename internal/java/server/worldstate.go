package server

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

// buildSetTimePacket builds the MC 1.21.4/1.21.5 (26.1) set_time packet.
// The format is:
//	worldAge  = monotonically increasing total tick count
//	count     = number of clocks (VarInt)
//	clocks    = array of [id (VarInt), dayTime (Long), advancing (Boolean)]
func buildSetTimePacket(worldAge, dayTime int64, advancing bool) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameSetTime,
		pk.Long(worldAge),
		pk.VarInt(1),          // clock count = 1
		pk.VarInt(0),          // clock id 0 (minecraft:world_clock registry index)
		pk.Long(dayTime),      // total ticks (positions the sun)
		pk.Boolean(advancing), // whether the sun moves
	)
}

// buildChangeDifficultyPacket builds ClientboundChangeDifficulty (id 10). Format
// is unchanged in 26.1: UnsignedByte difficulty + Boolean locked.
func buildChangeDifficultyPacket(difficulty byte, locked bool) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameChangeDifficulty,
		pk.UnsignedByte(difficulty),
		pk.Boolean(locked),
	)
}

// sendWorldState sends the default spawn position (compass target), the current
// difficulty, and the current time-of-day. In MC 26.1 the time packet uses the
// new world-clock list format (see buildSetTimePacket) — the old SetTime shape
// would crash the client, which is why earlier versions of this file omitted it.
func (s *PlayerSession) sendWorldState() {
	sp := s.Bridge.cfg.World.Spawn
	_ = s.SendPacket(buildSetDefaultSpawnPositionPacket("minecraft:overworld", int(sp.X), int(sp.Y), int(sp.Z), sp.Yaw, sp.Pitch))

	_ = s.SendPacket(buildChangeDifficultyPacket(s.Bridge.cfg.World.DifficultyByte(), true))

	w := s.Bridge.wm.GetDefaultWorld()
	_ = s.SendPacket(buildSetTimePacket(w.GetTime(), w.GetDayTime(), s.Bridge.cfg.World.DayNightCycle))
}
