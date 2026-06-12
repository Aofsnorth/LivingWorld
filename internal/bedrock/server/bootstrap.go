package server

import (
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
	_ = conn.WritePacket(&packet.ChunkRadiusUpdated{ChunkRadius: int32(radius)})
	_ = conn.WritePacket(&packet.NetworkChunkPublisherUpdate{Position: spawn, Radius: uint32(radius * 16)})

	// Initialize lastChunk coordinates so first move checks against spawn chunk.
	// Use the locked setter — the AOI reconcile may read these from the
	// player-event-loop goroutine while this join goroutine is still bootstrapping.
	bs.setChunkCenter(centerX, centerZ)

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

	// Re-center the network chunk publisher on the player's CURRENT chunk. The
	// publisher radius gates what the client will render, so it must track the
	// player; derive it from the floor-correct chunk coords (cx,cz) we were given
	// rather than int32(lastX), whose truncation + the old spawn-(0,0) fallback
	// kept the publisher pinned near spawn and culled far chunks the client had
	// already received. Center on the chunk's block midpoint.
	pos := protocol.BlockPos{cx*16 + 8, int32(bs.lastY), cz*16 + 8}
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
