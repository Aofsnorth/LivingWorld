package java

import (
	"log"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/nbt"
)

func (s *PlayerSession) sendInitialPlayPackets() error {
	conn := s.Conn

	log.Printf("[Java] Sending Login packet (ID=%d) to %s", packetid.ClientboundGameLogin, s.Username)
	
	// Step 1: Send Login packet
	if err := conn.WritePacket(pk.Marshal(
		packetid.ClientboundGameLogin,
		pk.Int(s.EntityID),
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
		pk.Long(s.Bridge.cfg.World.Seed),  // SpawnInfo.hashedSeed
		pk.Byte(pk.Byte(s.GameMode)),      // SpawnInfo.gamemode (matches session)
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
	_ = s.sendSpawnPosition()

	// Step 4: Send SetHealth with a full hunger bar.
	log.Printf("[Java] Sending SetHealth (ID=%d)", packetid.ClientboundGameSetHealth)
	_ = s.sendHealth()

	log.Printf("[Java] All initial play packets sent to %s", s.Username)

	// Send initial chunks around spawn to prevent client stuck on loading terrain
	s.updateChunks()

	// World state (default spawn + time of day). Without a time packet the
	// client never initializes its sky and renders it black.
	s.sendWorldState()

	return nil
}

func (s *PlayerSession) getChunkCount() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int32(len(s.LoadedChunks))
}

func (s *PlayerSession) sendPlayerInfo() {}

func buildSystemChatPacket(text string, actionBar bool) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameSystemChat,
		pk.NBT(nbt.RawMessage{Type: nbt.TagCompound, Data: func() []byte {
			b, _ := nbt.Marshal(chatText{Text: text})
			return b[3:]
		}()}),
		pk.Boolean(actionBar),
	)
}