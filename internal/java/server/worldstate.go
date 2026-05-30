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

// buildSetTimePacket builds the MC 26.1 (protocol 775) update_time (set_time)
// packet. 26.1 reworked time into per-dimension "world clocks" (registry
// minecraft:world_clock), so the packet is no longer the old three scalars — it
// carries a global world age followed by a list of per-clock updates:
//
//	age      = global game time, monotonic ticks (Long)
//	count    = number of clock updates (VarInt)
//	updates  = count × {
//	             clockId   = world_clock registry index; 0 = overworld (VarInt)
//	             advancing = this clock ticks on the client (Boolean)
//	             dayTime   = this clock's time-of-day, wraps at 24000 (Long)
//	           }
//
// Field order inside the entry is advancing-before-dayTime. Sending
// {clockId, dayTime, advancing} (the obvious order) decodes without error but
// makes the client read `advancing` from the high byte of dayTime (0x00 → false)
// and `dayTime` from the shifted remainder — the sun freezes at a bogus midday
// position. The flat three-scalar form (no clock list) instead leaves 8 trailing
// bytes and the client rejects the packet ("8 bytes extra").
func buildSetTimePacket(worldAge, dayTime int64, advancing bool) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameSetTime,
		pk.Long(worldAge),     // age (global)
		pk.VarInt(1),          // clock update count
		pk.VarInt(0),          // clockId 0 = minecraft:overworld
		pk.Boolean(advancing), // this clock advances on the client
		pk.Long(dayTime),      // this clock's time-of-day
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
// difficulty, and the current time-of-day (see buildSetTimePacket for the 26.1
// update_time format).
func (s *PlayerSession) sendWorldState() {
	sp := s.Bridge.cfg.World.Spawn
	_ = s.SendPacket(buildSetDefaultSpawnPositionPacket("minecraft:overworld", int(sp.X), int(sp.Y), int(sp.Z), sp.Yaw, sp.Pitch))

	_ = s.SendPacket(buildChangeDifficultyPacket(s.Bridge.cfg.World.DifficultyByte(), true))

	w := s.Bridge.wm.GetDefaultWorld()
	_ = s.SendPacket(buildSetTimePacket(w.GetTime(), w.GetDayTime(), s.Bridge.cfg.World.DayNightCycle))
}
