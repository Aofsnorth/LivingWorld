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
	if p.Edition == player.EditionBedrock && !hasSignedTextures(props) && p.BedrockSkinURL != "" {
		// Always keep the signed MineSkin "textures" property as the baseline for
		// every viewer — vanilla Java only renders signed skins from Mojang's
		// whitelisted domains, so discarding it (the old `bedrockHDSkins` branch)
		// forced vanilla clients to the default Steve. Only when no signed property
		// exists yet — MineSkin upload still pending, or no API key configured — do
		// we fall back to the unsigned local HD URL, which only lenient
		// (authlib-injector) launchers accept.
		name, val := skinbridge.TextureProperty(p.UUID, p.Username, p.BedrockSkinURL)
		props = upsertUnsignedTextures(props, name, val)
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

// hasSignedTextures reports whether props already carries a SIGNED "textures"
// property — the Mojang-whitelisted MineSkin skin that vanilla Java will render.
func hasSignedTextures(props []player.ProfileProperty) bool {
	for _, pr := range props {
		if pr.Name == "textures" && pr.Signature != "" {
			return true
		}
	}
	return false
}

// upsertUnsignedTextures returns props with an unsigned "textures" property set to
// val (replacing any existing unsigned "textures" entry). It copies the slice so
// it never mutates the PlayerSnapshot's backing array, which is shared across the
// per-viewer fan-out in entity_sync.go.
func upsertUnsignedTextures(props []player.ProfileProperty, name, val string) []player.ProfileProperty {
	out := append([]player.ProfileProperty(nil), props...)
	for i := range out {
		if out[i].Name == name {
			out[i].Value = val
			out[i].Signature = ""
			return out
		}
	}
	return append(out, player.ProfileProperty{Name: name, Value: val})
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
	// Format (MC 26.1.2, decompiled from 26.1.2.jar):
	//
	//   VarInt   entityID
	//   (Equipment entry) ...
	//
	// where each equipment entry is:
	//
	//   byte     slot (0=main hand, 1=offhand, 2..5=armor)
	//   ItemStack:
	//     VarInt  count            (0 → empty stack, no more fields)
	//     VarInt  itemID           (only when count > 0)
	//     VarInt  componentsAdded    (only when count > 0, then 0 for none)
	//     VarInt  componentsRemoved  (only when count > 0, then 0 for none)
	//
	// The 26.1 wire format DOES NOT have an array-length prefix — the
	// equipment array is terminated by the end of the packet. The 1.21.1+
	// Mojang source claims a `readByte()` for length, but the actual 26.1.2
	// vanilla decoder on the wire does not consume it (and adding the byte
	// produces "found 1 bytes extra"). Sending the array without a length
	// is the format the 26.1.2 client accepts.
	var buf bytes.Buffer
	_, _ = pk.VarInt(p.EntityRuntimeID).WriteTo(&buf)

	// Slot 0 = main hand.
	_, _ = pk.Byte(0).WriteTo(&buf)

	// ItemStack: count VarInt + (itemID + components if count > 0).
	if heldItem == nil || heldItem.ID == 0 || heldItem.Count <= 0 {
		_, _ = pk.VarInt(0).WriteTo(&buf) // count = 0 → empty stack
	} else {
		_, _ = pk.VarInt(int32(heldItem.Count)).WriteTo(&buf)
		_, _ = pk.VarInt(heldItem.ID).WriteTo(&buf)
		_, _ = pk.VarInt(0).WriteTo(&buf) // components added
		_, _ = pk.VarInt(0).WriteTo(&buf) // components removed
	}

	return s.SendPacket(pk.Packet{
		ID:   int32(packetid.ClientboundGameSetEquipment),
		Data: buf.Bytes(),
	})
}
