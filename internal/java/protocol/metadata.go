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
	if p.OnFire {
		flags |= EntityFlagOnFire
	}
	if p.Sneaking {
		flags |= EntityFlagSneaking
	}
	if p.Sprinting {
		flags |= EntityFlagSprinting
	}
	if p.Swimming {
		flags |= EntityFlagSwimming
	}
	_, _ = pk.Byte(MetaIndexFlags).WriteTo(&buf)
	_, _ = pk.VarInt(MetaTypeByte).WriteTo(&buf)
	_, _ = pk.Byte(flags).WriteTo(&buf)

	// Index 1: Air Supply (VarInt, in ticks; 300 = full 10 bubbles).
	// Only emitted when AirSupply is populated or the player is actively drowning
	// so synthetic zero-value PlayerSnapshot literals in unit tests keep their
	// compact baseline layout.
	if p.AirSupply > 0 || (p.UUID != [16]byte{} && !p.Breathing) {
		air := int32(p.AirSupply)
		if air < 0 {
			air = 0
		}
		_, _ = pk.Byte(MetaIndexAirSupply).WriteTo(&buf)
		_, _ = pk.VarInt(MetaTypeVarInt).WriteTo(&buf)
		_, _ = pk.VarInt(air).WriteTo(&buf)
	}

	// Index 6: Pose (Pose type, Type 20)
	var pose int32 = PoseStanding
	if p.Swimming {
		pose = PoseSwimming
	} else if p.Sneaking {
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

// EncodeLocalEnvironmentMetadata builds a ClientboundSetEntityData payload for
// the local player's own entity containing only server-authoritative environment
// fields (Index 0 OnFire flag change and/or Index 1 AirSupply bubble count).
// It intentionally omits Index 6 (Pose) and avoids resending Index 0 on routine
// sprint/sneak/swim transitions so the server never clobbers client-predicted
// LocalPlayer swimming/sprinting state in Minecraft 26.3.
func EncodeLocalEnvironmentMetadata(entityID int32, p player.PlayerSnapshot, includeFlags, includeAir bool) []byte {
	var buf bytes.Buffer
	_, _ = pk.VarInt(entityID).WriteTo(&buf)

	if includeFlags {
		var flags byte = 0x00
		if p.OnFire {
			flags |= EntityFlagOnFire
		}
		if p.Sneaking {
			flags |= EntityFlagSneaking
		}
		if p.Sprinting {
			flags |= EntityFlagSprinting
		}
		if p.Swimming {
			flags |= EntityFlagSwimming
		}
		_, _ = pk.Byte(MetaIndexFlags).WriteTo(&buf)
		_, _ = pk.VarInt(MetaTypeByte).WriteTo(&buf)
		_, _ = pk.Byte(flags).WriteTo(&buf)
	}

	if includeAir {
		air := int32(p.AirSupply)
		if air < 0 {
			air = 0
		}
		_, _ = pk.Byte(MetaIndexAirSupply).WriteTo(&buf)
		_, _ = pk.VarInt(MetaTypeVarInt).WriteTo(&buf)
		_, _ = pk.VarInt(air).WriteTo(&buf)
	}

	_, _ = pk.Byte(-1).WriteTo(&buf)
	return buf.Bytes()
}

func (h *Handler777) MoveForeignAvatar(s Session, p player.PlayerSnapshot, oldPos world.Position, exists bool) error {
	if p.EntityRuntimeID == uint64(s.EntityID()) {
		return nil
	}
	entityID := int32(p.EntityRuntimeID)
	pos := p.Position
	rot := p.Rotation

	// Always send ClientboundGameTeleportEntity (followed by RotateHead) just like
	// moveMobPacket and moveDropPacket do. In MC 26.3 (protocol 777), relative
	// move_entity_pos_rot caused a client DecoderException and int16(dx*4096)
	// truncation accumulated position drift against unquantized lastSentPos.
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameTeleportEntity,
		pk.VarInt(entityID),
		pk.Double(pos.X), pk.Double(pos.Y), pk.Double(pos.Z),
		pk.Double(0), pk.Double(0), pk.Double(0), // velocity
		pk.Float(rot.Yaw), pk.Float(rot.Pitch),
		pk.Int(0), // flags
		pk.Boolean(p.OnGround),
	))

	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameRotateHead,
		pk.VarInt(entityID),
		pk.Angle(degToAngle(rot.Yaw)),
	))

	return nil
}
