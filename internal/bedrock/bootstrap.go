package bedrock

import (
	"log"

	lwworld "livingworld/internal/world"

	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
)

func (s *Server) bootstrapWorld(conn *minecraft.Conn, radius int, chunkCache map[protocol.ChunkPos]*dfchunk.Chunk) {
	centerX := int32(s.cfg.World.Spawn.X) >> 4
	centerZ := int32(s.cfg.World.Spawn.Z) >> 4
	spawn := protocol.BlockPos{
		int32(s.cfg.World.Spawn.X),
		int32(lwworld.SuperflatSpawnY),
		int32(s.cfg.World.Spawn.Z),
	}
	log.Printf("[Bedrock] Sending world bootstrap radius=%d centerChunk=(%d,%d) groundY=%d", radius, centerX, centerZ, bedrockGroundY)
	_ = conn.WritePacket(&packet.ChunkRadiusUpdated{ChunkRadius: int32(radius)})
	_ = conn.WritePacket(&packet.NetworkChunkPublisherUpdate{Position: spawn, Radius: uint32(radius * 16)})
	s.converter.sendInitialChunks(conn, int(centerX), int(centerZ), radius, bedrockGroundY, chunkCache)
	s.sendInventory(conn)
}

func (s *Server) sendInventory(conn *minecraft.Conn) {
	_ = conn.WritePacket(&packet.InventoryContent{WindowID: protocol.WindowIDInventory, Content: make([]protocol.ItemInstance, 36)})
	_ = conn.WritePacket(&packet.InventoryContent{WindowID: protocol.WindowIDOffHand, Content: make([]protocol.ItemInstance, 1)})
	_ = conn.WritePacket(&packet.InventoryContent{WindowID: protocol.WindowIDArmour, Content: make([]protocol.ItemInstance, 4)})
}
