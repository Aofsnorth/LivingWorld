package server

import (
	"livingworld/internal/item"
	"livingworld/internal/player"
	"livingworld/internal/world"
	"livingworld/plugin"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

var airStateID = block.ToStateID[block.Air{}]

// ackBlockChange confirms a client's predicted block change so the client lifts
// its local prediction. Without this every break/place compounds prediction
// drift (ghost blocks, re-broken blocks turning invisible, items appearing to
// duplicate). Always called via defer right after parsing the sequence so any
// reject/early-return path still acks.
func (s *PlayerSession) ackBlockChange(sequence pk.VarInt) {
	_ = s.SendPacket(pk.Marshal(packetid.ClientboundGameBlockChangedAck, sequence))
}

func (s *PlayerSession) HandlePlayerAction(p pk.Packet) {
	var status pk.VarInt
	var pos pk.Position
	var face pk.Byte
	var sequence pk.VarInt
	if err := p.Scan(&status, &pos, &face, &sequence); err != nil {
		return
	}
	// 1.19+ / 26.3 block-action prediction: the client predicts the result locally for
	// block actions (status 0..3 in 26.3) and stamps them with a sequence id.
	// In Minecraft 26.3 (protocol 777), ServerboundPlayerActionPacket$Action ordinals are:
	//   0 = START_DESTROY_BLOCK
	//   1 = CHANGE_DESTROY_DIRECTION (inserted at index 1 in 26.3!)
	//   2 = ABORT_DESTROY_BLOCK
	//   3 = STOP_DESTROY_BLOCK
	//   4 = DROP_ALL_ITEMS (Ctrl+Q)
	//   5 = DROP_ITEM (Q)
	//   6 = RELEASE_USE_ITEM
	//   7 = SWAP_ITEM_WITH_OFFHAND (F)
	//   8 = STAB
	if status >= 0 && status <= 3 {
		defer s.ackBlockChange(sequence)
	}

	// Track crack state and broadcast the action's effect to the OTHER edition via
	// the world effect bus (the Java breaker already predicts its own crack/break;
	// the bus only feeds Bedrock viewers — the subscriber skips Java-source events).
	switch status {
	case 0: // START_DESTROY_BLOCK (start digging)
		s.broadcastSwing(false)
		current := s.Bridge.wm.GetDefaultWorld().GetBlock(pos.X, pos.Y, pos.Z)
		hardness := world.Hardness(current.ID())
		// In Minecraft Java Edition, blocks with hardness == 0 (short_grass,
		// tall_grass, ferns, flowers, torches, saplings, etc.) and Creative
		// mode breaks happen immediately inside startDestroyBlock on the
		// client, which ONLY sends status=0 (START_DESTROY_BLOCK) and never
		// sends status=3 (STOP_DESTROY_BLOCK).
		if s.GameMode() == 1 || (current.ID() != 0 && hardness == 0) {
			s.Bridge.wm.CrackManager().StopBreaking(s.UUID())
			s.finishBlockBreak(pos, current)
			return
		}
		// Capture the per-block break duration into CrackState so the
		// Bedrock effect subscriber and any future per-stage broadcaster
		// (notably for cross-edition observers) see the right crack rate
		// for stone vs grass vs wood. Vanilla hand-mining time (hardness *
		// 1.5) is the tool-agnostic default; tool-aware timing lives in
		// the BlockBreakEvent plugins.
		breakSecs := hardness * 1.5
		if breakSecs < 0 {
			breakSecs = 0
		}
		hadPrev, prevX, prevY, prevZ := s.Bridge.wm.CrackManager().StartBreaking(s.UUID(), pos.X, pos.Y, pos.Z, breakSecs)
		if hadPrev {
			// Switched to a new block: clear the crack overlay on the old one.
			s.Bridge.wm.PublishCrack(world.BlockUpdateSourceJava, s.UUID(), prevX, prevY, prevZ, -1)
		}
		s.Bridge.wm.PublishCrack(world.BlockUpdateSourceJava, s.UUID(), pos.X, pos.Y, pos.Z, 0)
	case 1: // CHANGE_DESTROY_DIRECTION (keep digging current block; face changed)
	case 2: // ABORT_DESTROY_BLOCK (cancel digging)
		s.Bridge.wm.CrackManager().StopBreaking(s.UUID())
		s.Bridge.wm.PublishCrack(world.BlockUpdateSourceJava, s.UUID(), pos.X, pos.Y, pos.Z, -1)
	case 3: // STOP_DESTROY_BLOCK (finish digging)
		s.broadcastSwing(false)
		s.Bridge.wm.CrackManager().StopBreaking(s.UUID())
		current := s.Bridge.wm.GetDefaultWorld().GetBlock(pos.X, pos.Y, pos.Z)
		s.finishBlockBreak(pos, current)
	case 4, 5: // DROP_ALL_ITEMS (4, Ctrl+Q) / DROP_ITEM (5, Q)
		s.handlePlayerDropItem(status == 4)
	case 7: // SWAP_ITEM_WITH_OFFHAND (F)
		if pl := s.Bridge.pm.GetPlayer(s.UUID()); pl != nil && pl.Inventory != nil {
			slot := int(s.SelectedSlot)
			if slot >= 0 && slot < 9 && slot < len(pl.Inventory.Items) && 40 < len(pl.Inventory.Items) {
				mainItem := pl.Inventory.Items[slot]
				offItem := pl.Inventory.Items[40]
				pl.Inventory.SetItem(slot, offItem)
				pl.Inventory.SetItem(40, mainItem)
				s.syncInventory()
				s.Bridge.pm.PublishEquipmentChange(s.UUID())
			}
		}
	case 8: // STAB
		s.broadcastSwing(false)
	}
}

// finishBlockBreak validates, emits BlockBreakEvent, drops loot, and replaces the
// broken block (plus the paired half of any double-tall plant) with air.
func (s *PlayerSession) finishBlockBreak(pos pk.Position, current world.Block) {
	if current.ID() == 0 {
		return
	}
	// Refuse to break unbreakable blocks (hardness < 0) in any gamemode so a client
	// cannot punch through bedrock.
	if world.Hardness(current.ID()) < 0 {
		_ = s.SendPacket(pk.Marshal(packetid.ClientboundGameBlockUpdate, pos, pk.VarInt(current.ID())))
		return
	}
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
	if s.GameMode() != 1 {
		toolName := ""
		if pl := s.Bridge.pm.GetPlayer(s.UUID()); pl != nil && pl.Inventory != nil {
			slot := int(s.SelectedSlot)
			if slot >= 0 && slot < 9 && slot < len(pl.Inventory.Items) {
				held := pl.Inventory.Items[slot]
				if held.ID != 0 && held.Count > 0 {
					if it, ok := item.ByID(held.ID); ok {
						toolName = it.Name
					}
				}
			}
		}
		s.Bridge.wm.DropBlockLootWithTool(current.ID(), toolName, pos.X, pos.Y, pos.Z)
	}
	s.Bridge.wm.SetBlockAndPublish(world.BlockUpdateSourceJava, pos.X, pos.Y, pos.Z, world.BlockAir{})
	s.Bridge.wm.OnBlockChanged(world.BlockUpdateSourceJava, pos.X, pos.Y, pos.Z)

	name := world.StateName(current.ID())
	// Play the block break sound directly to the breaking Java client so instant-break
	// blocks (flowers, tall grass, torches, creative breaks) always have audio feedback.
	_ = s.SendPacket(buildBlockSoundPacket(
		breakSoundForBlockName(name),
		float64(pos.X)+0.5, float64(pos.Y)+0.5, float64(pos.Z)+0.5,
		1.0, 0.8,
	))

	// If this was a 2-block-tall plant (tall_grass, large_fern, etc.), also clear
	// the paired upper or lower half so it doesn't leave a floating half-plant.
	if isDoublePlantName(name) {
		dw := s.Bridge.wm.GetDefaultWorld()
		for _, dy := range [2]int{1, -1} {
			otherY := pos.Y + dy
			other := dw.GetBlock(pos.X, otherY, pos.Z)
			if other.ID() != 0 && world.StateName(other.ID()) == name {
				s.Bridge.wm.PublishBlockDestroy(world.BlockUpdateSourceJava, s.UUID(), pos.X, otherY, pos.Z, other.ID())
				_ = s.SendPacket(pk.Marshal(
					packetid.ClientboundGameLevelEvent,
					pk.Int(2001),
					pk.Position{X: pos.X, Y: otherY, Z: pos.Z},
					pk.Int(livingWorldBlockIDToJavaStateID(other.ID())),
					pk.Boolean(false),
				))
				s.Bridge.wm.SetBlockAndPublish(world.BlockUpdateSourceJava, pos.X, otherY, pos.Z, world.BlockAir{})
				s.Bridge.wm.OnBlockChanged(world.BlockUpdateSourceJava, pos.X, otherY, pos.Z)
			}
		}
	}
}

func isDoublePlantName(name string) bool {
	switch name {
	case "minecraft:tall_grass", "minecraft:large_fern",
		"minecraft:sunflower", "minecraft:lilac",
		"minecraft:rose_bush", "minecraft:peony",
		"minecraft:tall_seagrass", "minecraft:pitcher_plant":
		return true
	default:
		return false
	}
}

// handlePlayerDropItem implements the vanilla Q (drop whole stack) and Ctrl+Q
// (drop single) hotbar actions. Spawns an item entity via the shared drop
// store, decrements the held slot, and resyncs the Java inventory so the held
// stack visibly shrinks.
func (s *PlayerSession) handlePlayerDropItem(dropAll bool) {
	pl := s.Bridge.pm.GetPlayer(s.UUID())
	if pl == nil || pl.Inventory == nil {
		return
	}
	slot := int(s.SelectedSlot)
	if slot < 0 || slot >= 9 || slot >= len(pl.Inventory.Items) {
		return
	}
	held := pl.Inventory.Items[slot]
	if held.ID == 0 || held.Count == 0 {
		return
	}
	dropCount := int(held.Count)
	if !dropAll && dropCount > 0 {
		dropCount = 1
	}
	// Resolve the canonical item name for the drop store. Unregistered ids
	// (creative-only / modded) silently do nothing — matches vanilla behaviour
	// where unknown items can't be dropped onto the world either.
	it, ok := item.ByID(held.ID)
	if !ok {
		return
	}
	// Throw the item out of the player. The Minecraft player eye height is
	// ~1.62, but a 0.25 offset above the feet is enough to clear their hitbox
	// and look natural; physics will sort out the rest.
	pl.Inventory.Items[slot].Count -= int8(dropCount)
	if pl.Inventory.Items[slot].Count <= 0 {
		pl.Inventory.Items[slot] = player.ItemStack{}
	}
	s.Bridge.wm.PlayerDropItem(it.Name, dropCount, pl.Position.X, pl.Position.Y+1.3, pl.Position.Z, float64(pl.Rotation.Pitch), float64(pl.Rotation.Yaw))
	s.broadcastSwing(true)
	s.syncInventory()
	s.Bridge.pm.PublishEquipmentChange(s.UUID())
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
	// See HandlePlayerAction: every block action carries a prediction sequence
	// that MUST be acked. Rejected placements (no item, occupied target) ack
	// the same way so the client can roll back its prediction cleanly.
	defer s.ackBlockChange(sequence)

	pl := s.Bridge.pm.GetPlayer(s.UUID())
	if pl != nil && !pl.Sneaking {
		clickedBlk := s.Bridge.wm.GetDefaultWorld().GetBlock(pos.X, pos.Y, pos.Z)
		if s.openBlockContainer(world.StateName(clickedBlk.ID()), pos.X, pos.Y, pos.Z) {
			s.broadcastSwing(false)
			return
		}
		if s.Bridge.wm.InteractBlock(world.BlockUpdateSourceJava, pos.X, pos.Y, pos.Z) {
			s.broadcastSwing(false)
			return
		}
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
	//
	// The crouch gate that vanilla applies on top of a non-replaceable block
	// (snipe: top + not sneaking ⇒ USE, not PLACE) is intentionally NOT
	// enforced here. LivingWorld has no block-USE semantics implemented yet, so
	// the gate would just stop players from placing dirt/grass on the ground
	// surface — a frustrating dead end that the Bedrock client doesn't even
	// hint at. The future interaction service (Phase 4d) can add per-block
	// USE semantics without touching this call site, mirroring the Bedrock
	// handler.
	clicked := s.Bridge.wm.GetDefaultWorld().GetBlock(pos.X, pos.Y, pos.Z)
	clickedID := clicked.ID()
	placedName := world.StateName(int32(stateID))
	clickedName := world.StateName(clickedID)

	// Support vanilla cluster-incrementing blocks (wildflowers, pink_petals, leaf_litter):
	// right-clicking an existing cluster with the same item increments flower_amount/segment_amount up to 4.
	if clickedName == placedName && (placedName == "minecraft:wildflowers" || placedName == "minecraft:pink_petals" || placedName == "minecraft:leaf_litter") {
		propKey := "flower_amount"
		if placedName == "minecraft:leaf_litter" {
			propKey = "segment_amount"
		}
		props := world.StateProperties(clickedID)
		if props != nil {
			nextVal := ""
			switch props[propKey] {
			case "1":
				nextVal = "2"
			case "2":
				nextVal = "3"
			case "3":
				nextVal = "4"
			default:
				s.rollbackClientPlacePrediction(pos, int(face))
				return
			}
			props[propKey] = nextVal
			if nextID := world.StateIDWithProperties(placedName, props); nextID != 0 {
				x, y, z = pos.X, pos.Y, pos.Z
				stateID = block.StateID(nextID)
			}
		}
	} else if isReplaceableBlock(clickedID) {
		x, y, z = pos.X, pos.Y, pos.Z
	}
	if pl == nil {
		return
	}
	// Overwrite gate: if the final target isn't replaceable, refuse.
	targetID := s.Bridge.wm.GetDefaultWorld().GetBlock(x, y, z).ID()
	targetName := world.StateName(targetID)
	if targetName == placedName && (placedName == "minecraft:wildflowers" || placedName == "minecraft:pink_petals" || placedName == "minecraft:leaf_litter") && (x != pos.X || y != pos.Y || z != pos.Z) {
		propKey := "flower_amount"
		if placedName == "minecraft:leaf_litter" {
			propKey = "segment_amount"
		}
		props := world.StateProperties(targetID)
		if props != nil {
			nextVal := ""
			switch props[propKey] {
			case "1":
				nextVal = "2"
			case "2":
				nextVal = "3"
			case "3":
				nextVal = "4"
			default:
				s.rollbackClientPlacePrediction(pos, int(face))
				return
			}
			props[propKey] = nextVal
			if nextID := world.StateIDWithProperties(placedName, props); nextID != 0 {
				stateID = block.StateID(nextID)
			}
		}
	} else if !isReplaceableBlock(targetID) {
		s.rollbackClientPlacePrediction(pos, int(face))
		return
	}

	s.Bridge.wm.SetBlockAndPublish(world.BlockUpdateSourceJava, x, y, z, world.BlockByID(int32(stateID)))
	s.Bridge.wm.OnBlockChanged(world.BlockUpdateSourceJava, x, y, z)
	s.broadcastSwing(false)
	if s.Bridge != nil && s.Bridge.sessions != nil {
		s.Bridge.sessions.Broadcast(buildBlockSoundPacket(
			placeSoundForBlockName(placedName),
			float64(x)+0.5, float64(y)+0.5, float64(z)+0.5,
			1.0, 0.8,
		))
	}

	// Decrement held item count (survival item consumption). This only runs
	// for a fully-valid placement so the client's predicted decrement always
	// matches the server's authoritative state.
	if pl.Inventory != nil && s.SelectedSlot >= 0 && s.SelectedSlot < 9 && int(s.SelectedSlot) < len(pl.Inventory.Items) {
		if pl.Inventory.Items[s.SelectedSlot].Count > 0 {
			pl.Inventory.Items[s.SelectedSlot].Count--
			if pl.Inventory.Items[s.SelectedSlot].Count <= 0 {
				pl.Inventory.Items[s.SelectedSlot] = player.ItemStack{}
			}
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
	return world.IsReplaceableBlock(stateID)
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

// broadcastSwing sends the arm-swing animation to other Java sessions and
// publishes EventSwing so Bedrock viewers also see the swing. In Minecraft 26.3
// (protocol 777), arm swinging uses ClientboundGameSwingAnimation (packet 123:
// entityId(VarInt), hand(VarInt), SwingAnimation{type(VarInt), duration(VarInt)})
// rather than ClientboundGameAnimate (which now only handles wake-up/crit particles).
func (s *PlayerSession) broadcastSwing(includeSelf bool) {
	pkt := pk.Marshal(
		packetid.ClientboundGameSwingAnimation,
		pk.VarInt(s.EntityID()),
		pk.VarInt(0), // InteractionHand.MAIN_HAND = 0
		pk.VarInt(1), // SwingAnimationType.WHACK = 1
		pk.VarInt(6), // SwingAnimation.DEFAULT.duration = 6
	)
	if includeSelf {
		s.Bridge.sessions.Broadcast(pkt)
	} else {
		s.Bridge.sessions.BroadcastExcept(s.UUID(), pkt)
	}
	if s.Bridge != nil && s.Bridge.pm != nil {
		s.Bridge.pm.PublishSwing(s.UUID())
	}
}

func (s *PlayerSession) HandleSwing(p pk.Packet) {
	// In Minecraft 26.3 (protocol 777), ServerboundPunchPacket (packet 46) is a
	// 0-byte unit record (StreamCodec.unit(INSTANCE)). Do not fail when p.Data is empty.
	var hand pk.VarInt
	if len(p.Data) > 0 {
		_ = p.Scan(&hand)
	}
	s.broadcastSwing(false)
}
