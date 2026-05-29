package bedrock

import (
	"log"

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
		int32(s.cfg.World.Spawn.Y),
		int32(s.cfg.World.Spawn.Z),
	}
	if radius <= 0 {
		radius = s.cfg.Bedrock.ViewDistance
	}
	log.Printf("[Bedrock] Sending world bootstrap radius=%d centerChunk=(%d,%d) spawnY=%.1f", radius, centerX, centerZ, s.cfg.World.Spawn.Y)
	_ = conn.WritePacket(&packet.ChunkRadiusUpdated{ChunkRadius: int32(radius)})
	_ = conn.WritePacket(&packet.NetworkChunkPublisherUpdate{
		Position: spawn,
		Radius:   uint32(radius * 16),
	})
	// Use the superflat generator's ground Y (from config: Y=3)
	groundY := int16(3)
	s.converter.sendInitialChunks(conn, int(centerX), int(centerZ), radius, groundY, chunkCache)
	s.sendInventory(conn)
}

func (s *Server) sendInventory(conn *minecraft.Conn) {
	_ = conn.WritePacket(&packet.InventoryContent{
		WindowID: protocol.WindowIDInventory,
		Content:  make([]protocol.ItemInstance, 36),
	})
	_ = conn.WritePacket(&packet.InventoryContent{
		WindowID: protocol.WindowIDOffHand,
		Content:  make([]protocol.ItemInstance, 1),
	})
	_ = conn.WritePacket(&packet.InventoryContent{
		WindowID: protocol.WindowIDArmour,
		Content:  make([]protocol.ItemInstance, 4),
	})
	_ = conn.WritePacket(&packet.CreativeContent{})
}
