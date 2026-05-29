package server

import (
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

func livingWorldBlockIDToJavaStateID(id int32) int32 {
	switch id {
	case 0:
		return int32(block.ToStateID[block.Air{}])
	case 1:
		return int32(block.ToStateID[block.Bedrock{}])
	case 2:
		return int32(block.ToStateID[block.Dirt{}])
	case 3:
		return int32(block.ToStateID[block.GrassBlock{}])
	case 4:
		return int32(block.ToStateID[block.Stone{}])
	default:
		return int32(block.ToStateID[block.Air{}])
	}
}

func javaStateIDToLivingWorldBlockID(stateID int32) int32 {
	switch block.StateID(stateID) {
	case block.ToStateID[block.Air{}]:
		return 0
	case block.ToStateID[block.Bedrock{}]:
		return 1
	case block.ToStateID[block.Dirt{}]:
		return 2
	case block.ToStateID[block.GrassBlock{}]:
		return 3
	case block.ToStateID[block.Stone{}]:
		return 4
	default:
		return 4
	}
}

func (j *javaBridge) startBlockEventLoop() {
	ch := j.wm.SubscribeBlockUpdates("java-bridge", 256)
	go func() {
		for ev := range ch {
			j.broadcastBlockUpdate(ev)
		}
	}()
}

func (j *javaBridge) broadcastBlockUpdate(ev world.BlockUpdateEvent) {
	stateID := livingWorldBlockIDToJavaStateID(ev.BlockID)
	j.sessions.Broadcast(pk.Marshal(
		packetid.ClientboundGameBlockUpdate,
		pk.Position{X: ev.X, Y: ev.Y, Z: ev.Z}, pk.VarInt(stateID),
	))
}
