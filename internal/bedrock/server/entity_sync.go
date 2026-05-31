package server

import (
	"livingworld/internal/bedrock/inventory"
	"livingworld/internal/item"
	"livingworld/internal/player"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// startPlayerEventLoop bridges shared player.Manager events into Bedrock
// viewers. Bedrock->Bedrock is handled here too; clients only ignore their own
// UUID, not the whole Bedrock edition.
func (s *Server) startPlayerEventLoop() {
	if s.playerEvents != nil {
		return
	}
	s.playerEvents = s.pm.Subscribe("bedrock-server", 256)
	go func() {
		for ev := range s.playerEvents {
			s.forEachSession(func(viewer *bedrockSession) {
				if ev.Player.UUID == viewer.id {
					return
				}
				switch ev.Type {
				case player.EventJoin:
					s.spawnPlayerFor(viewer, ev.Player)
				case player.EventMove:
					s.movePlayerFor(viewer, ev.Player, ev.Teleport)
				case player.EventLeave:
					s.removePlayerFor(viewer, ev.Player)
				case player.EventSwing:
					s.swingPlayerFor(viewer, ev.Player)
				case player.EventSneak:
					s.updateSneakFor(viewer, ev.Player)
				case player.EventEquipment:
					s.updateEquipmentFor(viewer, ev.Player)
				case player.EventHurt:
					s.hurtPlayerFor(viewer, ev.Player)
				}
			})
		}
	}()
}

func (s *Server) spawnExistingForeignPlayers(viewer *bedrockSession) {
	for _, p := range s.pm.GetAllPlayers() {
		if p.UUID == viewer.id {
			continue
		}
		s.spawnPlayerFor(viewer, p.Snapshot())
	}
}

func (s *Server) spawnPlayerFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	if p.Edition == player.EditionBedrock {
		target, ok := s.getSession(p.UUID)
		if !ok {
			return
		}
		viewer.spawnBedrockPlayer(target, p)
	} else {
		viewer.spawnJavaPlayer(p)
	}
	s.updateEquipmentFor(viewer, p) // render their held item right away
}

func (s *Server) movePlayerFor(viewer *bedrockSession, p player.PlayerSnapshot, teleport bool) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	// MoveActorAbsolute reliably updates a remote entity's rendered body AND head
	// rotation (Rotation packs {pitch, yaw, headYaw}); this is what dragonfly and
	// Geyser use for non-self entities. MovePlayer/MoveModeTeleport only corrects
	// position and left the Java player facing the wrong way on Bedrock.
	flags := byte(0)
	if teleport {
		flags |= packet.MoveFlagTeleport
	}
	if p.OnGround {
		flags |= packet.MoveFlagOnGround
	}
	viewer.write(&packet.MoveActorAbsolute{
		EntityRuntimeID: p.EntityRuntimeID,
		Position:        bedrockPosForSnapshot(p),
		Rotation:        mgl32.Vec3{p.Rotation.Pitch, p.Rotation.Yaw, p.Rotation.Yaw},
		Flags:           flags,
	})
}

func (s *Server) updateSneakFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	viewer.write(&packet.SetActorData{
		EntityRuntimeID: p.EntityRuntimeID,
		EntityMetadata:  bedrockMetadata(p.Username, p.Sneaking),
	})
}

func (s *Server) removePlayerFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	viewer.write(&packet.PlayerList{ActionType: packet.PlayerListActionRemove, Entries: []protocol.PlayerListEntry{{UUID: p.UUID}}})
	viewer.write(&packet.RemoveActor{EntityUniqueID: int64(p.EntityRuntimeID)})
}

// hurtPlayerFor plays the red hurt flash on another player's avatar for a viewer.
func (s *Server) hurtPlayerFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	viewer.write(&packet.ActorEvent{EntityRuntimeID: p.EntityRuntimeID, EventType: packet.ActorEventHurt})
}

func (s *Server) swingPlayerFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	viewer.write(&packet.Animate{
		ActionType:      packet.AnimateActionSwingArm,
		EntityRuntimeID: p.EntityRuntimeID,
	})
}

func (s *Server) updateEquipmentFor(viewer *bedrockSession, p player.PlayerSnapshot) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	// Get held item from player inventory
	pl := s.pm.GetPlayer(p.UUID)
	if pl == nil || pl.Inventory == nil {
		return
	}
	heldItem := pl.Inventory.GetHeldItem()
	if heldItem == nil || heldItem.ID == 0 {
		// Empty hand
		viewer.write(&packet.MobEquipment{
			EntityRuntimeID: p.EntityRuntimeID,
			NewItem:         protocol.ItemInstance{},
			InventorySlot:   byte(pl.HeldItemSlot),
			HotBarSlot:      byte(pl.HeldItemSlot),
		})
		return
	}

	// Resolve item ID to Bedrock runtime ID
	it, ok := item.ByID(heldItem.ID)
	if !ok {
		return
	}
	rid, ok := inventory.RuntimeIDByName(it.Name)
	if !ok {
		return
	}

	viewer.write(&packet.MobEquipment{
		EntityRuntimeID: p.EntityRuntimeID,
		NewItem: protocol.ItemInstance{
			Stack: protocol.ItemStack{
				ItemType: protocol.ItemType{NetworkID: rid},
				Count:    uint16(heldItem.Count),
			},
		},
		InventorySlot: byte(pl.HeldItemSlot),
		HotBarSlot:    byte(pl.HeldItemSlot),
	})
}
