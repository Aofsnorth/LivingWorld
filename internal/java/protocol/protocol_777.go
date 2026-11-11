package protocol

import (
	"log"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

type Handler777 struct{}

func init() {
	RegisterVersion(&Handler777{})
}

func (h *Handler777) ProtocolVersion() int {
	return 777
}

func (h *Handler777) ProtocolName() string {
	return "26.3"
}

func (h *Handler777) SendInitialPlayPackets(s Session) error {
	conn := s.Conn()

	log.Printf("[Java] Sending Login packet (ID=%d) to %s", packetid.ClientboundGameLogin, s.Username())

	// Step 1: Send Login packet
	if err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundGameLogin,
		pk.Int(s.EntityID()),
		pk.Boolean(false),                                    // isHardcore
		pk.VarInt(1),                                         // count of world names
		pk.Identifier("minecraft:overworld"),                 // worldNames[0]
		pk.VarInt(2),                                         // maxPlayers
		pk.VarInt(int32(s.Config().Java.ViewDistance)),       // viewDistance (from config)
		pk.VarInt(int32(s.Config().Java.SimulationDistance)), // simulationDistance (from config)
		pk.Boolean(false),                                    // reducedDebugInfo
		pk.Boolean(true),                                     // enableRespawnScreen
		pk.Boolean(false),                                    // doLimitedCrafting
		// World State (SpawnInfo)
		pk.VarInt(0),                         // SpawnInfo.dimension (dimension type ID in registry)
		pk.Identifier("minecraft:overworld"), // SpawnInfo.name (dimension name)
		pk.Long(s.Config().World.Seed),       // SpawnInfo.hashedSeed
		pk.VarInt(int32(s.GameMode())),       // SpawnInfo.gamemode (GameType.STREAM_CODEC = VarInt)
		pk.VarInt(0),                         // SpawnInfo.previousGamemode (ByteBufCodecs.OPTIONAL_VAR_INT: 0 = empty)
		pk.Boolean(false),                    // SpawnInfo.isDebug
		pk.Boolean(false),                    // SpawnInfo.isFlat
		pk.Boolean(false),                    // SpawnInfo.death (has last death location) - false
		pk.VarInt(0),                         // SpawnInfo.portalCooldown
		pk.VarInt(63),                        // SpawnInfo.seaLevel
		// 26.3 ClientboundLoginPacket trailing booleans
		pk.Boolean(s.Config().Java.OnlineMode), // onlineMode
		pk.Boolean(false),                      // enforcesSecureChat
	)); err != nil {
		log.Printf("[Java] Login packet send error: %v", err)
		return err
	}
	log.Printf("[Java] Login packet sent")

	// Send PlayerAbilities so the client knows its exact instabuild/flying
	// flags for the current gamemode (prevents survival clients from predicting
	// creative instant-break or vice versa).
	var abilityFlags byte = 0x00
	switch s.GameMode() {
	case 1: // Creative: invulnerable (0x01) | allowFlying (0x04) | instabuild (0x08)
		abilityFlags = 0x0D
	case 3: // Spectator: invulnerable (0x01) | flying (0x02) | allowFlying (0x04)
		abilityFlags = 0x07
	}
	_ = conn.WritePacket(pk.Marshal(
		packetid.ClientboundGamePlayerAbilities,
		pk.Byte(abilityFlags),
		pk.Float(0.05), // flyingSpeed
		pk.Float(0.1),  // fieldOfViewModifier (walking speed)
	))

	// Size the client's chunk-storage + simulation radius to exactly what the
	// server streams. Without these the client can keep its storage at a smaller
	// default and DISCARD chunks beyond it; as the player walks those discarded
	// chunks re-enter range but were never re-sent (server thinks they're loaded),
	// so terrain vanishes on one side approaching the player.
	_ = conn.WritePacket(pk.Marshal(
		packetid.ClientboundGameSetChunkCacheRadius,
		pk.VarInt(int32(s.Config().Java.ViewDistance)),
	))
	_ = conn.WritePacket(pk.Marshal(
		packetid.ClientboundGameSetSimulationDistance,
		pk.VarInt(int32(s.Config().Java.SimulationDistance)),
	))

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
