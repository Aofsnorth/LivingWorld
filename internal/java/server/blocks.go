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

	// Track crack state for cross-edition animation
	switch status {
	case 0: // start digging
		hadPrev, prevX, prevY, prevZ := s.Bridge.wm.CrackManager().StartBreaking(s.UUID(), pos.X, pos.Y, pos.Z)
		if hadPrev {
			// Player switched to a new block - stop crack on old block
			// TODO: Send stop crack to Bedrock clients
			_ = prevX
			_ = prevY
			_ = prevZ
		}
		// TODO: Broadcast start crack to Bedrock clients
	case 1: // cancel digging
		s.Bridge.wm.CrackManager().StopBreaking(s.UUID())
		// TODO: Broadcast stop crack to Bedrock clients
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
		// Survival placeholder: until real held-item placement is implemented,
		// don't conjure stone. Re-sync both clicked and target block to roll back
		// client prediction.
		currentID := s.Bridge.wm.GetDefaultWorld().GetBlock(pos.X, pos.Y, pos.Z).ID()
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
		targetID := s.Bridge.wm.GetDefaultWorld().GetBlock(x, y, z).ID()
		_ = s.SendPacket(pk.Marshal(packetid.ClientboundGameBlockUpdate, pk.Position{X: x, Y: y, Z: z}, pk.VarInt(livingWorldBlockIDToJavaStateID(targetID))))
		return
	}

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
	s.Bridge.wm.SetBlockAndPublish(world.BlockUpdateSourceJava, x, y, z, world.BlockByID(int32(stateID)))

	// Decrement held item count (survival item consumption)
	pl := s.Bridge.pm.GetPlayer(s.UUID())
	if pl != nil && pl.Inventory != nil && s.SelectedSlot >= 0 && s.SelectedSlot < 9 && int(s.SelectedSlot) < len(pl.Inventory.Items) {
		if pl.Inventory.Items[s.SelectedSlot].Count > 0 {
			pl.Inventory.Items[s.SelectedSlot].Count--
			s.syncInventory()
		}
	}
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
