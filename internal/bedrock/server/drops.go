package server

import (
	"time"

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
	// Server-authoritative drop physics (StartDropPhysics) drives this: the Bedrock
	// client interpolates between absolute positions, but plain MoveActorAbsolute
	// carries no velocity so a falling item snaps each update and looks stuttery.
	// Pairing every position update with SetActorMotion gives the client the live
	// velocity vector to extrapolate from — items now arc, bounce-roll and settle
	// the way they do on Java. The OnGround flag also tells the client to stop
	// running its own gravity so resting items don't shiver against the floor.
	store.OnMove(func(d drops.Drop) {
		// Re-entrancy guard against pickup: store.TickPhysics snapshots each
		// moved drop into a local slice OUTSIDE the lock and fires OnMove
		// after releasing it. If the central pickup loop Claims the drop
		// between the snapshot and the OnMove callback (it iterates the
		// store in its own goroutine, so this is the common case for a
		// pickup that lands on the same 50 ms tick as a physics step), the
		// OnMove still fires with the pre-Claim position data and pushes
		// MoveActorAbsolute+SetActorAbsolute to the client — which then
		// briefly re-places the entity at the original position WHILE the
		// TakeItemActor magnet is playing, producing the "ghost stuck drop
		// + magnet" visual the user reported. A quick "is this drop still
		// alive in the store?" check before broadcasting kills the ghost:
		// either the drop is still settling (broadcast the update) or it
		// was just claimed by a pickup (skip, the RemoveActor/TakeItemActor
		// path owns the entity from here on).
		if _, ok := store.Get(d.EntityID); !ok {
			return
		}
		flags := byte(0)
		if d.OnGround {
			flags |= packet.MoveFlagOnGround
		}
		pos := mgl32.Vec3{float32(d.X), float32(d.Y), float32(d.Z)}
		vel := mgl32.Vec3{float32(d.VX), float32(d.VY), float32(d.VZ)}
		s.forEachSession(func(bs *bedrockSession) {
			bs.write(&packet.SetActorMotion{
				EntityRuntimeID: uint64(d.EntityID),
				Velocity:        vel,
			})
			bs.write(&packet.MoveActorAbsolute{
				EntityRuntimeID: uint64(d.EntityID),
				Flags:           flags,
				Position:        pos,
			})
		})
	})
}

// registerPickupHandler registers a callback with the world manager to handle
// item pickups for Bedrock players (animation + inventory sync).
func (s *Server) registerPickupHandler() {
	s.wm.OnItemPickup(func(playerUUID [16]byte, dropEntityID int64, playerEntityID uint64) {
		// TakeItemActor flies the dropped item to the collector — that IS the
		// vanilla pickup "magnet" animation on Bedrock. Sending RemoveActor in the
		// same write kills the entity before the client can play the tween, which
		// is why pickups used to just blink out. Send TakeItemActor right away so
		// the magnet plays, then queue a defensive RemoveActor ~10 ticks later so
		// any client that didn't despawn after the animation gets cleaned up.
		//
		// The TakerEntityRuntimeID is the runtime id of the player who is doing
		// the collecting. Foreign viewers (other Bedrock sessions) see the
		// collector as `playerEntityID` (the per-session runtime id that the
		// player-spawn code assigned them). The local collector's own client
		// however identifies itself as `bedrockLocalRuntime` (1), NOT
		// `playerEntityID` — addressing the wrong id silently no-ops the magnet
		// (the item sits still, then the delayed RemoveActor makes it blink out).
		// So for the collector's own session we use the local-runtime id; for
		// everyone else we use the per-session id.
		uid, _ := uuid.FromBytes(playerUUID[:])
		collectorSession, _ := s.getSession(uid)
		s.forEachSession(func(bs *bedrockSession) {
			taker := playerEntityID
			if bs == collectorSession {
				taker = bedrockLocalRuntime
			}
			bs.write(&packet.TakeItemActor{
				ItemEntityRuntimeID:  uint64(dropEntityID),
				TakerEntityRuntimeID: taker,
			})
		})
		go func() {
			time.Sleep(500 * time.Millisecond)
			s.forEachSession(func(bs *bedrockSession) {
				bs.write(&packet.RemoveActor{EntityUniqueID: dropEntityID})
			})
		}()

		// Inventory sync only when the collector is a Bedrock player.
		pl := s.pm.GetPlayer(uid)
		if pl == nil || pl.Edition != player.EditionBedrock {
			return
		}
		if collectorSession != nil {
			s.syncBedrockInventory(collectorSession, pl)
		}
	})
}

// handleBedrockPlayerDrop spawns an item entity on the ground for the Q / Ctrl+Q
// hotbar drop action and decrements the held slot authoritatively. The Bedrock
// client doesn't always tell us "drop all" vs "drop one" in the transaction
// itself — it just sends the slot, the old stack count, and the new stack count,
// and the difference is the dropped count. So this helper is the single point
// that turns the wire data into a thrown item entity.
func (s *Server) handleBedrockPlayerDrop(bs *bedrockSession, hotbarSlot uint32, count int) {
	if count <= 0 {
		return
	}
	pl := s.pm.GetPlayer(bs.id)
	if pl == nil || pl.Inventory == nil {
		return
	}
	slot := int(hotbarSlot)
	if slot < 0 || slot >= len(pl.Inventory.Items) {
		return
	}
	held := pl.Inventory.Items[slot]
	if held.ID == 0 || held.Count == 0 {
		return
	}
	if int(count) > int(held.Count) {
		count = int(held.Count)
	}
	// Resolve canonical item name for the drop store. Unknown runtime ids
	// (modded / creative-only) silently no-op; vanilla would just bounce the
	// item back into the slot too.
	it, ok := item.ByID(held.ID)
	if !ok {
		return
	}
	pl.Inventory.Items[slot].Count -= int8(count)
	if pl.Inventory.Items[slot].Count <= 0 {
		pl.Inventory.Items[slot] = player.ItemStack{}
	}
	s.wm.PlayerDropItem(it.Name, count, pl.Position.X+0.5, pl.Position.Y+0.25, pl.Position.Z+0.5, float64(pl.Rotation.Yaw))
	s.syncBedrockInventory(bs, pl)
	s.pm.PublishEquipmentChange(bs.id)
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
		// Use the drop's real current velocity (matching the OnSpawn handler) so a
		// freshly-joined viewer sees the item with its true motion, not a fabricated
		// one.
		vel := mgl32.Vec3{float32(d.VX), float32(d.VY), float32(d.VZ)}
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
