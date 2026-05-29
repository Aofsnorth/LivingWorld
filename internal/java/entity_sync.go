package java

import (
	"log"
	"math"

	"livingworld/internal/player"

	"github.com/Tnze/go-mc/data/entity"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

func (j *javaBridge) startPlayerEventLoop() {
	if j.playerEvents != nil {
		return
	}
	j.playerEvents = j.pm.Subscribe("java-bridge", 256)
	go func() {
		for ev := range j.playerEvents {
			if ev.Player.Edition != player.EditionBedrock {
				continue
			}
			switch ev.Type {
			case player.EventJoin:
				j.sessions.ForEach(func(s *PlayerSession) { s.spawnForeignAvatar(ev.Player) })
			case player.EventMove:
				j.sessions.ForEach(func(s *PlayerSession) { s.moveForeignAvatar(ev.Player) })
			case player.EventLeave:
				j.sessions.ForEach(func(s *PlayerSession) { s.removeForeignAvatar(ev.Player) })
			}
		}
	}()
}

func (s *PlayerSession) spawnExistingForeignPlayers() {
	for _, p := range s.Bridge.pm.GetAllPlayers() {
		if p.Edition == player.EditionBedrock && p.UUID != s.UUID {
			s.spawnForeignAvatar(p.Snapshot())
		}
	}
}

func (s *PlayerSession) spawnForeignAvatar(p player.PlayerSnapshot) {
	if p.UUID == s.UUID || p.EntityRuntimeID == 0 {
		return
	}
	entityID := int32(p.EntityRuntimeID)
	pos := p.Position
	rot := p.Rotation

	// Minimal Java visibility: spawn Bedrock players as a zombie avatar.
	// This avoids the strict PlayerInfo/skin pipeline while still making the
	// cross-protocol player visible and movable. Later this can be upgraded to a
	// true minecraft:player spawn with profile/skin data.
	err := s.SendPacket(pk.Marshal(
		packetid.ClientboundGameAddEntity,
		pk.VarInt(entityID),
		pk.UUID(p.UUID),
		pk.VarInt(entity.Zombie.ID),
		pk.Double(pos.X), pk.Double(pos.Y), pk.Double(pos.Z),
		// Minecraft 26.1 AddEntity format: movement Vec3.LP_STREAM_CODEC
		// is encoded immediately after position, before rotations. Zero
		// movement is encoded by LpVec3 as a single byte 0.
		pk.Byte(0),
		pk.Angle(degToAngle(rot.Pitch)),
		pk.Angle(degToAngle(rot.Yaw)),
		pk.Angle(degToAngle(rot.Yaw)),
		pk.VarInt(0),
	))
	if err != nil {
		log.Printf("[Java] failed to spawn Bedrock avatar %s: %v", p.Username, err)
	}
}

func (s *PlayerSession) moveForeignAvatar(p player.PlayerSnapshot) {
	if p.UUID == s.UUID || p.EntityRuntimeID == 0 {
		return
	}
	entityID := int32(p.EntityRuntimeID)
	pos := p.Position
	rot := p.Rotation
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameTeleportEntity,
		pk.VarInt(entityID),
		pk.Double(pos.X), pk.Double(pos.Y), pk.Double(pos.Z),
		pk.Double(0), pk.Double(0), pk.Double(0),
		pk.Float(rot.Yaw), pk.Float(rot.Pitch),
		// Relative.SET_STREAM_CODEC encoded as packed int. 0 = absolute teleport.
		pk.Int(0),
		pk.Boolean(p.OnGround),
	))
}

func (s *PlayerSession) removeForeignAvatar(p player.PlayerSnapshot) {
	if p.UUID == s.UUID || p.EntityRuntimeID == 0 {
		return
	}
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameRemoveEntities,
		pk.Ary[pk.VarInt]{Ary: []pk.VarInt{pk.VarInt(p.EntityRuntimeID)}},
	))
}

func degToAngle(deg float32) pk.Byte {
	v := int(math.Round(float64(deg) * 256.0 / 360.0))
	return pk.Byte(int8(v & 0xff))
}
