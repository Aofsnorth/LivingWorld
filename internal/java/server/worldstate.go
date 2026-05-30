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

// buildSetTimePacket builds the MC 26.1 set_time packet (id 113). 26.1 replaced
// the old (Long worldAge, Long timeOfDay, Bool tickDayTime) body with a
// count-prefixed list of per-dimension "world clocks"; sending the old shape
// crashes the client. We emit exactly one overworld clock (id 0).
//
//	worldAge  = monotonically increasing total tick count
//	dayTime   = time-of-day in ticks (0..23999; 0=dawn, 6000=noon, 18000=midnight)
//	advancing = whether the client auto-advances the sun (rate 1.0 vs frozen 0.0)
//
// Verified against ViaVersion's 1.21.9→1.21.11 rewriter and the Rosegold
// protocol>=775 decoder (the live wiki is still pinned to protocol 773).
func buildSetTimePacket(worldAge, dayTime int64, advancing bool) pk.Packet {
	rate := pk.Float(0)
	if advancing {
		rate = pk.Float(1.0)
	}
	return pk.Marshal(
		packetid.ClientboundGameSetTime,
		pk.Long(worldAge),   // field 1: world age / game time
		pk.VarInt(1),        // field 2: clock count = 1
		pk.VarInt(0),        // 2a: clock id 0 = overworld day clock
		pk.VarLong(dayTime), // 2b: total ticks (positions the sun)
		pk.Float(0),         // 2c: partial tick
		rate,                // 2d: rate (1.0 advancing, 0.0 frozen)
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
