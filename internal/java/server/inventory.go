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

// HandleCreativeSlot processes ServerboundSetCreativeModeSlot: a creative-mode
// client placing/clearing an item directly into a window-0 slot. Previously a
// no-op, so creative items were never recorded and never rendered in the player's
// hand for anyone else. We decode the stack, write it into the internal inventory,
// echo the change back to the creator, and publish an equipment change so other
// players (both editions) re-render the held item.
func (s *PlayerSession) HandleCreativeSlot(p pk.Packet) {
	slot, st, ok := readCreativeSlot(p.Data)
	if !ok {
		return
	}
	pl := s.Bridge.pm.GetPlayer(s.UUID())
	if pl == nil || pl.Inventory == nil {
		return
	}
	internal, ok := clientSlotToInternal(slot)
	if !ok {
		return // crafting result/grid/armor: not modeled by the flat inventory
	}
	pl.Inventory.SetItem(internal, st)
	s.syncInventory() // keep the creator's own client in sync
	// Only the held hotbar slot / offhand affect the rendered hand, so skip event
	// spam for pure main-inventory edits.
	if internal == pl.Inventory.HeldSlot || internal == 40 {
		s.Bridge.pm.PublishEquipmentChange(s.UUID())
	}
}

// readCreativeSlot decodes a ServerboundSetCreativeModeSlot body: Short slotIndex
// then a network ItemStack (mirrors writeSlot: VarInt count; if count>0 VarInt
// itemID + VarInt nAdd + VarInt nRemove). count<=0 means clear the slot. The
// item id + count are read before the component counts, so this server's
// simplified 0/0 component model is preserved and any component bytes are ignored.
func readCreativeSlot(data []byte) (slot int16, st player.ItemStack, ok bool) {
	r := bytes.NewReader(data)
	var sh pk.Short
	if _, err := sh.ReadFrom(r); err != nil {
		return 0, player.ItemStack{}, false
	}
	var cnt pk.VarInt
	if _, err := cnt.ReadFrom(r); err != nil {
		return 0, player.ItemStack{}, false
	}
	if cnt <= 0 {
		return int16(sh), player.ItemStack{}, true // empty / cleared slot
	}
	var id pk.VarInt
	if _, err := id.ReadFrom(r); err != nil {
		return 0, player.ItemStack{}, false
	}
	return int16(sh), player.ItemStack{ID: int32(id), Count: int8(cnt)}, true
}

// clientSlotToInternal maps a window-0 client slot index to the internal flat
// 46-slot array — the inverse of syncInventory's remap. Slots that aren't stored
// (crafting result/grid/armor) return ok=false.
func clientSlotToInternal(client int16) (int, bool) {
	switch {
	case client >= 9 && client <= 35: // main inventory (identity)
		return int(client), true
	case client >= 36 && client <= 44: // hotbar -> internal 0-8
		return int(client - 36), true
	case client == 45: // offhand -> internal 40
		return 40, true
	default: // 0 result, 1-4 grid, 5-8 armor: not modeled
		return 0, false
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
