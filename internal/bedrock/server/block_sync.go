package server

import (
	"livingworld/internal/world"
	bedrockworld "livingworld/internal/bedrock/world"

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
	})
}
