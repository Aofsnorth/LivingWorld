package server

import (
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// LivingWorld canonical block IDs ARE Java global state IDs, so translation in
// both directions is the identity function. These wrappers are kept for call-site
// clarity and so the mapping has an obvious single point of change.
func livingWorldBlockIDToJavaStateID(id int32) int32 { return id }

func javaStateIDToLivingWorldBlockID(stateID int32) int32 { return stateID }

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
