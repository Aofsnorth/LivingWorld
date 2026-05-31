package protocol

import (
	"bytes"

	"livingworld/internal/player"
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// encodePlayerMetadata builds the body of a ClientboundSetEntityData packet for a
// player avatar following the MC 26.1 (protocol 775) entity-data layout. Kept as a
// pure function (no Session) so the wire layout can be unit-tested.
func encodePlayerMetadata(p player.PlayerSnapshot) []byte {
	entityID := int32(p.EntityRuntimeID)
	var buf bytes.Buffer
	_, _ = pk.VarInt(entityID).WriteTo(&buf)

	// Index 0: Entity Flags (Byte)
	var flags byte = 0x00
	if p.Sneaking {
		flags |= EntityFlagSneaking
	}
	_, _ = pk.Byte(MetaIndexFlags).WriteTo(&buf)
	_, _ = pk.VarInt(MetaTypeByte).WriteTo(&buf)
	_, _ = pk.Byte(flags).WriteTo(&buf)

	// Index 6: Pose (Pose type, Type 20)
	var pose int32 = PoseStanding
	if p.Sneaking {
		pose = PoseCrouching
	}
	_, _ = pk.Byte(MetaIndexPose).WriteTo(&buf)
	_, _ = pk.VarInt(MetaTypePose).WriteTo(&buf)
	_, _ = pk.VarInt(pose).WriteTo(&buf)

	// Index 16: Displayed Skin Parts (Byte). In MC 26.1 the Avatar class shifted
	// this down from the old index 17 (which is now the absorption Float â€” sending
	// a Byte there crashes the client with "Invalid entity data item type").
	_, _ = pk.Byte(MetaIndexSkinParts).WriteTo(&buf)
	_, _ = pk.VarInt(MetaTypeByte).WriteTo(&buf)
	_, _ = pk.Byte(p.SkinParts).WriteTo(&buf)

	// Terminate metadata
	_, _ = pk.Byte(-1).WriteTo(&buf)

	return buf.Bytes()
}

func (h *Handler775) MoveForeignAvatar(s Session, p player.PlayerSnapshot, oldPos world.Position, exists bool) error {
	if p.EntityRuntimeID == uint64(s.EntityID()) {
		return nil
	}
	entityID := int32(p.EntityRuntimeID)
	pos := p.Position
	rot := p.Rotation

	dx := pos.X - oldPos.X
	dy := pos.Y - oldPos.Y
	dz := pos.Z - oldPos.Z
	distSq := dx*dx + dy*dy + dz*dz

	isTeleport := !exists || distSq > 64.0 // > 8 blocks distance

	if isTeleport {
		_ = s.SendPacket(pk.Marshal(
			packetid.ClientboundGameTeleportEntity,
			pk.VarInt(entityID),
			pk.Double(pos.X), pk.Double(pos.Y), pk.Double(pos.Z),
			pk.Double(0), pk.Double(0), pk.Double(0), // velocity
			pk.Float(rot.Yaw), pk.Float(rot.Pitch),
			pk.Int(0), // flags
			pk.Boolean(p.OnGround),
		))
	} else {
		// Use relative move!
		deltaX := int16(dx * 4096)
		deltaY := int16(dy * 4096)
		deltaZ := int16(dz * 4096)

		_ = s.SendPacket(pk.Marshal(
			packetid.ClientboundGameMoveEntityPosRot,
			pk.VarInt(entityID),
			pk.Short(deltaX), pk.Short(deltaY), pk.Short(deltaZ),
			pk.Angle(degToAngle(rot.Yaw)),
			pk.Angle(degToAngle(rot.Pitch)),
			pk.Boolean(p.OnGround),
		))
	}

	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameRotateHead,
		pk.VarInt(entityID),
		pk.Angle(degToAngle(rot.Yaw)),
	))

	return nil
}
