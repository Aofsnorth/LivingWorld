package java

import (
	"bytes"
	"log"
	"math"

	"livingworld/internal/player"

	"github.com/Tnze/go-mc/data/entity"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

const javaPlayerInfoAddPlayerBit = 0x01

func (j *javaBridge) startPlayerEventLoop() {
	if j.playerEvents != nil {
		return
	}
	j.playerEvents = j.pm.Subscribe("java-bridge", 256)
	go func() {
		for ev := range j.playerEvents {
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
		if p.UUID != s.UUID {
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

	if err := s.sendPlayerInfoAdd(p); err != nil {
		log.Printf("[Java] failed to send PlayerInfoUpdate for %s: %v", p.Username, err)
		return
	}

	err := s.SendPacket(pk.Marshal(
		packetid.ClientboundGameAddEntity,
		pk.VarInt(entityID),
		pk.UUID(p.UUID),
		pk.VarInt(entity.Player.ID),
		pk.Double(pos.X), pk.Double(pos.Y), pk.Double(pos.Z),
		pk.Byte(0), // Vec3.LP_STREAM_CODEC zero movement.
		pk.Angle(degToAngle(rot.Pitch)),
		pk.Angle(degToAngle(rot.Yaw)),
		pk.Angle(degToAngle(rot.Yaw)),
		pk.VarInt(0),
	))
	if err != nil {
		log.Printf("[Java] failed to spawn player entity %s: %v", p.Username, err)
		return
	}
}

func (s *PlayerSession) sendPlayerInfoAdd(p player.PlayerSnapshot) error {
	var buf bytes.Buffer
	// ClientboundPlayerInfoUpdatePacket 26.1:
	// actions EnumSet<Action> -> fixed bitset for 8 actions = one byte.
	// entries list -> VarInt length, then UUID, then ADD_PLAYER payload.
	_, _ = pk.Byte(javaPlayerInfoAddPlayerBit).WriteTo(&buf)
	_, _ = pk.VarInt(1).WriteTo(&buf)
	_, _ = pk.UUID(p.UUID).WriteTo(&buf)
	_, _ = pk.String(p.Username).WriteTo(&buf) // ByteBufCodecs.PLAYER_NAME
	props := p.ProfileProperties
	// Unsigned Bedrock skin texture properties are not accepted reliably by all
	// Java clients. Keep the profile property codec exact and only pass through
	// real Java properties for now to avoid disconnects.
	if p.Edition == player.EditionBedrock {
		props = nil
	}
	_, _ = pk.VarInt(len(props)).WriteTo(&buf)
	for _, prop := range props {
		_, _ = pk.String(prop.Name).WriteTo(&buf)
		_, _ = pk.String(prop.Value).WriteTo(&buf)
		// 26.1 GAME_PROFILE_PROPERTIES uses optional signature.
		if prop.Signature != "" {
			_, _ = pk.Boolean(true).WriteTo(&buf)
			_, _ = pk.String(prop.Signature).WriteTo(&buf)
		} else {
			_, _ = pk.Boolean(false).WriteTo(&buf)
		}
	}
	return s.SendPacket(pk.Packet{ID: int32(packetid.ClientboundGamePlayerInfoUpdate), Data: buf.Bytes()})
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
		pk.Int(0),
		pk.Boolean(p.OnGround),
	))
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameRotateHead,
		pk.VarInt(entityID),
		pk.Angle(degToAngle(rot.Yaw)),
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
	_ = s.sendPlayerInfoRemove(p)
}

func (s *PlayerSession) sendPlayerInfoRemove(p player.PlayerSnapshot) error {
	var buf bytes.Buffer
	_, _ = pk.VarInt(1).WriteTo(&buf)
	_, _ = pk.UUID(p.UUID).WriteTo(&buf)
	return s.SendPacket(pk.Packet{ID: int32(packetid.ClientboundGamePlayerInfoRemove), Data: buf.Bytes()})
}

func degToAngle(deg float32) pk.Byte {
	v := int(math.Round(float64(deg) * 256.0 / 360.0))
	return pk.Byte(int8(v & 0xff))
}
