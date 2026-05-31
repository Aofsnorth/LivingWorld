package protocol

import (
	"bytes"
	"log"

	"livingworld/internal/player"
	"livingworld/internal/skinbridge"

	"github.com/Tnze/go-mc/data/entity"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

func (h *Handler775) SpawnForeignAvatar(s Session, p player.PlayerSnapshot) error {
	if p.UUID == s.UUID() || p.EntityRuntimeID == 0 {
		return nil
	}
	entityID := int32(p.EntityRuntimeID)
	pos := p.Position
	rot := p.Rotation

	if err := h.SendPlayerInfoAdd(s, p); err != nil {
		log.Printf("[Java] failed to send PlayerInfoUpdate for %s: %v", p.Username, err)
		return err
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
		return err
	}

	// Send initial metadata (including sneak status) so the spawned avatar renders correctly
	_ = h.UpdateForeignMetadata(s, p)

	return nil
}

const javaPlayerInfoAddPlayerBit = 0x01 | 0x04 | 0x08

func (h *Handler775) SendPlayerInfoAdd(s Session, p player.PlayerSnapshot) error {
	var buf bytes.Buffer
	_, _ = pk.Byte(javaPlayerInfoAddPlayerBit).WriteTo(&buf)
	_, _ = pk.VarInt(1).WriteTo(&buf)
	_, _ = pk.UUID(p.UUID).WriteTo(&buf)
	_, _ = pk.String(p.Username).WriteTo(&buf) // ByteBufCodecs.PLAYER_NAME
	props := p.ProfileProperties
	if p.Edition == player.EditionBedrock && p.BedrockSkinURL != "" {
		// With HD enabled, prefer the full-resolution local skin (fixes the 64×64
		// "burik" downscale). Otherwise use the local URL only as a fallback when
		// there is no signed MineSkin property, since strict clients reject
		// unsigned URLs and would show the default skin.
		if s.Config().Java.BedrockHDSkins || len(props) == 0 {
			name, val := skinbridge.TextureProperty(p.UUID, p.Username, p.BedrockSkinURL)
			props = []player.ProfileProperty{{Name: name, Value: val}}
		}
	}
	_, _ = pk.VarInt(len(props)).WriteTo(&buf)
	for _, prop := range props {
		_, _ = pk.String(prop.Name).WriteTo(&buf)
		_, _ = pk.String(prop.Value).WriteTo(&buf)
		if prop.Signature != "" {
			_, _ = pk.Boolean(true).WriteTo(&buf)
			_, _ = pk.String(prop.Signature).WriteTo(&buf)
		} else {
			_, _ = pk.Boolean(false).WriteTo(&buf)
		}
	}

	// Bit 2: UPDATE_GAME_MODE
	var gameMode int32 = 0 // Survival
	if p.Creative {
		gameMode = 1 // Creative
	}
	_, _ = pk.VarInt(gameMode).WriteTo(&buf)

	// Bit 3: UPDATE_LISTED
	_, _ = pk.Boolean(true).WriteTo(&buf)

	return s.SendPacket(pk.Packet{ID: int32(packetid.ClientboundGamePlayerInfoUpdate), Data: buf.Bytes()})
}

func (h *Handler775) SendPlayerInfoRemove(s Session, p player.PlayerSnapshot) error {
	var buf bytes.Buffer
	_, _ = pk.VarInt(1).WriteTo(&buf)
	_, _ = pk.UUID(p.UUID).WriteTo(&buf)
	return s.SendPacket(pk.Packet{ID: int32(packetid.ClientboundGamePlayerInfoRemove), Data: buf.Bytes()})
}

func (h *Handler775) RemoveForeignAvatar(s Session, p player.PlayerSnapshot) error {
	if p.EntityRuntimeID == uint64(s.EntityID()) {
		return nil
	}
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameRemoveEntities,
		pk.Ary[pk.VarInt]{Ary: []pk.VarInt{pk.VarInt(p.EntityRuntimeID)}},
	))
	return h.SendPlayerInfoRemove(s, p)
}

func (h *Handler775) SwingForeignAvatar(s Session, p player.PlayerSnapshot) error {
	if p.EntityRuntimeID == uint64(s.EntityID()) {
		return nil
	}
	return s.SendPacket(pk.Marshal(
		packetid.ClientboundGameAnimate,
		pk.VarInt(int32(p.EntityRuntimeID)), pk.UnsignedByte(0),
	))
}

func (h *Handler775) UpdateForeignMetadata(s Session, p player.PlayerSnapshot) error {
	return s.SendPacket(pk.Packet{
		ID:   int32(packetid.ClientboundGameSetEntityData),
		Data: encodePlayerMetadata(p),
	})
}

func (h *Handler775) UpdateForeignEquipment(s Session, p player.PlayerSnapshot) error {
	if p.UUID == s.UUID() {
		return nil // the client renders its own held item from its inventory
	}
	// Get held item from player
	pl := s.PlayerManager().GetPlayer(p.UUID)
	if pl == nil || pl.Inventory == nil {
		return nil
	}
	heldItem := pl.Inventory.GetHeldItem()

	// Build equipment packet: ClientboundGameSetEquipment
	// Format: VarInt entityID + equipment entries (slot Byte + ItemStack)
	var buf bytes.Buffer
	_, _ = pk.VarInt(p.EntityRuntimeID).WriteTo(&buf)

	// Slot 0 = main hand
	_, _ = pk.Byte(0).WriteTo(&buf)

	// ItemStack: count VarInt + itemID VarInt + components (nAdd VarInt + nRemove VarInt)
	if heldItem == nil || heldItem.ID == 0 {
		// Empty hand
		_, _ = pk.VarInt(0).WriteTo(&buf) // count = 0 (empty)
	} else {
		_, _ = pk.VarInt(heldItem.Count).WriteTo(&buf) // count
		_, _ = pk.VarInt(heldItem.ID).WriteTo(&buf)    // itemID
		_, _ = pk.VarInt(0).WriteTo(&buf)              // components: 0 add
		_, _ = pk.VarInt(0).WriteTo(&buf)              // components: 0 remove
	}

	return s.SendPacket(pk.Packet{
		ID:   int32(packetid.ClientboundGameSetEquipment),
		Data: buf.Bytes(),
	})
}
