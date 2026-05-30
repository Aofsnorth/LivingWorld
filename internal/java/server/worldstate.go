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
// minecraft:world_clock). Verified against the decompiled 26.1 server jar
// (ClientboundSetTimePacket.STREAM_CODEC):
//
//	gameTime     = global game time, monotonic ticks (Long)
//	clockUpdates = Map<Holder<WorldClock>, ClockNetworkState>, encoded as:
//	   count = number of clock updates (VarInt)
//	   count × {
//	     clockId     = world_clock holder id; 0 = overworld (VarInt)
//	     ClockNetworkState {
//	       totalTicks  = this clock's time-of-day (VarLong)
//	       partialTick = sub-tick fraction (Float)
//	       rate        = ticks-per-tick; 1.0 = normal, 0.0 = frozen (Float)
//	     }
//	   }
//
// The value is NOT a boolean+long. The old "tick day time" flag became the
// float `rate`: send 1.0 to let the client advance the sun itself between
// server syncs, 0.0 to freeze it. Sending a Boolean+Long here makes the client
// read totalTicks as a VarLong from the wrong bytes and the sun sticks at a
// bogus midday position.
func buildSetTimePacket(worldAge, dayTime int64, advancing bool) pk.Packet {
	rate := pk.Float(0)
	if advancing {
		rate = pk.Float(1)
	}
	return pk.Marshal(
		packetid.ClientboundGameSetTime,
		pk.Long(worldAge), // gameTime (global)
		pk.VarInt(1),      // clockUpdates map count
		pk.VarInt(0),      // clockId 0 = minecraft:overworld
		pk.VarLong(dayTime), // ClockNetworkState.totalTicks (time-of-day)
		pk.Float(0),         // ClockNetworkState.partialTick
		rate,                // ClockNetworkState.rate (1.0 advance, 0.0 freeze)
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
