package server

import (
	"livingworld/internal/bedrock/inventory"
	"livingworld/internal/drops"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// startDropLoop renders the shared drop store to Bedrock clients. Item spawns
// become AddItemActor, despawns become RemoveActor. Pickup itself (proximity +
// inventory) is driven centrally by the Java bridge's pickup loop over the same
// store, so it covers Bedrock players too; here we only mirror spawn/despawn and
// play the Bedrock pickup animation when a drop is taken near a Bedrock player.
func (s *Server) startDropLoop() {
	store := s.wm.Drops()

	store.OnSpawn(func(d drops.Drop) {
		rid, ok := inventory.RuntimeIDByName(d.Item)
		if !ok {
			return // unknown item: do NOT spawn — a bad item id crashes nearby clients
		}
		inst := protocol.ItemInstance{
			StackNetworkID: 0,
			Stack: protocol.ItemStack{
				ItemType: protocol.ItemType{NetworkID: rid},
				Count:    uint16(d.Count),
			},
		}
		s.forEachSession(func(bs *bedrockSession) {
			bs.write(&packet.AddItemActor{
				EntityUniqueID:  d.EntityID,
				EntityRuntimeID: uint64(d.EntityID),
				Item:            inst,
				Position:        mgl32.Vec3{float32(d.X), float32(d.Y), float32(d.Z)},
			})
		})
	})

	store.OnDespawn(func(id int64) {
		s.forEachSession(func(bs *bedrockSession) {
			bs.write(&packet.RemoveActor{EntityUniqueID: id})
		})
	})
}

// spawnExistingDropsFor sends every active drop to a freshly-joined Bedrock
// viewer so items already on the ground are visible.
func (s *Server) spawnExistingDropsFor(bs *bedrockSession) {
	for _, d := range s.wm.Drops().All() {
		rid, ok := inventory.RuntimeIDByName(d.Item)
		if !ok {
			continue
		}
		bs.write(&packet.AddItemActor{
			EntityUniqueID:  d.EntityID,
			EntityRuntimeID: uint64(d.EntityID),
			Item: protocol.ItemInstance{Stack: protocol.ItemStack{
				ItemType: protocol.ItemType{NetworkID: rid},
				Count:    uint16(d.Count),
			}},
			Position: mgl32.Vec3{float32(d.X), float32(d.Y), float32(d.Z)},
		})
	}
}
