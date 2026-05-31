package server

import (
	"livingworld/internal/bedrock/inventory"
	"livingworld/internal/drops"
	"livingworld/internal/item"
	"livingworld/internal/player"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
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
		// Use velocity from Drop struct (vanilla physics)
		vel := mgl32.Vec3{
			float32(d.VX),
			float32(d.VY),
			float32(d.VZ),
		}
		s.forEachSession(func(bs *bedrockSession) {
			bs.write(&packet.AddItemActor{
				EntityUniqueID:  d.EntityID,
				EntityRuntimeID: uint64(d.EntityID),
				Item:            inst,
				Position:        mgl32.Vec3{float32(d.X), float32(d.Y), float32(d.Z)},
				Velocity:        vel,
			})
		})
	})

	store.OnDespawn(func(id int64) {
		s.forEachSession(func(bs *bedrockSession) {
			bs.write(&packet.RemoveActor{EntityUniqueID: id})
		})
	})
}

// registerPickupHandler registers a callback with the world manager to handle
// item pickups for Bedrock players (animation + inventory sync).
func (s *Server) registerPickupHandler() {
	s.wm.OnItemPickup(func(playerUUID [16]byte, dropEntityID int64, playerEntityID uint64) {
		// Fly the item to the collector on every Bedrock viewer, whatever edition
		// the collector is. TakeItemActor both animates and removes the item, so
		// the store no longer sends a RemoveActor on pickup (which cancelled it).
		s.forEachSession(func(bs *bedrockSession) {
			bs.write(&packet.TakeItemActor{
				ItemEntityRuntimeID:  uint64(dropEntityID),
				TakerEntityRuntimeID: playerEntityID,
			})
		})

		// Inventory sync only when the collector is a Bedrock player.
		uid, _ := uuid.FromBytes(playerUUID[:])
		pl := s.pm.GetPlayer(uid)
		if pl == nil || pl.Edition != player.EditionBedrock {
			return
		}
		if bs, ok := s.getSession(uid); ok {
			s.syncBedrockInventory(bs, pl)
		}
	})
}

// syncBedrockInventory sends the player's current inventory to their Bedrock client.
func (s *Server) syncBedrockInventory(bs *bedrockSession, pl *player.Player) {
	if pl.Inventory == nil {
		return
	}
	items := pl.Inventory.Items
	content := make([]protocol.ItemInstance, 36)
	for i := 0; i < 36 && i < len(items); i++ {
		if items[i].ID == 0 || items[i].Count == 0 {
			continue
		}
		// Convert Java item ID to item name, then to Bedrock runtime ID.
		it, ok := item.ByID(items[i].ID)
		if !ok {
			continue
		}
		rid, ok := inventory.RuntimeIDByName(it.Name)
		if !ok {
			continue
		}
		content[i] = protocol.ItemInstance{
			Stack: protocol.ItemStack{
				ItemType: protocol.ItemType{NetworkID: rid},
				Count:    uint16(items[i].Count),
			},
		}
	}
	bs.write(&packet.InventoryContent{
		WindowID: protocol.WindowIDInventory,
		Content:  content,
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
		vel := mgl32.Vec3{
			float32((d.EntityID%100 - 50)) * 0.002,
			0.2,
			float32((d.EntityID%73 - 36)) * 0.002,
		}
		bs.write(&packet.AddItemActor{
			EntityUniqueID:  d.EntityID,
			EntityRuntimeID: uint64(d.EntityID),
			Item: protocol.ItemInstance{Stack: protocol.ItemStack{
				ItemType: protocol.ItemType{NetworkID: rid},
				Count:    uint16(d.Count),
			}},
			Position: mgl32.Vec3{float32(d.X), float32(d.Y), float32(d.Z)},
			Velocity: vel,
		})
	}
}
