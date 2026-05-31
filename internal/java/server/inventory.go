package server

import (
	"bytes"

	"livingworld/internal/item"
	"livingworld/internal/player"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// itemNetworkID resolves an item name to its Java protocol id.
func itemNetworkID(name string) (int32, bool) {
	it, ok := item.ByName(name)
	if !ok {
		return 0, false
	}
	return it.ID, true
}

// writeSlot writes a network ItemStack (MC 26.1): VarInt count; if count<=0 the
// slot is empty; else VarInt itemID + DataComponentPatch (VarInt nAdd=0, VarInt
// nRemove=0). Verified from the decompiled 26.1 jar (ItemStack$1.encode).
func writeSlot(buf *bytes.Buffer, st player.ItemStack) {
	if st.ID == 0 || st.Count <= 0 {
		pk.VarInt(0).WriteTo(buf)
		return
	}
	pk.VarInt(int32(st.Count)).WriteTo(buf)
	pk.VarInt(st.ID).WriteTo(buf)
	pk.VarInt(0).WriteTo(buf) // components added
	pk.VarInt(0).WriteTo(buf) // components removed
}

func (s *PlayerSession) sendInventory() {
	var buf bytes.Buffer
	pk.VarInt(0).WriteTo(&buf)
	pk.VarInt(0).WriteTo(&buf)
	pk.VarInt(46).WriteTo(&buf)
	for i := 0; i < 46; i++ {
		pk.VarInt(0).WriteTo(&buf)
	}
	pk.VarInt(0).WriteTo(&buf)
	_ = s.SendPacket(pk.Packet{
		ID:   int32(packetid.ClientboundGameContainerSetContent),
		Data: buf.Bytes(),
	})
}

// syncInventory pushes the player's full inventory contents to the client using
// ClientboundGameContainerSetContent (window 0). Called after a pickup so the new
// item appears. Layout: VarInt windowID, VarInt stateId, VarInt count, count×Slot,
// then the carried (cursor) Slot.
//
// IMPORTANT: Java window 0 has a special layout (crafting result + grid + armor +
// main + hotbar + offhand). The internal inventory is a flat 46-slot array, so we
// must remap indices when sending to the client:
//
//	Client slot 0      = crafting result (always empty, not stored)
//	Client slot 1-4    = crafting grid 2×2 (not stored)
//	Client slot 5-8    = armor slots (not stored yet)
//	Client slot 9-35   = main inventory (internal 9-35)
//	Client slot 36-44  = hotbar (internal 0-8)
//	Client slot 45     = offhand (internal 40)
func (s *PlayerSession) syncInventory() {
	pl := s.Bridge.pm.GetPlayer(s.UUID())
	if pl == nil || pl.Inventory == nil {
		return
	}
	items := pl.Inventory.Items
	var buf bytes.Buffer
	pk.VarInt(0).WriteTo(&buf)  // window 0 = player inventory
	pk.VarInt(0).WriteTo(&buf)  // state id
	pk.VarInt(46).WriteTo(&buf) // 46 slots total in window 0

	// Slot 0: crafting result (always empty)
	writeSlot(&buf, player.ItemStack{})
	// Slot 1-4: crafting grid 2×2 (always empty for now)
	for i := 0; i < 4; i++ {
		writeSlot(&buf, player.ItemStack{})
	}
	// Slot 5-8: armor (helmet, chest, legs, boots) — not stored yet, send empty
	for i := 0; i < 4; i++ {
		writeSlot(&buf, player.ItemStack{})
	}
	// Slot 9-35: main inventory (internal 9-35)
	for i := 9; i <= 35; i++ {
		if i < len(items) {
			writeSlot(&buf, items[i])
		} else {
			writeSlot(&buf, player.ItemStack{})
		}
	}
	// Slot 36-44: hotbar (internal 0-8)
	for i := 0; i < 9; i++ {
		if i < len(items) {
			writeSlot(&buf, items[i])
		} else {
			writeSlot(&buf, player.ItemStack{})
		}
	}
	// Slot 45: offhand (internal 40)
	if 40 < len(items) {
		writeSlot(&buf, items[40])
	} else {
		writeSlot(&buf, player.ItemStack{})
	}

	writeSlot(&buf, player.ItemStack{}) // carried item = empty
	_ = s.SendPacket(pk.Packet{
		ID:   int32(packetid.ClientboundGameContainerSetContent),
		Data: buf.Bytes(),
	})
}

func (s *PlayerSession) HandleCreativeSlot(p pk.Packet) {
	var slot pk.Short
	if err := p.Scan(&slot); err != nil {
		return
	}
}

func (s *PlayerSession) HandleSetCarriedItem(p pk.Packet) {
	var slot pk.Short
	if err := p.Scan(&slot); err != nil {
		return
	}
	s.mu.Lock()
	s.SelectedSlot = int32(slot)
	s.mu.Unlock()
	// Publish the held-slot change so other clients (both editions) re-render
	// this player's hand item.
	s.Bridge.pm.UpdateHeldSlot(s.UUID(), int(slot))
}
