package server

import (
	"log"

	lwworld "livingworld/internal/world"

	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func (s *Server) bootstrapWorld(conn *minecraft.Conn, radius int, bs *bedrockSession) {
	bs.chunkMu.Lock()
	bs.viewDistance = int32(radius)
	bs.chunkMu.Unlock()

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

	// Initialize lastChunk coordinates so first move checks against spawn chunk
	bs.lastChunkX = centerX
	bs.lastChunkZ = centerZ

	// Load spawn area chunks and register them as loaded for this session
	s.updateBedrockChunks(bs, centerX, centerZ)

	s.sendInventory(conn)
}

func (s *Server) updateBedrockChunks(bs *bedrockSession, cx, cz int32) {
	bs.chunkMu.Lock()
	radius := bs.viewDistance
	bs.chunkMu.Unlock()

	if radius <= 0 {
		radius = int32(s.cfg.Bedrock.ViewDistance)
	}

	// Update the network chunk publisher center to the player's new position
	px, py, pz := int32(bs.lastX), int32(bs.lastY), int32(bs.lastZ)
	// If lastX is 0 (spawning), fallback to spawn block coordinates
	if px == 0 && pz == 0 {
		px = int32(s.cfg.World.Spawn.X)
		py = int32(lwworld.SuperflatSpawnY)
		pz = int32(s.cfg.World.Spawn.Z)
	}
	pos := protocol.BlockPos{px, py, pz}
	_ = bs.conn.WritePacket(&packet.NetworkChunkPublisherUpdate{
		Position: pos,
		Radius:   uint32(radius * 16),
	})

	// Clean up (unload) chunks that have drifted out of the player's view distance
	bs.chunkMu.Lock()
	for pos := range bs.LoadedChunks {
		dx := pos.X() - cx
		dz := pos.Z() - cz
		if dx < -radius || dx > radius || dz < -radius || dz > radius {
			delete(bs.LoadedChunks, pos)
		}
	}
	bs.chunkMu.Unlock()

	// Load and send new chunks in view distance
	w := s.wm.GetDefaultWorld()
	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			pos := protocol.ChunkPos{cx + dx, cz + dz}
			bs.chunkMu.Lock()
			isLoaded := bs.LoadedChunks[pos]
			if !isLoaded {
				bs.LoadedChunks[pos] = true
			}
			bs.chunkMu.Unlock()

			if !isLoaded {
				s.converter.SendChunk(bs.conn, w, int(pos.X()), int(pos.Z()), bs.chunkCache)
			}
		}
	}
}

func (s *Server) sendInventory(conn *minecraft.Conn) {
	_ = conn.WritePacket(&packet.InventoryContent{WindowID: protocol.WindowIDInventory, Content: make([]protocol.ItemInstance, 36)})
	_ = conn.WritePacket(&packet.InventoryContent{WindowID: protocol.WindowIDOffHand, Content: make([]protocol.ItemInstance, 1)})
	_ = conn.WritePacket(&packet.InventoryContent{WindowID: protocol.WindowIDArmour, Content: make([]protocol.ItemInstance, 4)})
}
