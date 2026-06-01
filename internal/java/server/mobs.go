package server

import (
	"math"

	"livingworld/internal/mobs"

	"github.com/Tnze/go-mc/data/entity"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// yawToAngle converts a yaw in Minecraft degrees to the protocol's single-byte
// angle (256 steps over 360°). Used for the head-rotation packet and AddEntity,
// which encode rotation as a byte rather than a float.
func yawToAngle(deg float64) pk.Angle {
	v := int(math.Round(deg * 256.0 / 360.0))
	return pk.Angle(int8(v & 0xff))
}

// javaMobTypeIDs maps a namespaced mob type to its Java (protocol 775) entity
// type id. Unknown types fall back to a pig.
var javaMobTypeIDs = map[string]int32{
	"minecraft:pig":      int32(entity.Pig.ID),
	"minecraft:cow":      int32(entity.Cow.ID),
	"minecraft:chicken":  int32(entity.Chicken.ID),
	"minecraft:sheep":    int32(entity.Sheep.ID),
	"minecraft:creeper":  int32(entity.Creeper.ID),
	"minecraft:zombie":   int32(entity.Zombie.ID),
	"minecraft:skeleton": int32(entity.Skeleton.ID),
}

// startMobSync renders shared mob spawns/despawns to all Java sessions.
func (j *javaBridge) startMobSync() {
	store := j.wm.Mobs()
	store.OnSpawn(func(m mobs.Mob) { j.sessions.Broadcast(spawnMobPacket(m)) })
	store.OnDespawn(func(id int64) { j.sessions.Broadcast(removeMobPacket(id)) })
	store.OnMove(func(m mobs.Mob) {
		j.sessions.Broadcast(moveMobPacket(m))
		// The teleport carries the BODY yaw; the head is rotated separately or it
		// keeps facing south while the body turns. Send both so the mob looks where
		// it walks.
		j.sessions.Broadcast(headRotatePacket(m))
	})
}

func moveMobPacket(m mobs.Mob) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameTeleportEntity,
		pk.VarInt(int32(m.EntityID)),
		pk.Double(m.X), pk.Double(m.Y), pk.Double(m.Z),
		pk.Double(0), pk.Double(0), pk.Double(0), // velocity
		pk.Float(float32(m.Yaw)), pk.Float(0), // body yaw (degrees), pitch
		pk.Int(0),        // flags
		pk.Boolean(true), // onGround
	)
}

// headRotatePacket turns the mob's head to match its heading (Yaw). Without it the
// head stays at the AddEntity head-yaw while the body teleport-rotates, which reads
// as a mob walking with its head locked forward.
func headRotatePacket(m mobs.Mob) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameRotateHead,
		pk.VarInt(int32(m.EntityID)),
		yawToAngle(m.Yaw),
	)
}

func spawnMobPacket(m mobs.Mob) pk.Packet {
	typeID, ok := javaMobTypeIDs[m.Type]
	if !ok {
		typeID = int32(entity.Pig.ID)
	}
	yaw := yawToAngle(m.Yaw)
	return pk.Marshal(
		packetid.ClientboundGameAddEntity,
		pk.VarInt(int32(m.EntityID)),
		pk.UUID(m.UUID),
		pk.VarInt(typeID),
		pk.Double(m.X), pk.Double(m.Y), pk.Double(m.Z),
		pk.Byte(0),   // head pitch
		pk.Angle(0),  // pitch
		yaw,          // yaw
		yaw,          // head yaw
		pk.VarInt(0), // data
	)
}

func removeMobPacket(id int64) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameRemoveEntities,
		pk.Ary[pk.VarInt]{Ary: []pk.VarInt{pk.VarInt(int32(id))}},
	)
}

// spawnExistingMobs sends all current mobs to a session on join.
func (s *PlayerSession) spawnExistingMobs() {
	for _, m := range s.Bridge.wm.Mobs().All() {
		_ = s.SendPacket(spawnMobPacket(m))
	}
}
