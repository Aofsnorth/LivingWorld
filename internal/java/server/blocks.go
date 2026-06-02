package server

import (
	"livingworld/internal/item"
	"livingworld/internal/world"
	"livingworld/plugin"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

var airStateID = block.ToStateID[block.Air{}]

func (s *PlayerSession) HandlePlayerAction(p pk.Packet) {
	var status pk.VarInt
	var pos pk.Position
	var face pk.Byte
	var sequence pk.VarInt
	if err := p.Scan(&status, &pos, &face, &sequence); err != nil {
		return
	}

	// Track crack state and broadcast the action's effect to the OTHER edition via
	// the world effect bus (the Java breaker already predicts its own crack/break;
	// the bus only feeds Bedrock viewers — the subscriber skips Java-source events).
	switch status {
	case 0: // start digging
		hadPrev, prevX, prevY, prevZ := s.Bridge.wm.CrackManager().StartBreaking(s.UUID(), pos.X, pos.Y, pos.Z)
		if hadPrev {
			// Switched to a new block: clear the crack overlay on the old one.
			s.Bridge.wm.PublishCrack(world.BlockUpdateSourceJava, s.UUID(), prevX, prevY, prevZ, -1)
		}
		s.Bridge.wm.PublishCrack(world.BlockUpdateSourceJava, s.UUID(), pos.X, pos.Y, pos.Z, 0)
	case 1: // cancel digging
		s.Bridge.wm.CrackManager().StopBreaking(s.UUID())
		s.Bridge.wm.PublishCrack(world.BlockUpdateSourceJava, s.UUID(), pos.X, pos.Y, pos.Z, -1)
	case 2: // finish digging
		s.Bridge.wm.CrackManager().StopBreaking(s.UUID())
		current := s.Bridge.wm.GetDefaultWorld().GetBlock(pos.X, pos.Y, pos.Z)
		ev := &plugin.BlockBreakEvent{
			BaseEvent:  plugin.BaseEvent{Type_: plugin.EventBlockBreak},
			PlayerName: s.Username(),
			X:          pos.X, Y: pos.Y, Z: pos.Z,
			BlockID: current.ID(),
		}
		if plugin.Manager().EmitCancellable(ev) {
			// A plugin vetoed the break: re-affirm the block to the client so its
			// optimistic removal is rolled back.
			_ = s.SendPacket(pk.Marshal(packetid.ClientboundGameBlockUpdate, pos, pk.VarInt(current.ID())))
			return
		}
		// Clear any crack overlay this break opened on the other edition (a
		// block-update-to-air does NOT clear the Java BlockDestruction overlay), then
		// play the break particles + sound on Bedrock viewers.
		s.Bridge.wm.PublishCrack(world.BlockUpdateSourceJava, s.UUID(), pos.X, pos.Y, pos.Z, -1)
		s.Bridge.wm.PublishBlockDestroy(world.BlockUpdateSourceJava, s.UUID(), pos.X, pos.Y, pos.Z, current.ID())
		// Roll vanilla loot for the broken block and spawn item entities BEFORE the
		// block becomes air (the loot lookup needs the block's id).
		s.Bridge.wm.DropBlockLoot(current.ID(), pos.X, pos.Y, pos.Z)
		s.Bridge.wm.SetBlockAndPublish(world.BlockUpdateSourceJava, pos.X, pos.Y, pos.Z, world.BlockAir{})
	}
}

func (s *PlayerSession) HandleUseItemOn(p pk.Packet) {
	var hand pk.VarInt
	var pos pk.Position
	var face pk.VarInt
	var cursorX, cursorY, cursorZ pk.Float
	var insideBlock, worldBorderHit pk.Boolean
	var sequence pk.VarInt
	if err := p.Scan(&hand, &pos, &face, &cursorX, &cursorY, &cursorZ, &insideBlock, &worldBorderHit, &sequence); err != nil {
		return
	}

	stateID := s.getBlockStateForPlacement()
	if stateID == 0 {
		// No held block (hand empty, or held item is not placeable). Re-sync the
		// clicked and target blocks to roll back the client's place prediction,
		// otherwise the held stack gets visually decremented with nothing to show
		// for it.
		s.rollbackClientPlacePrediction(pos, int(face))
		return
	}

	// Compute the target position (the cell the new block would occupy).
	x, y, z := pos.X, pos.Y, pos.Z
	switch face {
	case 0:
		y--
	case 1:
		y++
	case 2:
		z--
	case 3:
		z++
	case 4:
		x--
	case 5:
		x++
	}

	// Vanilla placement rules (1.21.1+ UseItemOn, see AbstractContainerMenu /
	// PlayerGameMode.useItemOn):
	//
	//  1. If the *clicked* block is REPLACEABLE (air, tall grass, water, etc.)
	//     we place at the clicked position, not the offset.
	//  2. Otherwise, the target must be AIR (no overwrite) — overwriting an
	//     existing block is not a valid vanilla action.
	//  3. If the player is placing on TOP of a solid block (face=1, +Y) and is
	//     NOT sneaking, the action is treated as USE-on-block (open chest,
	//     etc.) not PLACE. The client predicts PLACE anyway, so the server
	//     must reject + re-affirm the clicked block to undo the prediction.
	clicked := s.Bridge.wm.GetDefaultWorld().GetBlock(pos.X, pos.Y, pos.Z)
	clickedID := clicked.ID()
	if isReplaceableBlock(clickedID) {
		x, y, z = pos.X, pos.Y, pos.Z
	}
	pl := s.Bridge.pm.GetPlayer(s.UUID())
	if pl == nil {
		return
	}
	// Crouch gate: placing on top of a non-replaceable block without sneaking
	// is USE, not PLACE.
	if face == 1 && !isReplaceableBlock(clickedID) && !pl.Sneaking {
		s.rollbackClientPlacePrediction(pos, int(face))
		return
	}
	// Overwrite gate: if the final target isn't air, refuse.
	targetID := s.Bridge.wm.GetDefaultWorld().GetBlock(x, y, z).ID()
	if targetID != world.AirID {
		s.rollbackClientPlacePrediction(pos, int(face))
		return
	}

	s.Bridge.wm.SetBlockAndPublish(world.BlockUpdateSourceJava, x, y, z, world.BlockByID(int32(stateID)))

	// Decrement held item count (survival item consumption). This only runs
	// for a fully-valid placement so the client's predicted decrement always
	// matches the server's authoritative state.
	if pl.Inventory != nil && s.SelectedSlot >= 0 && s.SelectedSlot < 9 && int(s.SelectedSlot) < len(pl.Inventory.Items) {
		if pl.Inventory.Items[s.SelectedSlot].Count > 0 {
			pl.Inventory.Items[s.SelectedSlot].Count--
			s.syncInventory()
			// The held stack shrank (or emptied): re-render the hand for others.
			s.Bridge.pm.PublishEquipmentChange(s.UUID())
		}
	}
}

// rollbackClientPlacePrediction re-affirms the clicked and target blocks so
// the client's optimistic place is undone when the server refuses the
// action (no held block, crouch gate, target not air, etc.).
func (s *PlayerSession) rollbackClientPlacePrediction(pos pk.Position, face int) {
	wm := s.Bridge.wm.GetDefaultWorld()
	currentID := wm.GetBlock(pos.X, pos.Y, pos.Z).ID()
	_ = s.SendPacket(pk.Marshal(packetid.ClientboundGameBlockUpdate, pos, pk.VarInt(livingWorldBlockIDToJavaStateID(currentID))))
	x, y, z := pos.X, pos.Y, pos.Z
	switch face {
	case 0:
		y--
	case 1:
		y++
	case 2:
		z--
	case 3:
		z++
	case 4:
		x--
	case 5:
		x++
	}
	targetID := wm.GetBlock(x, y, z).ID()
	_ = s.SendPacket(pk.Marshal(packetid.ClientboundGameBlockUpdate, pk.Position{X: x, Y: y, Z: z}, pk.VarInt(livingWorldBlockIDToJavaStateID(targetID))))
}

// isReplaceableBlock reports whether the block id can be replaced by a new
// block placement (i.e. it's air, a plant, a fluid, etc.). The 26.1 client
// predicts a PLACE on top of these, so the server should follow through.
//
// We treat the common cases the world can actually produce; the worldgen
// pipeline (superflat, terrain) only ever writes air, stone, dirt, grass,
// and ore, so the only replaceable block we see in practice is air. The
// list is left open so other modules (plants, fluids in Phase 4d) can extend
// it without changing the call site.
func isReplaceableBlock(stateID int32) bool {
	return stateID == world.AirID
}

func (s *PlayerSession) getBlockStateForPlacement() block.StateID {
	// Resolve held item dari inventory player
	pl := s.Bridge.pm.GetPlayer(s.UUID())
	if pl == nil || pl.Inventory == nil {
		return 0
	}

	// SelectedSlot adalah hotbar index (0-8)
	if s.SelectedSlot < 0 || s.SelectedSlot >= 9 {
		return 0
	}

	// Inventory.Items[0-8] adalah hotbar
	if int(s.SelectedSlot) >= len(pl.Inventory.Items) {
		return 0
	}

	heldItem := pl.Inventory.Items[s.SelectedSlot]
	if heldItem.ID == 0 || heldItem.Count == 0 {
		return 0
	}

	// Resolve item ID → name → block state ID
	it, ok := item.ByID(heldItem.ID)
	if !ok {
		return 0
	}

	stateID, placeable := item.BlockStateID(it.Name)
	if !placeable {
		return 0
	}

	return block.StateID(stateID)
}

func (s *PlayerSession) HandleSwing(p pk.Packet) {
	var hand pk.VarInt
	if err := p.Scan(&hand); err != nil {
		return
	}
	s.Bridge.sessions.BroadcastExcept(s.UUID(), pk.Marshal(
		packetid.ClientboundGameAnimate,
		pk.VarInt(s.EntityID()), pk.UnsignedByte(0),
	))
	// Publish to shared player manager so Bedrock viewers also see the swing.
	if s.Bridge != nil && s.Bridge.pm != nil {
		s.Bridge.pm.PublishSwing(s.UUID())
	}
}
