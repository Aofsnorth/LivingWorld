package server

import (
	"sort"
	"time"

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
	// Use the real snapped spawn Y. The old hardcoded SuperflatSpawnY (4) is a
	// superflat-era leftover and put the publisher below terrain for overworld
	// spawns (only X/Z drive the radius, but Y is what the client anchors on).
	spawn := protocol.BlockPos{
		int32(s.cfg.World.Spawn.X),
		int32(s.cfg.World.Spawn.Y),
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
	var unloaded []protocol.ChunkPos
	bs.chunkMu.Lock()
	for pos := range bs.LoadedChunks {
		dx := pos.X() - cx
		dz := pos.Z() - cz
		if dx < -radius || dx > radius || dz < -radius || dz > radius {
			delete(bs.LoadedChunks, pos)
			unloaded = append(unloaded, pos)
		}
	}
	bs.chunkMu.Unlock()
	if len(unloaded) > 0 && bs.chunkCache != nil {
		bs.chunkCache.Mu.Lock()
		for _, pos := range unloaded {
			delete(bs.chunkCache.Cache, pos)
		}
		bs.chunkCache.Mu.Unlock()
	}

	// Load and send new chunks in view distance (nearest-first, preloaded in parallel).
	w := s.wm.GetDefaultWorld()
	var toSend []protocol.ChunkPos
	bs.chunkMu.Lock()
	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			pos := protocol.ChunkPos{cx + dx, cz + dz}
			if !bs.LoadedChunks[pos] {
				bs.LoadedChunks[pos] = true
				toSend = append(toSend, pos)
			}
		}
	}
	bs.chunkMu.Unlock()

	if len(toSend) == 0 {
		return
	}

	sort.Slice(toSend, func(i, j int) bool {
		dxi, dzi := toSend[i].X()-cx, toSend[i].Z()-cz
		dxj, dzj := toSend[j].X()-cx, toSend[j].Z()-cz
		return dxi*dxi+dzi*dzi < dxj*dxj+dzj*dzj
	})

	// Send the immediate 3x3 area (9 chunks) synchronously so the player spawns on
	// solid terrain right away, then stream outer rings progressively in a goroutine.
	immediate := min(9, len(toSend))
	firstBatch := toSend[:immediate]
	s.converter.PreloadChunks(w, firstBatch, bs.chunkCache)
	for _, pos := range firstBatch {
		s.converter.SendChunk(bs.conn, w, int(pos.X()), int(pos.Z()), bs.chunkCache)
	}

	rest := toSend[immediate:]
	if len(rest) > 0 {
		go func(remaining []protocol.ChunkPos, radius int32) {
			const batchSize = 16
			for start := 0; start < len(remaining); start += batchSize {
				curCX, curCZ := bs.chunkCenter()
				end := min(start+batchSize, len(remaining))
				var active []protocol.ChunkPos
				for _, pos := range remaining[start:end] {
					dx := pos.X() - curCX
					dz := pos.Z() - curCZ
					if dx >= -radius && dx <= radius && dz >= -radius && dz <= radius {
						active = append(active, pos)
					} else {
						bs.chunkMu.Lock()
						delete(bs.LoadedChunks, pos)
						bs.chunkMu.Unlock()
					}
				}
				if len(active) > 0 {
					s.converter.PreloadChunks(w, active, bs.chunkCache)
					for _, pos := range active {
						s.converter.SendChunk(bs.conn, w, int(pos.X()), int(pos.Z()), bs.chunkCache)
					}
				}
				if end < len(remaining) {
					time.Sleep(15 * time.Millisecond)
				}
			}
		}(rest, radius)
	}
}

func (s *Server) sendInventory(conn *minecraft.Conn) {
	_ = conn.WritePacket(&packet.InventoryContent{WindowID: protocol.WindowIDInventory, Content: make([]protocol.ItemInstance, 36)})
	_ = conn.WritePacket(&packet.InventoryContent{WindowID: protocol.WindowIDOffHand, Content: make([]protocol.ItemInstance, 1)})
	_ = conn.WritePacket(&packet.InventoryContent{WindowID: protocol.WindowIDArmour, Content: make([]protocol.ItemInstance, 4)})
}
