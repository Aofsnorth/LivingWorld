package bedrock

import (
	"livingworld/internal/world"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func livingWorldBlockIDToBedrockRID(id int32) uint32 {
	switch id {
	case 0:
		return blockRID("minecraft:air")
	case 1:
		return blockRID("minecraft:bedrock")
	case 2:
		return blockRID("minecraft:dirt")
	case 3:
		return blockRID("minecraft:grass_block", map[string]any{"minecraft:snowy_bit": false})
	case 4:
		return blockRID("minecraft:stone")
	default:
		return blockRID("minecraft:air")
	}
}

func bedrockRIDToLivingWorldBlockID(rid uint32) int32 {
	name, _, ok := dfchunk.RuntimeIDToState(rid)
	if !ok {
		return 0
	}
	switch name {
	case "minecraft:air":
		return 0
	case "minecraft:bedrock":
		return 1
	case "minecraft:dirt":
		return 2
	case "minecraft:grass_block":
		return 3
	case "minecraft:stone":
		return 4
	default:
		return 4
	}
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
	rid := livingWorldBlockIDToBedrockRID(ev.BlockID)
	s.forEachSession(func(bs *bedrockSession) {
		bs.write(&packet.UpdateBlock{
			Position:          protocol.BlockPos{int32(ev.X), int32(ev.Y), int32(ev.Z)},
			NewBlockRuntimeID: rid,
			Flags:             packet.BlockUpdateNetwork | packet.BlockUpdateNeighbours,
			Layer:             0,
		})
	})
}
