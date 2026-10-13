package server

import (
	"log"

	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/world"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// mustBedrockRID resolves a canonical block ID for direct-packet use and logs
// loudly when the mapping fails, since such a block would be sent with the
// registry's fallback RID (0 = cyan_terracotta, NOT air).
func mustBedrockRID(id int32) uint32 {
	rid, ok := bedrockworld.LivingWorldBlockIDToBedrockRID(id)
	if !ok {
		log.Printf("[Bedrock] WARNING: block id %d has no Bedrock mapping; sending fallback RID %d", id, rid)
	}
	return rid
}

func (s *Server) startBlockEventLoop() {
	ch := s.wm.SubscribeBlockUpdates("bedrock-server", 256)
	go func() {
		for ev := range ch {
			s.broadcastBlockUpdate(ev)
		}
	}()
}

func (s *Server) broadcastBlockUpdate(ev world.BlockUpdateEvent) {
	rid, _ := bedrockworld.LivingWorldBlockIDToBedrockRID(ev.BlockID)
	s.forEachSession(func(bs *bedrockSession) {
		bs.write(&packet.UpdateBlock{
			Position:          protocol.BlockPos{int32(ev.X), int32(ev.Y), int32(ev.Z)},
			NewBlockRuntimeID: rid,
			Flags:             packet.BlockUpdateNetwork | packet.BlockUpdateNeighbours,
			Layer:             0,
		})

		// Also update the cached chunk block if it exists in the session's cache.
		// MutateChunkBody takes the chunk body lock exclusively so this SetBlock
		// cannot race a concurrent EncodeSubChunk in HandleSubChunkRequest
		// (dfchunk.Chunk is not thread-safe).
		cx := int32(ev.X) >> 4
		cz := int32(ev.Z) >> 4
		pos := protocol.ChunkPos{cx, cz}
		lx := uint8(ev.X & 15)
		ly := int16(ev.Y)
		lz := uint8(ev.Z & 15)
		bs.chunkCache.MutateChunkBody(pos, func(ch *dfchunk.Chunk) {
			ch.SetBlock(lx, ly, lz, 0, rid)
		})
	})
}
