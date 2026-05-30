package protocol

import (
	"bytes"
	"log"
	"math"

	"livingworld/internal/player"
	"livingworld/internal/skinbridge"
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/data/entity"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

type Handler775 struct{}

func init() {
	RegisterVersion(&Handler775{})
}

func (h *Handler775) ProtocolVersion() int {
	return 775
}

func (h *Handler775) ProtocolName() string {
	return "26.1"
}

func (h *Handler775) SendInitialPlayPackets(s Session) error {
	conn := s.Conn()

	log.Printf("[Java] Sending Login packet (ID=%d) to %s", packetid.ClientboundGameLogin, s.Username())
	
	// Step 1: Send Login packet
	if err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundGameLogin,
		pk.Int(s.EntityID()),
		pk.Boolean(false),                  // isHardcore
		pk.VarInt(1),                      // count of world names
		pk.Identifier("minecraft:overworld"), // worldNames[0]
		pk.VarInt(2),                      // maxPlayers
		pk.VarInt(10),                     // viewDistance
		pk.VarInt(10),                     // simulationDistance
		pk.Boolean(false),                 // reducedDebugInfo
		pk.Boolean(true),                  // enableRespawnScreen
		pk.Boolean(false),                 // doLimitedCrafting
		// World State (SpawnInfo)
		pk.VarInt(0),                      // SpawnInfo.dimension (dimension type ID in registry)
		pk.Identifier("minecraft:overworld"), // SpawnInfo.name (dimension name)
		pk.Long(s.Config().World.Seed),  // SpawnInfo.hashedSeed
		pk.Byte(pk.Byte(s.GameMode())),      // SpawnInfo.gamemode (matches session)
		pk.UnsignedByte(255),              // SpawnInfo.previousGamemode (none = 255)
		pk.Boolean(false),                 // SpawnInfo.isDebug
		pk.Boolean(false),                 // SpawnInfo.isFlat
		pk.Boolean(false),                 // SpawnInfo.death (has last death location) - false
		pk.VarInt(0),                      // SpawnInfo.portalCooldown
		pk.VarInt(63),                      // SpawnInfo.seaLevel
		// enforcesSecureChat
		pk.Boolean(false),                 // enforcesSecureChat
	)); err != nil {
		log.Printf("[Java] Login packet send error: %v", err)
		return err
	}
	log.Printf("[Java] Login packet sent")

	// Step 2: Send GameEvent - "Start waiting for level chunks" (event 13)
	// Required in 1.20.2+ to signal the client to start accepting chunk data
	_ = conn.WritePacket(pk.Marshal(
		packetid.ClientboundGameGameEvent,
		pk.UnsignedByte(13), // event: start waiting for level chunks
		pk.Float(0),         // value (unused for this event)
	))

	// Step 3: Send PlayerPosition (use the session's terrain-aware spawn, not a
	// hardcoded y that would drop the player out of the sky).
	log.Printf("[Java] Sending PlayerPosition (ID=%d)", packetid.ClientboundGamePlayerPosition)
	_ = s.SendSpawnPosition()

	// Step 4: Send SetHealth with a full hunger bar.
	log.Printf("[Java] Sending SetHealth (ID=%d)", packetid.ClientboundGameSetHealth)
	_ = s.SendHealth()

	log.Printf("[Java] All initial play packets sent to %s", s.Username())

	// Send initial chunks around spawn to prevent client stuck on loading terrain
	s.UpdateChunks()

	// World state (default spawn + time of day). Without a time packet the
	// client never initializes its sky and renders it black.
	s.SendWorldState()

	return nil
}

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
	if p.Edition == player.EditionBedrock {
		// Use MineSkin properties if they are already populated via UpdateProfileProperty.
		// Otherwise, fallback to the local HTTP server (which works for some clients but not official Java).
		if len(props) == 0 && p.BedrockSkinURL != "" {
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
	// this down from the old index 17 (which is now the absorption Float — sending
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

func (h *Handler775) HandlePacket(s Session, p pk.Packet) {
	switch packetid.ServerboundPacketID(p.ID) {
	case packetid.ServerboundGameMovePlayerPos:
		s.HandleMovePos(p)
	case packetid.ServerboundGameMovePlayerPosRot:
		s.HandleMovePosRot(p)
	case packetid.ServerboundGameMovePlayerRot:
		s.HandleMoveRot(p)
	case packetid.ServerboundGameMovePlayerStatusOnly:
		s.HandleMoveStatusOnly(p)
	case packetid.ServerboundGamePlayerAction:
		s.HandlePlayerAction(p)
	case packetid.ServerboundGameUseItemOn:
		s.HandleUseItemOn(p)
	case packetid.ServerboundGameChat:
		s.HandleChat(p)
	case packetid.ServerboundGameChatCommand:
		s.HandleChatCommand(p)
	case packetid.ServerboundGameSetCreativeModeSlot:
		s.HandleCreativeSlot(p)
	case packetid.ServerboundGameSetCarriedItem:
		s.HandleSetCarriedItem(p)
	case packetid.ServerboundGameSwing:
		s.HandleSwing(p)
	case packetid.ServerboundGamePlayerCommand:
		s.HandlePlayerCommand(p)
	case packetid.ServerboundGamePlayerInput:
		var flags pk.Byte
		if err := p.Scan(&flags); err == nil {
			sneaking := (flags & 0x20) != 0
			s.PlayerManager().UpdateSneak(s.UUID(), sneaking)
		}
	case packetid.ServerboundGameKeepAlive:
	case packetid.ServerboundGameAcceptTeleportation:
	case packetid.ServerboundGamePlayerAbilities:
	case packetid.ServerboundGamePlayerLoaded:
	case packetid.ServerboundGameChunkBatchReceived:
	case packetid.ServerboundGameClientInformation:
		var (
			locale        pk.String
			viewDist      pk.Byte
			chatMode      pk.VarInt
			chatColors    pk.Boolean
			skinParts     pk.Byte
			mainHand      pk.VarInt
			textFiltering pk.Boolean
			allowListings pk.Boolean
		)
		if err := p.Scan(&locale, &viewDist, &chatMode, &chatColors, &skinParts, &mainHand, &textFiltering, &allowListings); err == nil {
			s.PlayerManager().UpdateSkinParts(s.UUID(), byte(skinParts))
		}
	case packetid.ServerboundGamePingRequest:
	case packetid.ServerboundGamePong:
	case packetid.ServerboundGameClientTickEnd:
	case packetid.ServerboundGameContainerClose:
	case packetid.ServerboundGameContainerSlotStateChanged:
	default:
	}
}

func degToAngle(deg float32) pk.Byte {
	v := int(math.Round(float64(deg) * 256.0 / 360.0))
	return pk.Byte(int8(v & 0xff))
}
