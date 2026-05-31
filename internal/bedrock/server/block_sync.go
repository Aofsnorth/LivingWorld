package server

import (
	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/world"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func (s *Server) startBlockEventLoop() {
	ch := s.wm.SubscribeBlockUpdates("bedrock-server", 256)
	go func() {
		for ev := range ch {
			s.broadcastBlockUpdate(ev)
		}
	}()
}

func (s *Server) broadcastBlockUpdate(ev world.BlockUpdateEvent) {
	rid := bedrockworld.LivingWorldBlockIDToBedrockRID(ev.BlockID)
	s.forEachSession(func(bs *bedrockSession) {
		bs.write(&packet.UpdateBlock{
			Position:          protocol.BlockPos{int32(ev.X), int32(ev.Y), int32(ev.Z)},
			NewBlockRuntimeID: rid,
			Flags:             packet.BlockUpdateNetwork | packet.BlockUpdateNeighbours,
			Layer:             0,
		})

		// Also update the cached chunk block if it exists in the session's cache.
		cx := int32(ev.X) >> 4
		cz := int32(ev.Z) >> 4
		pos := protocol.ChunkPos{cx, cz}
		bs.chunkCache.Mu.Lock()
		if ch, ok := bs.chunkCache.Cache[pos]; ok {
			lx := uint8(ev.X & 15)
			ly := int16(ev.Y)
			lz := uint8(ev.Z & 15)
			ch.SetBlock(lx, ly, lz, 0, rid)
		}
		bs.chunkCache.Mu.Unlock()
	})
}
