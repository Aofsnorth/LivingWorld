package server

import (
	"bytes"
	"strings"
	"sync"

	"livingworld/internal/item"
	"livingworld/internal/player"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

const (
	winTypeGeneric9x3   = 2
	winTypeGeneric3x3   = 6
	winTypeAnvil        = 8
	winTypeBlastFurnace = 10
	winTypeBrewingStand = 11
	winTypeCrafting     = 12
	winTypeEnchantment  = 13
	winTypeFurnace      = 14
	winTypeGrindstone   = 15
	winTypeHopper       = 16
	winTypeLoom         = 18
	winTypeShulkerBox   = 20
	winTypeSmithing     = 21
	winTypeSmoker       = 22
	winTypeCartography  = 23
	winTypeStonecutter  = 24
)

var (
	worldContainersMu sync.Mutex
	worldContainers   = make(map[[3]int]*[27]player.ItemStack)
)

func getWorldContainer(pos [3]int) *[27]player.ItemStack {
	worldContainersMu.Lock()
	defer worldContainersMu.Unlock()
	c, ok := worldContainers[pos]
	if !ok {
		c = new([27]player.ItemStack)
		worldContainers[pos] = c
	}
	return c
}

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

	// Slot 0: crafting result computed from 2×2 grid
	writeSlot(&buf, evalCraft2x2(s.CraftGrid2x2))
	// Slot 1-4: crafting grid 2×2
	for i := 0; i < 4; i++ {
		writeSlot(&buf, s.CraftGrid2x2[i])
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

	writeSlot(&buf, s.CarriedItem) // carried cursor item
	_ = s.SendPacket(pk.Packet{
		ID:   int32(packetid.ClientboundGameContainerSetContent),
		Data: buf.Bytes(),
	})
}

// HandleCreativeSlot processes ServerboundSetCreativeModeSlot: a creative-mode
// client placing/clearing an item directly into a window-0 slot, or dropping an
// item outside the creative inventory when slot < 0 (slot == -1).
func (s *PlayerSession) HandleCreativeSlot(p pk.Packet) {
	slot, st, ok := readCreativeSlot(p.Data)
	if !ok {
		return
	}
	pl := s.Bridge.pm.GetPlayer(s.UUID())
	if pl == nil || pl.Inventory == nil {
		return
	}
	if slot < 0 {
		if st.ID != 0 && st.Count > 0 {
			if def, ok := item.ByID(st.ID); ok && s.Bridge.wm != nil {
				s.Bridge.wm.PlayerDropItem(def.Name, int(st.Count), pl.Position.X, pl.Position.Y+1.3, pl.Position.Z, float64(pl.Rotation.Pitch), float64(pl.Rotation.Yaw))
				s.broadcastSwing(true)
			}
		}
		return
	}
	if slot >= 1 && slot <= 4 {
		s.CraftGrid2x2[slot-1] = st
		s.syncInventory()
		return
	}
	internal, ok := clientSlotToInternal(slot)
	if !ok {
		return // crafting result/grid/armor: not modeled by the flat inventory
	}
	pl.Inventory.SetItem(internal, st)
	// Do NOT call s.syncInventory() here: in Creative mode the client manages its
	// own Creative inventory cursor item locally (s.CarriedItem is empty on the
	// server), so echoing ClientboundContainerSetContent with an empty carriedItem
	// would wipe the item currently held on the player's cursor while dragging.
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

// HandleContainerClick processes ServerboundGameContainerClick (window 0 player inventory)
// so dropping items from the inventory UI (button/mode 4 Q/Ctrl+Q or clicking outside the
// inventory window at slot -999) and moving/swapping items in window 0 stay in sync with
// the authoritative server inventory, spawn real dropped item entities, and broadcast
// equipment changes when the held slot or offhand changes.
func (s *PlayerSession) HandleContainerClick(p pk.Packet) {
	pl := s.Bridge.pm.GetPlayer(s.UUID())
	if pl == nil || pl.Inventory == nil {
		return
	}
	r := bytes.NewReader(p.Data)
	var windowID pk.VarInt
	var stateID pk.VarInt
	var slotNum pk.Short
	var button pk.Byte
	var mode pk.VarInt
	if _, err := windowID.ReadFrom(r); err != nil {
		return
	}
	if _, err := stateID.ReadFrom(r); err != nil {
		return
	}
	if _, err := slotNum.ReadFrom(r); err != nil {
		return
	}
	if _, err := button.ReadFrom(r); err != nil {
		return
	}
	if _, err := mode.ReadFrom(r); err != nil {
		return
	}

	var changedCount pk.VarInt
	if _, err := changedCount.ReadFrom(r); err != nil || changedCount < 0 || changedCount > 128 {
		s.syncInventory()
		return
	}

	type slotChange struct {
		slot int16
		st   player.ItemStack
	}
	changes := make([]slotChange, 0, int(changedCount))
	for i := 0; i < int(changedCount); i++ {
		var cs pk.Short
		if _, err := cs.ReadFrom(r); err != nil {
			s.syncInventory()
			return
		}
		st, ok := readHashedSlot(r)
		if !ok {
			s.syncInventory()
			return
		}
		changes = append(changes, slotChange{slot: int16(cs), st: st})
	}
	carried, hasCarried := readHashedSlot(r)

	winID := int32(windowID)
	if winID != 0 && winID != 1 {
		return
	}
	wType := s.OpenWindowType
	if winID == 1 && wType == 0 {
		wType = winTypeCrafting
	}

	// Mode 5 is Quick Craft (drag across slots). Wait until the drag-end packet
	// (button 2, 6, or 10) which carries the final changedSlots and carriedItem.
	// Ignoring intermediate start/add packets (which use slotNum == -999 and empty
	// carriedItem) prevents falsely dropping the cursor stack onto the ground.
	if mode == 5 && button != 2 && button != 6 && button != 10 {
		return
	}

	// Authoritative output slot click (crafting result or station output)
	if slotNum >= 0 && isWindowOutputSlot(winID, wType, int16(slotNum)) {
		if winID == 0 || wType == winTypeCrafting {
			craftRes := s.evalWindowCraftResult(winID)
			if craftRes.ID != 0 && craftRes.Count > 0 {
				if mode == 1 {
					maxCrafts := s.maxCraftCount(winID)
					for c := 0; c < maxCrafts; c++ {
						curRes := s.evalWindowCraftResult(winID)
						if curRes.ID != craftRes.ID || curRes.Count != craftRes.Count {
							break
						}
						if !addItemToInventory(pl.Inventory, curRes) {
							break
						}
						s.consumeCraftIngredients(winID, 1)
					}
				} else if mode == 0 {
					maxStack := int(itemMaxStack(craftRes.ID))
					if s.CarriedItem.ID == 0 || s.CarriedItem.Count <= 0 {
						s.CarriedItem = craftRes
						s.consumeCraftIngredients(winID, 1)
					} else if s.CarriedItem.ID == craftRes.ID && int(s.CarriedItem.Count)+int(craftRes.Count) <= maxStack {
						s.CarriedItem.Count += craftRes.Count
						s.consumeCraftIngredients(winID, 1)
					}
				}
			}
		} else {
			stationRes := s.evalStationResult(wType)
			if stationRes.ID != 0 && stationRes.Count > 0 {
				if mode == 1 {
					maxCrafts := s.maxStationCrafts(wType)
					for c := 0; c < maxCrafts; c++ {
						curRes := s.evalStationResult(wType)
						if curRes.ID != stationRes.ID || curRes.Count != stationRes.Count {
							break
						}
						if !addItemToInventory(pl.Inventory, curRes) {
							break
						}
						s.consumeStationIngredients(wType, 1)
					}
				} else if mode == 0 {
					maxStack := int(itemMaxStack(stationRes.ID))
					if s.CarriedItem.ID == 0 || s.CarriedItem.Count <= 0 {
						s.CarriedItem = stationRes
						s.consumeStationIngredients(wType, 1)
					} else if s.CarriedItem.ID == stationRes.ID && int(s.CarriedItem.Count)+int(stationRes.Count) <= maxStack {
						s.CarriedItem.Count += stationRes.Count
						s.consumeStationIngredients(wType, 1)
					}
				}
			}
		}
		s.syncActiveWindow(winID)
		s.Bridge.pm.PublishEquipmentChange(s.UUID())
		return
	}

	// Snapshot old inventory slots so we can compute exact net removed items for throws.
	oldSlots := make(map[int]player.ItemStack, len(changes))
	for _, ch := range changes {
		if internal, ok := windowSlotToInternal(winID, wType, ch.slot); ok {
			if it := pl.Inventory.GetItem(internal); it != nil {
				oldSlots[internal] = *it
			}
		}
	}

	// Mode 4 is Throw (pressing Q / Ctrl+Q while hovering a slot in the inventory UI).
	// Clicking outside the inventory window (slot -999) in Mode 0 drops the carried cursor stack.
	isThrow := mode == 4
	droppedAny := false

	// Clicking outside the inventory window (slot -999 in Mode 0) while carrying an item on the cursor:
	if mode == 0 && slotNum == -999 && s.CarriedItem.ID != 0 && s.CarriedItem.Count > 0 {
		dropAmt := int(s.CarriedItem.Count)
		if button == 1 {
			dropAmt = 1
		}
		if dropAmt > int(s.CarriedItem.Count) {
			dropAmt = int(s.CarriedItem.Count)
		}
		if def, ok := item.ByID(s.CarriedItem.ID); ok && s.Bridge.wm != nil {
			s.Bridge.wm.PlayerDropItem(def.Name, dropAmt, pl.Position.X, pl.Position.Y+1.3, pl.Position.Z, float64(pl.Rotation.Pitch), float64(pl.Rotation.Yaw))
			droppedAny = true
		}
		s.CarriedItem.Count -= int8(dropAmt)
		if s.CarriedItem.Count <= 0 {
			s.CarriedItem = player.ItemStack{}
		}
	} else if isThrow && slotNum >= 0 && len(changes) == 0 {
		// Direct Q / Ctrl+Q on a slot if client sent 0 changedSlots entries:
		if topIdx, ok := isTopWindowSlot(winID, wType, int16(slotNum)); ok {
			cur := s.getTopWindowSlot(winID, wType, topIdx)
			if cur.ID != 0 && cur.Count > 0 {
				dropAmt := 1
				if button != 0 {
					dropAmt = int(cur.Count)
				}
				if dropAmt > int(cur.Count) {
					dropAmt = int(cur.Count)
				}
				if def, ok := item.ByID(cur.ID); ok && s.Bridge.wm != nil {
					s.Bridge.wm.PlayerDropItem(def.Name, dropAmt, pl.Position.X, pl.Position.Y+1.3, pl.Position.Z, float64(pl.Rotation.Pitch), float64(pl.Rotation.Yaw))
					droppedAny = true
				}
				cur.Count -= int8(dropAmt)
				if cur.Count <= 0 {
					cur = player.ItemStack{}
				}
				s.setTopWindowSlot(winID, wType, topIdx, cur)
			}
		} else if internal, ok := windowSlotToInternal(winID, wType, int16(slotNum)); ok {
			cur := pl.Inventory.GetItem(internal)
			if cur != nil && cur.ID != 0 && cur.Count > 0 {
				dropAmt := 1
				if button != 0 {
					dropAmt = int(cur.Count)
				}
				if dropAmt > int(cur.Count) {
					dropAmt = int(cur.Count)
				}
				if def, ok := item.ByID(cur.ID); ok && s.Bridge.wm != nil {
					s.Bridge.wm.PlayerDropItem(def.Name, dropAmt, pl.Position.X, pl.Position.Y+1.3, pl.Position.Z, float64(pl.Rotation.Pitch), float64(pl.Rotation.Yaw))
					droppedAny = true
				}
				cur.Count -= int8(dropAmt)
				if cur.Count <= 0 {
					pl.Inventory.SetItem(internal, player.ItemStack{})
				}
			}
		}
	} else {
		// Apply client-reported slot changes and track any net reduction for throws.
		for _, ch := range changes {
			if isWindowOutputSlot(winID, wType, ch.slot) {
				continue
			}
			if topIdx, ok := isTopWindowSlot(winID, wType, ch.slot); ok {
				prev := s.getTopWindowSlot(winID, wType, topIdx)
				s.setTopWindowSlot(winID, wType, topIdx, ch.st)
				if isThrow && prev.ID != 0 && prev.Count > 0 {
					var dropped int
					if ch.st.ID == 0 || ch.st.Count <= 0 {
						dropped = int(prev.Count)
					} else if ch.st.ID == prev.ID && ch.st.Count < prev.Count {
						dropped = int(prev.Count - ch.st.Count)
					}
					if dropped > 0 {
						if def, ok := item.ByID(prev.ID); ok && s.Bridge.wm != nil {
							s.Bridge.wm.PlayerDropItem(def.Name, dropped, pl.Position.X, pl.Position.Y+1.3, pl.Position.Z, float64(pl.Rotation.Pitch), float64(pl.Rotation.Yaw))
							droppedAny = true
						}
					}
				}
				continue
			}
			internal, ok := windowSlotToInternal(winID, wType, ch.slot)
			if !ok {
				continue
			}
			prev := oldSlots[internal]
			pl.Inventory.SetItem(internal, ch.st)
			if isThrow && prev.ID != 0 && prev.Count > 0 {
				var dropped int
				if ch.st.ID == 0 || ch.st.Count <= 0 {
					dropped = int(prev.Count)
				} else if ch.st.ID == prev.ID && ch.st.Count < prev.Count {
					dropped = int(prev.Count - ch.st.Count)
				}
				if dropped > 0 {
					if def, ok := item.ByID(prev.ID); ok && s.Bridge.wm != nil {
						s.Bridge.wm.PlayerDropItem(def.Name, dropped, pl.Position.X, pl.Position.Y+1.3, pl.Position.Z, float64(pl.Rotation.Pitch), float64(pl.Rotation.Yaw))
						droppedAny = true
					}
				}
			}
		}
		if hasCarried {
			s.CarriedItem = carried
		}
	}

	if droppedAny {
		s.broadcastSwing(true)
	}
	s.syncActiveWindow(winID)
	s.Bridge.pm.PublishEquipmentChange(s.UUID())
}

// HandleContainerClose drops any item still held on the cursor when closing the
// inventory window, returns crafting/station items to inventory, and resyncs.
func (s *PlayerSession) HandleContainerClose(p pk.Packet) {
	pl := s.Bridge.pm.GetPlayer(s.UUID())
	if s.CarriedItem.ID != 0 && s.CarriedItem.Count > 0 {
		if pl != nil {
			if def, ok := item.ByID(s.CarriedItem.ID); ok && s.Bridge.wm != nil {
				s.Bridge.wm.PlayerDropItem(def.Name, int(s.CarriedItem.Count), pl.Position.X, pl.Position.Y+1.3, pl.Position.Z, float64(pl.Rotation.Pitch), float64(pl.Rotation.Yaw))
				s.broadcastSwing(true)
			}
		}
		s.CarriedItem = player.ItemStack{}
	}
	s.returnCraftingGridItems(pl)
	s.OpenWindowID = 0
	s.OpenWindowType = 0
	s.syncInventory()
	s.Bridge.pm.PublishEquipmentChange(s.UUID())
}

// readHashedSlot decodes a 1.21.5+ HashedSlot (or legacy Slot) from ServerboundContainerClick:
// Boolean present; if present: VarInt itemID, VarInt count, VarInt nAddedComponents,
// [VarInt type + Int32 crc32c]..., VarInt nRemovedComponents, [VarInt type]...
func readHashedSlot(r *bytes.Reader) (player.ItemStack, bool) {
	var present pk.Boolean
	if _, err := present.ReadFrom(r); err != nil {
		return player.ItemStack{}, false
	}
	if !present {
		return player.ItemStack{}, true
	}
	var itemID pk.VarInt
	var count pk.VarInt
	if _, err := itemID.ReadFrom(r); err != nil {
		return player.ItemStack{}, false
	}
	if _, err := count.ReadFrom(r); err != nil {
		return player.ItemStack{}, false
	}
	var nAdd pk.VarInt
	if _, err := nAdd.ReadFrom(r); err != nil || nAdd < 0 || nAdd > 256 {
		return player.ItemStack{}, false
	}
	for i := 0; i < int(nAdd); i++ {
		var compType pk.VarInt
		var hash pk.Int
		if _, err := compType.ReadFrom(r); err != nil {
			return player.ItemStack{}, false
		}
		if _, err := hash.ReadFrom(r); err != nil {
			return player.ItemStack{}, false
		}
	}
	var nRem pk.VarInt
	if _, err := nRem.ReadFrom(r); err != nil || nRem < 0 || nRem > 256 {
		return player.ItemStack{}, false
	}
	for i := 0; i < int(nRem); i++ {
		var compType pk.VarInt
		if _, err := compType.ReadFrom(r); err != nil {
			return player.ItemStack{}, false
		}
	}
	if count <= 0 || itemID <= 0 {
		return player.ItemStack{}, true
	}
	return player.ItemStack{ID: int32(itemID), Count: int8(count)}, true
}

func (s *PlayerSession) openBlockContainer(blockName string, x, y, z int) bool {
	base := strings.TrimPrefix(blockName, "minecraft:")
	var wType int32
	var title string

	switch {
	case base == "crafting_table":
		wType = winTypeCrafting
		title = "Crafting"
	case base == "furnace":
		wType = winTypeFurnace
		title = "Furnace"
	case base == "smoker":
		wType = winTypeSmoker
		title = "Smoker"
	case base == "blast_furnace":
		wType = winTypeBlastFurnace
		title = "Blast Furnace"
	case base == "stonecutter":
		wType = winTypeStonecutter
		title = "Stonecutter"
	case base == "anvil" || base == "chipped_anvil" || base == "damaged_anvil":
		wType = winTypeAnvil
		title = "Repair & Name"
	case base == "grindstone":
		wType = winTypeGrindstone
		title = "Repair & Disenchant"
	case base == "smithing_table":
		wType = winTypeSmithing
		title = "Upgrade Gear"
	case base == "cartography_table":
		wType = winTypeCartography
		title = "Cartography Table"
	case base == "loom":
		wType = winTypeLoom
		title = "Loom"
	case base == "enchanting_table":
		wType = winTypeEnchantment
		title = "Enchant"
	case base == "brewing_stand":
		wType = winTypeBrewingStand
		title = "Brewing Stand"
	case base == "chest" || base == "trapped_chest" || strings.HasSuffix(base, "copper_chest"):
		wType = winTypeGeneric9x3
		title = "Chest"
	case base == "barrel":
		wType = winTypeGeneric9x3
		title = "Barrel"
	case base == "ender_chest":
		wType = winTypeGeneric9x3
		title = "Ender Chest"
	case strings.HasSuffix(base, "shulker_box"):
		wType = winTypeShulkerBox
		title = "Shulker Box"
	case base == "dispenser":
		wType = winTypeGeneric3x3
		title = "Dispenser"
	case base == "dropper":
		wType = winTypeGeneric3x3
		title = "Dropper"
	case base == "crafter":
		wType = winTypeGeneric3x3
		title = "Crafter"
	case base == "hopper":
		wType = winTypeHopper
		title = "Hopper"
	default:
		return false
	}

	if pl := s.Bridge.pm.GetPlayer(s.UUID()); pl != nil {
		s.returnCraftingGridItems(pl)
	}

	s.OpenWindowID = 1
	s.OpenWindowType = wType
	s.OpenBlockPos = [3]int{x, y, z}

	var openBuf bytes.Buffer
	_, _ = pk.VarInt(1).WriteTo(&openBuf)
	_, _ = pk.VarInt(wType).WriteTo(&openBuf)
	openBuf.WriteByte(0x08) // NBT Tag_String
	openBuf.WriteByte(0x00)
	openBuf.WriteByte(byte(len(title)))
	openBuf.WriteString(title)
	_ = s.SendPacket(pk.Packet{
		ID:   int32(packetid.ClientboundGameOpenScreen),
		Data: openBuf.Bytes(),
	})
	s.syncOpenWindow()
	return true
}

func (s *PlayerSession) openCraftingTable() {
	_ = s.openBlockContainer("minecraft:crafting_table", 0, 0, 0)
}

func (s *PlayerSession) syncOpenWindow() {
	pl := s.Bridge.pm.GetPlayer(s.UUID())
	if pl == nil || pl.Inventory == nil {
		return
	}
	wType := s.OpenWindowType
	if wType == 0 {
		wType = winTypeCrafting
	}
	topCount := windowTopSlotCount(wType)
	totalSlots := topCount + 36
	items := pl.Inventory.Items

	var buf bytes.Buffer
	_, _ = pk.VarInt(1).WriteTo(&buf)
	_, _ = pk.VarInt(0).WriteTo(&buf)
	_, _ = pk.VarInt(int32(totalSlots)).WriteTo(&buf)

	for i := 0; i < topCount; i++ {
		if isWindowOutputSlot(1, wType, int16(i)) {
			if wType == winTypeCrafting {
				writeSlot(&buf, evalCraft3x3(s.CraftGrid3x3))
			} else {
				writeSlot(&buf, s.evalStationResult(wType))
			}
		} else {
			writeSlot(&buf, s.getTopWindowSlot(1, wType, i))
		}
	}

	for i := 9; i < 36; i++ {
		if i < len(items) {
			writeSlot(&buf, items[i])
		} else {
			writeSlot(&buf, player.ItemStack{})
		}
	}
	for i := 0; i < 9; i++ {
		if i < len(items) {
			writeSlot(&buf, items[i])
		} else {
			writeSlot(&buf, player.ItemStack{})
		}
	}
	writeSlot(&buf, s.CarriedItem)

	_ = s.SendPacket(pk.Packet{
		ID:   int32(packetid.ClientboundGameContainerSetContent),
		Data: buf.Bytes(),
	})

	// Send container properties for furnace/smoker/blast_furnace and anvil so the UI renders active state.
	switch wType {
	case winTypeFurnace, winTypeSmoker, winTypeBlastFurnace:
		res := s.evalStationResult(wType)
		lit := int16(0)
		prog := int16(0)
		if res.ID != 0 && res.Count > 0 {
			lit = 200
			prog = 200
		}
		s.sendContainerData(1, 0, lit)  // fuel left
		s.sendContainerData(1, 1, 200)  // max fuel
		s.sendContainerData(1, 2, prog) // cook progress
		s.sendContainerData(1, 3, 200)  // max cook time
	case winTypeAnvil:
		res := s.evalStationResult(wType)
		cost := int16(0)
		if res.ID != 0 && res.Count > 0 {
			cost = 1
		}
		s.sendContainerData(1, 0, cost)
	}
}

func (s *PlayerSession) sendContainerData(winID int32, prop, val int16) {
	_ = s.SendPacket(pk.Marshal(
		packetid.ClientboundGameContainerSetData,
		pk.VarInt(winID),
		pk.Short(prop),
		pk.Short(val),
	))
}

func (s *PlayerSession) syncActiveWindow(winID int32) {
	if winID == 1 && s.OpenWindowID == 1 {
		s.syncOpenWindow()
	} else {
		s.syncInventory()
	}
}

func windowTopSlotCount(wType int32) int {
	switch wType {
	case winTypeCrafting:
		return 10
	case winTypeFurnace, winTypeSmoker, winTypeBlastFurnace, winTypeAnvil, winTypeGrindstone, winTypeCartography:
		return 3
	case winTypeSmithing, winTypeLoom:
		return 4
	case winTypeStonecutter, winTypeEnchantment:
		return 2
	case winTypeBrewingStand, winTypeHopper:
		return 5
	case winTypeGeneric3x3:
		return 9
	case winTypeGeneric9x3, winTypeShulkerBox:
		return 27
	default:
		return 10
	}
}

func isStorageWindowType(wType int32) bool {
	switch wType {
	case winTypeGeneric9x3, winTypeShulkerBox, winTypeGeneric3x3, winTypeHopper:
		return true
	default:
		return false
	}
}

func isWindowOutputSlot(winID, wType int32, slot int16) bool {
	if winID == 0 {
		return slot == 0
	}
	if winID == 1 {
		switch wType {
		case winTypeCrafting:
			return slot == 0
		case winTypeFurnace, winTypeSmoker, winTypeBlastFurnace, winTypeAnvil, winTypeGrindstone, winTypeCartography:
			return slot == 2
		case winTypeSmithing, winTypeLoom:
			return slot == 3
		case winTypeStonecutter:
			return slot == 1
		}
	}
	return false
}

func windowSlotToInternal(winID, wType int32, client int16) (int, bool) {
	if winID == 1 {
		top := int16(windowTopSlotCount(wType))
		if client >= top && client <= top+26 {
			return int(client - top + 9), true
		}
		if client >= top+27 && client <= top+35 {
			return int(client - (top + 27)), true
		}
		return 0, false
	}
	return clientSlotToInternal(client)
}

func isTopWindowSlot(winID, wType int32, slot int16) (int, bool) {
	if isWindowOutputSlot(winID, wType, slot) {
		return 0, false
	}
	if winID == 0 {
		if slot >= 1 && slot <= 4 {
			return int(slot - 1), true
		}
		return 0, false
	}
	if winID == 1 {
		top := int16(windowTopSlotCount(wType))
		if slot >= 0 && slot < top {
			return int(slot), true
		}
	}
	return 0, false
}

func (s *PlayerSession) isOpenEnderChest() bool {
	if s.Bridge == nil || s.Bridge.wm == nil {
		return false
	}
	w := s.Bridge.wm.GetDefaultWorld()
	if w == nil {
		return false
	}
	blk := w.GetBlock(s.OpenBlockPos[0], s.OpenBlockPos[1], s.OpenBlockPos[2])
	return blockStateName(blk) == "minecraft:ender_chest"
}

func (s *PlayerSession) getTopWindowSlot(winID, wType int32, idx int) player.ItemStack {
	if winID == 0 {
		if idx >= 0 && idx < 4 {
			return s.CraftGrid2x2[idx]
		}
		return player.ItemStack{}
	}
	if wType == winTypeCrafting {
		gridIdx := idx - 1
		if gridIdx >= 0 && gridIdx < 9 {
			return s.CraftGrid3x3[gridIdx]
		}
		return player.ItemStack{}
	}
	if isStorageWindowType(wType) {
		if idx >= 0 && idx < 27 {
			if s.isOpenEnderChest() {
				return s.EnderChest[idx]
			}
			c := getWorldContainer(s.OpenBlockPos)
			worldContainersMu.Lock()
			st := c[idx]
			worldContainersMu.Unlock()
			return st
		}
		return player.ItemStack{}
	}
	if idx >= 0 && idx < len(s.StationSlots) {
		return s.StationSlots[idx]
	}
	return player.ItemStack{}
}

func (s *PlayerSession) setTopWindowSlot(winID, wType int32, idx int, st player.ItemStack) {
	if st.Count <= 0 || st.ID <= 0 {
		st = player.ItemStack{}
	}
	if winID == 0 {
		if idx >= 0 && idx < 4 {
			s.CraftGrid2x2[idx] = st
		}
		return
	}
	if wType == winTypeCrafting {
		gridIdx := idx - 1
		if gridIdx >= 0 && gridIdx < 9 {
			s.CraftGrid3x3[gridIdx] = st
		}
		return
	}
	if isStorageWindowType(wType) {
		if idx >= 0 && idx < 27 {
			if s.isOpenEnderChest() {
				s.EnderChest[idx] = st
				return
			}
			c := getWorldContainer(s.OpenBlockPos)
			worldContainersMu.Lock()
			c[idx] = st
			worldContainersMu.Unlock()
		}
		return
	}
	if idx >= 0 && idx < len(s.StationSlots) {
		s.StationSlots[idx] = st
	}
}

func (s *PlayerSession) evalStationResult(wType int32) player.ItemStack {
	switch wType {
	case winTypeFurnace, winTypeSmoker, winTypeBlastFurnace:
		in := baseItemName(s.StationSlots[0])
		if in == "" {
			return player.ItemStack{}
		}
		outName := smeltOutputForInput(in, wType)
		if outName == "" {
			return player.ItemStack{}
		}
		return makeCraftStack(outName, 1)
	case winTypeStonecutter:
		in := baseItemName(s.StationSlots[0])
		if in == "" {
			return player.ItemStack{}
		}
		outName, cnt := stonecutterOutputForInput(in)
		if outName == "" {
			return player.ItemStack{}
		}
		return makeCraftStack(outName, cnt)
	case winTypeAnvil, winTypeGrindstone:
		in0 := s.StationSlots[0]
		in1 := s.StationSlots[1]
		if in0.ID <= 0 || in0.Count <= 0 {
			return player.ItemStack{}
		}
		out := in0
		if in1.ID == in0.ID && in1.Count > 0 {
			maxS := itemMaxStack(in0.ID)
			sum := int(in0.Count) + int(in1.Count)
			if sum > int(maxS) {
				sum = int(maxS)
			}
			out.Count = int8(sum)
		}
		return out
	case winTypeSmithing:
		baseIn := baseItemName(s.StationSlots[1])
		addIn := baseItemName(s.StationSlots[2])
		if baseIn == "" {
			baseIn = baseItemName(s.StationSlots[0])
		}
		if strings.HasPrefix(baseIn, "diamond_") && (addIn == "netherite_ingot" || baseItemName(s.StationSlots[1]) == "netherite_ingot") {
			suffix := strings.TrimPrefix(baseIn, "diamond_")
			return makeCraftStack("netherite_"+suffix, 1)
		}
		return player.ItemStack{}
	case winTypeCartography:
		in0 := baseItemName(s.StationSlots[0])
		if in0 == "paper" || in0 == "map" || in0 == "filled_map" {
			return makeCraftStack("map", 1)
		}
		return player.ItemStack{}
	case winTypeLoom:
		if s.StationSlots[0].ID > 0 && s.StationSlots[0].Count > 0 {
			return player.ItemStack{ID: s.StationSlots[0].ID, Count: 1}
		}
		return player.ItemStack{}
	}
	return player.ItemStack{}
}

func (s *PlayerSession) maxStationCrafts(wType int32) int {
	switch wType {
	case winTypeFurnace, winTypeSmoker, winTypeBlastFurnace, winTypeStonecutter, winTypeCartography, winTypeLoom:
		if s.StationSlots[0].ID > 0 && s.StationSlots[0].Count > 0 {
			return int(s.StationSlots[0].Count)
		}
		return 0
	case winTypeAnvil, winTypeGrindstone, winTypeSmithing:
		return 1
	}
	return 0
}

func (s *PlayerSession) consumeStationIngredients(wType int32, times int) {
	if times <= 0 {
		return
	}
	switch wType {
	case winTypeFurnace, winTypeSmoker, winTypeBlastFurnace:
		if s.StationSlots[0].ID > 0 && s.StationSlots[0].Count > 0 {
			s.StationSlots[0].Count -= int8(times)
			if s.StationSlots[0].Count <= 0 {
				s.StationSlots[0] = player.ItemStack{}
			}
		}
		if s.StationSlots[1].ID > 0 && s.StationSlots[1].Count > 0 && s.StationSlots[0].Count == 0 {
			s.StationSlots[1].Count--
			if s.StationSlots[1].Count <= 0 {
				s.StationSlots[1] = player.ItemStack{}
			}
		}
	case winTypeStonecutter, winTypeCartography:
		for i := 0; i < 2; i++ {
			if s.StationSlots[i].ID > 0 && s.StationSlots[i].Count > 0 {
				s.StationSlots[i].Count -= int8(times)
				if s.StationSlots[i].Count <= 0 {
					s.StationSlots[i] = player.ItemStack{}
				}
			}
		}
	case winTypeAnvil, winTypeGrindstone:
		s.StationSlots[0] = player.ItemStack{}
		s.StationSlots[1] = player.ItemStack{}
	case winTypeSmithing, winTypeLoom:
		for i := 0; i < 3; i++ {
			if s.StationSlots[i].ID > 0 && s.StationSlots[i].Count > 0 {
				s.StationSlots[i].Count -= int8(times)
				if s.StationSlots[i].Count <= 0 {
					s.StationSlots[i] = player.ItemStack{}
				}
			}
		}
	}
}

func smeltOutputForInput(in string, wType int32) string {
	// Food items (Furnace & Smoker)
	if wType == winTypeFurnace || wType == winTypeSmoker {
		switch in {
		case "beef":
			return "cooked_beef"
		case "porkchop":
			return "cooked_porkchop"
		case "chicken":
			return "cooked_chicken"
		case "mutton":
			return "cooked_mutton"
		case "rabbit":
			return "cooked_rabbit"
		case "cod":
			return "cooked_cod"
		case "salmon":
			return "cooked_salmon"
		case "potato":
			return "baked_potato"
		case "kelp":
			return "dried_kelp"
		}
		if wType == winTypeSmoker {
			return ""
		}
	}
	// Ores & metals (Furnace & Blast Furnace)
	if wType == winTypeFurnace || wType == winTypeBlastFurnace {
		switch in {
		case "raw_iron", "iron_ore", "deepslate_iron_ore":
			return "iron_ingot"
		case "raw_gold", "gold_ore", "deepslate_gold_ore", "nether_gold_ore":
			return "gold_ingot"
		case "raw_copper", "copper_ore", "deepslate_copper_ore":
			return "copper_ingot"
		case "diamond_ore", "deepslate_diamond_ore":
			return "diamond"
		case "emerald_ore", "deepslate_emerald_ore":
			return "emerald"
		case "coal_ore", "deepslate_coal_ore":
			return "coal"
		case "lapis_ore", "deepslate_lapis_ore":
			return "lapis_lazuli"
		case "redstone_ore", "deepslate_redstone_ore":
			return "redstone"
		case "nether_quartz_ore":
			return "quartz"
		case "ancient_debris":
			return "netherite_scrap"
		}
		if wType == winTypeBlastFurnace {
			return ""
		}
	}
	// General Furnace recipes
	if planks, _ := planksFromLog(in); planks != "" {
		return "charcoal"
	}
	switch in {
	case "cobblestone":
		return "stone"
	case "stone":
		return "smooth_stone"
	case "stone_bricks":
		return "cracked_stone_bricks"
	case "cobbled_deepslate":
		return "deepslate"
	case "sand", "red_sand":
		return "glass"
	case "clay_ball":
		return "brick"
	case "clay":
		return "terracotta"
	case "netherrack":
		return "nether_brick"
	case "sandstone":
		return "smooth_sandstone"
	case "red_sandstone":
		return "smooth_red_sandstone"
	case "quartz_block":
		return "smooth_quartz"
	case "cactus":
		return "green_dye"
	case "wet_sponge":
		return "sponge"
	}
	return ""
}

func stonecutterOutputForInput(in string) (string, int8) {
	switch in {
	case "stone":
		return "stone_bricks", 1
	case "stone_bricks":
		return "chiseled_stone_bricks", 1
	case "cobblestone":
		return "cobblestone_stairs", 1
	case "cobbled_deepslate":
		return "polished_deepslate", 1
	case "polished_deepslate":
		return "deepslate_bricks", 1
	case "deepslate_bricks":
		return "deepslate_tiles", 1
	case "sandstone":
		return "cut_sandstone", 1
	case "red_sandstone":
		return "cut_red_sandstone", 1
	case "andesite":
		return "polished_andesite", 1
	case "diorite":
		return "polished_diorite", 1
	case "granite":
		return "polished_granite", 1
	case "blackstone":
		return "polished_blackstone", 1
	case "polished_blackstone":
		return "polished_blackstone_bricks", 1
	case "tuff":
		return "polished_tuff", 1
	case "bricks":
		return "brick_stairs", 1
	case "quartz_block":
		return "quartz_pillar", 1
	case "copper_block":
		return "cut_copper", 4
	}
	return "", 0
}

func (s *PlayerSession) evalWindowCraftResult(winID int32) player.ItemStack {
	if winID == 1 {
		return evalCraft3x3(s.CraftGrid3x3)
	}
	return evalCraft2x2(s.CraftGrid2x2)
}

func (s *PlayerSession) maxCraftCount(winID int32) int {
	minCount := 64
	found := false
	if winID == 1 {
		for i := 0; i < 9; i++ {
			st := s.CraftGrid3x3[i]
			if st.ID > 0 && st.Count > 0 {
				found = true
				if int(st.Count) < minCount {
					minCount = int(st.Count)
				}
			}
		}
	} else {
		for i := 0; i < 4; i++ {
			st := s.CraftGrid2x2[i]
			if st.ID > 0 && st.Count > 0 {
				found = true
				if int(st.Count) < minCount {
					minCount = int(st.Count)
				}
			}
		}
	}
	if !found {
		return 0
	}
	return minCount
}

func (s *PlayerSession) consumeCraftIngredients(winID int32, times int) {
	if times <= 0 {
		return
	}
	if winID == 1 {
		for i := 0; i < 9; i++ {
			if s.CraftGrid3x3[i].ID > 0 && s.CraftGrid3x3[i].Count > 0 {
				s.CraftGrid3x3[i].Count -= int8(times)
				if s.CraftGrid3x3[i].Count <= 0 {
					s.CraftGrid3x3[i] = player.ItemStack{}
				}
			}
		}
	} else {
		for i := 0; i < 4; i++ {
			if s.CraftGrid2x2[i].ID > 0 && s.CraftGrid2x2[i].Count > 0 {
				s.CraftGrid2x2[i].Count -= int8(times)
				if s.CraftGrid2x2[i].Count <= 0 {
					s.CraftGrid2x2[i] = player.ItemStack{}
				}
			}
		}
	}
}

func (s *PlayerSession) returnCraftingGridItems(pl *player.Player) {
	if pl == nil || pl.Inventory == nil {
		return
	}
	for i := 0; i < 4; i++ {
		if s.CraftGrid2x2[i].ID > 0 && s.CraftGrid2x2[i].Count > 0 {
			addItemToInventory(pl.Inventory, s.CraftGrid2x2[i])
			s.CraftGrid2x2[i] = player.ItemStack{}
		}
	}
	for i := 0; i < 9; i++ {
		if s.CraftGrid3x3[i].ID > 0 && s.CraftGrid3x3[i].Count > 0 {
			addItemToInventory(pl.Inventory, s.CraftGrid3x3[i])
			s.CraftGrid3x3[i] = player.ItemStack{}
		}
	}
	for i := 0; i < len(s.StationSlots); i++ {
		if s.StationSlots[i].ID > 0 && s.StationSlots[i].Count > 0 {
			addItemToInventory(pl.Inventory, s.StationSlots[i])
			s.StationSlots[i] = player.ItemStack{}
		}
	}
}

func itemMaxStack(id int32) int8 {
	def, ok := item.ByID(id)
	if !ok {
		return 64
	}
	name := def.Name
	if strings.HasSuffix(name, "_sword") ||
		strings.HasSuffix(name, "_pickaxe") ||
		strings.HasSuffix(name, "_axe") ||
		strings.HasSuffix(name, "_shovel") ||
		strings.HasSuffix(name, "_hoe") ||
		strings.HasSuffix(name, "_helmet") ||
		strings.HasSuffix(name, "_chestplate") ||
		strings.HasSuffix(name, "_leggings") ||
		strings.HasSuffix(name, "_boots") ||
		name == "minecraft:shield" ||
		name == "minecraft:bow" ||
		name == "minecraft:crossbow" ||
		name == "minecraft:flint_and_steel" ||
		name == "minecraft:shears" ||
		strings.HasSuffix(name, "_bed") ||
		strings.HasSuffix(name, "_boat") {
		return 1
	}
	return 64
}

func addItemToInventory(inv *player.Inventory, st player.ItemStack) bool {
	if inv == nil || st.ID <= 0 || st.Count <= 0 {
		return false
	}
	maxStack := itemMaxStack(st.ID)
	remaining := st.Count

	if maxStack > 1 {
		for i := 0; i < 36 && remaining > 0; i++ {
			cur := inv.GetItem(i)
			if cur != nil && cur.ID == st.ID && cur.Count < maxStack {
				space := maxStack - cur.Count
				add := remaining
				if add > space {
					add = space
				}
				inv.SetItem(i, player.ItemStack{ID: st.ID, Count: cur.Count + add})
				remaining -= add
			}
		}
	}
	for i := 0; i < 36 && remaining > 0; i++ {
		cur := inv.GetItem(i)
		if cur == nil || cur.ID <= 0 || cur.Count <= 0 {
			add := remaining
			if add > maxStack {
				add = maxStack
			}
			inv.SetItem(i, player.ItemStack{ID: st.ID, Count: add})
			remaining -= add
		}
	}
	return remaining == 0
}

func makeCraftStack(name string, count int8) player.ItemStack {
	if id, ok := itemNetworkID(name); ok && id > 0 && count > 0 {
		return player.ItemStack{ID: id, Count: count}
	}
	return player.ItemStack{}
}

func baseItemName(st player.ItemStack) string {
	if st.ID <= 0 || st.Count <= 0 {
		return ""
	}
	def, ok := item.ByID(st.ID)
	if !ok {
		return ""
	}
	return strings.TrimPrefix(def.Name, "minecraft:")
}

func isPlanksName(name string) bool {
	return strings.HasSuffix(name, "_planks")
}

func woodTypeFromPlanks(name string) string {
	if strings.HasSuffix(name, "_planks") {
		return strings.TrimSuffix(name, "_planks")
	}
	return ""
}

func planksFromLog(name string) (string, int8) {
	n := strings.TrimPrefix(name, "stripped_")
	switch {
	case strings.HasSuffix(n, "_log"):
		return strings.TrimSuffix(n, "_log") + "_planks", 4
	case strings.HasSuffix(n, "_wood"):
		return strings.TrimSuffix(n, "_wood") + "_planks", 4
	case strings.HasSuffix(n, "_stem"):
		return strings.TrimSuffix(n, "_stem") + "_planks", 4
	case strings.HasSuffix(n, "_hyphae"):
		return strings.TrimSuffix(n, "_hyphae") + "_planks", 4
	case n == "bamboo_block":
		return "bamboo_planks", 2
	}
	return "", 0
}

func toolPrefixForMaterial(mat string) string {
	if isPlanksName(mat) {
		return "wooden"
	}
	switch mat {
	case "cobblestone", "blackstone", "cobbled_deepslate":
		return "stone"
	case "iron_ingot":
		return "iron"
	case "gold_ingot":
		return "golden"
	case "diamond":
		return "diamond"
	}
	return ""
}

func armorPrefixForMaterial(mat string) string {
	switch mat {
	case "leather":
		return "leather"
	case "iron_ingot":
		return "iron"
	case "gold_ingot":
		return "golden"
	case "diamond":
		return "diamond"
	}
	return ""
}

func evalCraft2x2(grid [4]player.ItemStack) player.ItemStack {
	var g9 [9]player.ItemStack
	g9[0] = grid[0]
	g9[1] = grid[1]
	g9[3] = grid[2]
	g9[4] = grid[3]
	return evalCraft3x3(g9)
}

func evalCraft3x3(grid [9]player.ItemStack) player.ItemStack {
	var names [3][3]string
	minR, maxR := 3, -1
	minC, maxC := 3, -1
	nonEmpty := 0
	var itemsList [9]string

	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			n := baseItemName(grid[r*3+c])
			names[r][c] = n
			if n != "" {
				if r < minR {
					minR = r
				}
				if r > maxR {
					maxR = r
				}
				if c < minC {
					minC = c
				}
				if c > maxC {
					maxC = c
				}
				itemsList[nonEmpty] = n
				nonEmpty++
			}
		}
	}

	if nonEmpty == 0 {
		return player.ItemStack{}
	}

	h := maxR - minR + 1
	w := maxC - minC + 1
	cell := func(dr, dc int) string {
		return names[minR+dr][minC+dc]
	}

	// 1. Single-item recipes (1x1 anywhere)
	if nonEmpty == 1 {
		in := itemsList[0]
		if planks, cnt := planksFromLog(in); planks != "" {
			return makeCraftStack(planks, cnt)
		}
		if wood := woodTypeFromPlanks(in); wood != "" {
			return makeCraftStack(wood+"_button", 1)
		}
		switch in {
		case "stone":
			return makeCraftStack("stone_button", 1)
		case "polished_blackstone":
			return makeCraftStack("polished_blackstone_button", 1)
		case "iron_block":
			return makeCraftStack("iron_ingot", 9)
		case "gold_block":
			return makeCraftStack("gold_ingot", 9)
		case "diamond_block":
			return makeCraftStack("diamond", 9)
		case "emerald_block":
			return makeCraftStack("emerald", 9)
		case "coal_block":
			return makeCraftStack("coal", 9)
		case "redstone_block":
			return makeCraftStack("redstone", 9)
		case "lapis_block":
			return makeCraftStack("lapis_lazuli", 9)
		case "copper_block":
			return makeCraftStack("copper_ingot", 9)
		case "hay_block":
			return makeCraftStack("wheat", 9)
		case "bone_block":
			return makeCraftStack("bone_meal", 9)
		case "bone":
			return makeCraftStack("bone_meal", 3)
		case "sugar_cane":
			return makeCraftStack("sugar", 1)
		}
		return player.ItemStack{}
	}

	// 2. Shapeless 2-item recipes
	if nonEmpty == 2 {
		a, b := itemsList[0], itemsList[1]
		if (a == "iron_ingot" && b == "flint") || (a == "flint" && b == "iron_ingot") {
			return makeCraftStack("flint_and_steel", 1)
		}
	}

	// 3. 1x2 vertical recipes
	if w == 1 && h == 2 {
		top, bot := cell(0, 0), cell(1, 0)
		if isPlanksName(top) && isPlanksName(bot) {
			return makeCraftStack("stick", 4)
		}
		if top == "bamboo" && bot == "bamboo" {
			return makeCraftStack("stick", 1)
		}
		if (top == "coal" || top == "charcoal") && bot == "stick" {
			return makeCraftStack("torch", 4)
		}
		if top == "redstone" && bot == "stick" {
			return makeCraftStack("redstone_torch", 1)
		}
	}

	// 4. 2x1 horizontal recipes
	if w == 2 && h == 1 {
		left, right := cell(0, 0), cell(0, 1)
		if left == right {
			if wood := woodTypeFromPlanks(left); wood != "" {
				return makeCraftStack(wood+"_pressure_plate", 1)
			}
			switch left {
			case "stone":
				return makeCraftStack("stone_pressure_plate", 1)
			case "iron_ingot":
				return makeCraftStack("heavy_weighted_pressure_plate", 1)
			case "gold_ingot":
				return makeCraftStack("light_weighted_pressure_plate", 1)
			}
		}
	}

	// 5. 2x2 recipes
	if w == 2 && h == 2 {
		c00, c01, c10, c11 := cell(0, 0), cell(0, 1), cell(1, 0), cell(1, 1)
		// Shears (diagonal iron ingots)
		if (c00 == "iron_ingot" && c11 == "iron_ingot" && c01 == "" && c10 == "") ||
			(c01 == "iron_ingot" && c10 == "iron_ingot" && c00 == "" && c11 == "") {
			return makeCraftStack("shears", 1)
		}
		if nonEmpty == 4 {
			if c00 == "string" && c01 == "string" && isPlanksName(c10) && isPlanksName(c11) {
				return makeCraftStack("loom", 1)
			}
			if isPlanksName(c00) && isPlanksName(c01) && isPlanksName(c10) && isPlanksName(c11) {
				return makeCraftStack("crafting_table", 1)
			}
			if c00 == c01 && c00 == c10 && c00 == c11 {
				switch c00 {
				case "sand":
					return makeCraftStack("sandstone", 1)
				case "red_sand":
					return makeCraftStack("red_sandstone", 1)
				case "stone":
					return makeCraftStack("stone_bricks", 4)
				case "snowball":
					return makeCraftStack("snow_block", 1)
				case "clay_ball":
					return makeCraftStack("clay", 1)
				case "brick":
					return makeCraftStack("bricks", 1)
				case "glowstone_dust":
					return makeCraftStack("glowstone", 1)
				case "quartz":
					return makeCraftStack("quartz_block", 1)
				case "string":
					return makeCraftStack("white_wool", 1)
				case "iron_ingot":
					return makeCraftStack("iron_trapdoor", 1)
				}
			}
		}
	}

	// 6. 1x3 vertical recipes (swords, shovels)
	if w == 1 && h == 3 {
		t0, t1, t2 := cell(0, 0), cell(1, 0), cell(2, 0)
		if t2 == "stick" && t0 == t1 {
			if pref := toolPrefixForMaterial(t0); pref != "" {
				return makeCraftStack(pref+"_sword", 1)
			}
		}
		if t1 == "stick" && t2 == "stick" {
			if pref := toolPrefixForMaterial(t0); pref != "" {
				return makeCraftStack(pref+"_shovel", 1)
			}
		}
	}

	// 7. 3x1 horizontal recipes (slabs, bread, paper)
	if w == 3 && h == 1 {
		a, b, c := cell(0, 0), cell(0, 1), cell(0, 2)
		if a == b && b == c {
			if wood := woodTypeFromPlanks(a); wood != "" {
				return makeCraftStack(wood+"_slab", 6)
			}
			switch a {
			case "cobblestone":
				return makeCraftStack("cobblestone_slab", 6)
			case "stone":
				return makeCraftStack("stone_slab", 6)
			case "stone_bricks":
				return makeCraftStack("stone_brick_slab", 6)
			case "sandstone":
				return makeCraftStack("sandstone_slab", 6)
			case "wheat":
				return makeCraftStack("bread", 1)
			case "sugar_cane":
				return makeCraftStack("paper", 3)
			}
		}
	}

	// 8. 2x3 recipes (doors, axes, hoes)
	if w == 2 && h == 3 {
		c00, c01 := cell(0, 0), cell(0, 1)
		c10, c11 := cell(1, 0), cell(1, 1)
		c20, c21 := cell(2, 0), cell(2, 1)

		// Doors (all 6 slots equal), Smithing Table, Cartography Table
		if nonEmpty == 6 {
			if c00 == "iron_ingot" && c01 == "iron_ingot" && isPlanksName(c10) && isPlanksName(c11) && isPlanksName(c20) && isPlanksName(c21) {
				return makeCraftStack("smithing_table", 1)
			}
			if c00 == "paper" && c01 == "paper" && isPlanksName(c10) && isPlanksName(c11) && isPlanksName(c20) && isPlanksName(c21) {
				return makeCraftStack("cartography_table", 1)
			}
		}
		if nonEmpty == 6 && c00 == c01 && c00 == c10 && c00 == c11 && c00 == c20 && c00 == c21 {
			if wood := woodTypeFromPlanks(c00); wood != "" {
				return makeCraftStack(wood+"_door", 3)
			}
			if c00 == "iron_ingot" {
				return makeCraftStack("iron_door", 3)
			}
		}

		// Axe
		if c00 != "" && c00 == c01 {
			if pref := toolPrefixForMaterial(c00); pref != "" {
				if (c10 == c00 && c11 == "stick" && c20 == "" && c21 == "stick") ||
					(c11 == c00 && c10 == "stick" && c21 == "" && c20 == "stick") {
					return makeCraftStack(pref+"_axe", 1)
				}
				// Hoe
				if (c10 == "" && c11 == "stick" && c20 == "" && c21 == "stick") ||
					(c11 == "" && c10 == "stick" && c21 == "" && c20 == "stick") {
					return makeCraftStack(pref+"_hoe", 1)
				}
			}
		}
	}

	// 9. 3x2 recipes (trapdoors, fences, fence gates, boats, helmets, boots, buckets, beds, walls)
	if w == 3 && h == 2 {
		r00, r01, r02 := cell(0, 0), cell(0, 1), cell(0, 2)
		r10, r11, r12 := cell(1, 0), cell(1, 1), cell(1, 2)

		// All 6 equal (trapdoor / cobblestone wall)
		if nonEmpty == 6 && r00 == r01 && r00 == r02 && r00 == r10 && r00 == r11 && r00 == r12 {
			if wood := woodTypeFromPlanks(r00); wood != "" {
				return makeCraftStack(wood+"_trapdoor", 2)
			}
			if r00 == "cobblestone" {
				return makeCraftStack("cobblestone_wall", 6)
			}
		}

		// Fence: [plank, stick, plank / plank, stick, plank]
		if r01 == "stick" && r11 == "stick" && r00 == r02 && r00 == r10 && r00 == r12 {
			if wood := woodTypeFromPlanks(r00); wood != "" {
				return makeCraftStack(wood+"_fence", 3)
			}
		}

		// Fence gate: [stick, plank, stick / stick, plank, stick]
		if r00 == "stick" && r02 == "stick" && r10 == "stick" && r12 == "stick" && r01 == r11 {
			if wood := woodTypeFromPlanks(r01); wood != "" {
				return makeCraftStack(wood+"_fence_gate", 1)
			}
		}

		// Boat: [plank, "", plank / plank, plank, plank]
		if r01 == "" && r00 == r02 && r00 == r10 && r00 == r11 && r00 == r12 {
			if wood := woodTypeFromPlanks(r00); wood != "" {
				return makeCraftStack(wood+"_boat", 1)
			}
		}

		// Bucket: [iron_ingot, "", iron_ingot / "", iron_ingot, ""]
		if r00 == "iron_ingot" && r01 == "" && r02 == "iron_ingot" && r10 == "" && r11 == "iron_ingot" && r12 == "" {
			return makeCraftStack("bucket", 1)
		}

		// Bowl: [plank, "", plank / "", plank, ""]
		if isPlanksName(r00) && r01 == "" && isPlanksName(r02) && r10 == "" && isPlanksName(r11) && r12 == "" {
			return makeCraftStack("bowl", 4)
		}

		// Stonecutter: ["", iron_ingot, "" / stone, stone, stone]
		if r00 == "" && r01 == "iron_ingot" && r02 == "" && r10 == "stone" && r11 == "stone" && r12 == "stone" {
			return makeCraftStack("stonecutter", 1)
		}

		// Grindstone: [stick, stone_slab, stick / plank, "", plank]
		if r00 == "stick" && r01 == "stone_slab" && r02 == "stick" && isPlanksName(r10) && r11 == "" && isPlanksName(r12) {
			return makeCraftStack("grindstone", 1)
		}

		// Helmet: [mat, mat, mat / mat, "", mat]
		if r11 == "" && r00 != "" && r00 == r01 && r00 == r02 && r00 == r10 && r00 == r12 {
			if pref := armorPrefixForMaterial(r00); pref != "" {
				return makeCraftStack(pref+"_helmet", 1)
			}
		}

		// Boots: [mat, "", mat / mat, "", mat]
		if r01 == "" && r11 == "" && r00 != "" && r00 == r02 && r00 == r10 && r00 == r12 {
			if pref := armorPrefixForMaterial(r00); pref != "" {
				return makeCraftStack(pref+"_boots", 1)
			}
		}

		// Bed: 3 wool on row 0, 3 planks on row 1
		if strings.HasSuffix(r00, "_wool") && r00 == r01 && r00 == r02 &&
			isPlanksName(r10) && isPlanksName(r11) && isPlanksName(r12) {
			color := strings.TrimSuffix(r00, "_wool")
			return makeCraftStack(color+"_bed", 1)
		}
	}

	// 10. 3x3 recipes (pickaxes, furnace, chest, storage blocks, stairs, ladders, armor, shield, bowls)
	if w == 3 && h == 3 {
		r00, r01, r02 := cell(0, 0), cell(0, 1), cell(0, 2)
		r10, r11, r12 := cell(1, 0), cell(1, 1), cell(1, 2)
		r20, r21, r22 := cell(2, 0), cell(2, 1), cell(2, 2)

		// Pickaxe: [mat, mat, mat / "", stick, "" / "", stick, ""]
		if r10 == "" && r12 == "" && r20 == "" && r22 == "" && r11 == "stick" && r21 == "stick" {
			if (isPlanksName(r00) && isPlanksName(r01) && isPlanksName(r02)) || (r00 != "" && r00 == r01 && r00 == r02) {
				if pref := toolPrefixForMaterial(r00); pref != "" {
					return makeCraftStack(pref+"_pickaxe", 1)
				}
			}
		}

		// 8-item ring around empty center (chest / furnace)
		if nonEmpty == 8 && r11 == "" {
			if isPlanksName(r00) && isPlanksName(r01) && isPlanksName(r02) &&
				isPlanksName(r10) && isPlanksName(r12) &&
				isPlanksName(r20) && isPlanksName(r21) && isPlanksName(r22) {
				return makeCraftStack("chest", 1)
			}
			if (r00 == "cobblestone" || r00 == "blackstone" || r00 == "cobbled_deepslate") &&
				r00 == r01 && r00 == r02 && r00 == r10 && r00 == r12 && r00 == r20 && r00 == r21 && r00 == r22 {
				return makeCraftStack("furnace", 1)
			}
			// Barrel: [plank, slab, plank / plank, "", plank / plank, slab, plank]
			if isPlanksName(r00) && isPlanksName(r02) && isPlanksName(r10) && isPlanksName(r12) && isPlanksName(r20) && isPlanksName(r22) &&
				strings.HasSuffix(r01, "_slab") && strings.HasSuffix(r21, "_slab") {
				return makeCraftStack("barrel", 1)
			}
		}

		// Anvil: [iron_block, iron_block, iron_block / "", iron_ingot, "" / iron_ingot, iron_ingot, iron_ingot]
		if r00 == "iron_block" && r01 == "iron_block" && r02 == "iron_block" &&
			r10 == "" && r11 == "iron_ingot" && r12 == "" &&
			r20 == "iron_ingot" && r21 == "iron_ingot" && r22 == "iron_ingot" {
			return makeCraftStack("anvil", 1)
		}

		// Smoker: 4 logs around furnace
		if r00 == "" && r02 == "" && r20 == "" && r22 == "" && r11 == "furnace" {
			if p0, _ := planksFromLog(r01); p0 != "" {
				if p1, _ := planksFromLog(r10); p1 != "" {
					if p2, _ := planksFromLog(r12); p2 != "" {
						if p3, _ := planksFromLog(r21); p3 != "" {
							return makeCraftStack("smoker", 1)
						}
					}
				}
			}
		}

		// Blast Furnace: 5 iron ingots + furnace + 3 smooth_stone/stone
		if r00 == "iron_ingot" && r01 == "iron_ingot" && r02 == "iron_ingot" &&
			r10 == "iron_ingot" && r11 == "furnace" && r12 == "iron_ingot" &&
			(r20 == "smooth_stone" || r20 == "stone") && r20 == r21 && r20 == r22 {
			return makeCraftStack("blast_furnace", 1)
		}

		// 9 identical items -> storage block
		if nonEmpty == 9 && r00 == r01 && r00 == r02 && r00 == r10 && r00 == r11 && r00 == r12 && r00 == r20 && r00 == r21 && r00 == r22 {
			switch r00 {
			case "iron_ingot":
				return makeCraftStack("iron_block", 1)
			case "gold_ingot":
				return makeCraftStack("gold_block", 1)
			case "diamond":
				return makeCraftStack("diamond_block", 1)
			case "emerald":
				return makeCraftStack("emerald_block", 1)
			case "coal":
				return makeCraftStack("coal_block", 1)
			case "redstone":
				return makeCraftStack("redstone_block", 1)
			case "lapis_lazuli":
				return makeCraftStack("lapis_block", 1)
			case "copper_ingot":
				return makeCraftStack("copper_block", 1)
			case "wheat":
				return makeCraftStack("hay_block", 1)
			case "bone_meal":
				return makeCraftStack("bone_block", 1)
			}
		}

		// Ladder: [stick, "", stick / stick, stick, stick / stick, "", stick]
		if r00 == "stick" && r01 == "" && r02 == "stick" &&
			r10 == "stick" && r11 == "stick" && r12 == "stick" &&
			r20 == "stick" && r21 == "" && r22 == "stick" {
			return makeCraftStack("ladder", 3)
		}

		// Shield: [plank, iron_ingot, plank / plank, plank, plank / "", plank, ""]
		if r01 == "iron_ingot" && r20 == "" && r22 == "" &&
			isPlanksName(r00) && isPlanksName(r02) &&
			isPlanksName(r10) && isPlanksName(r11) && isPlanksName(r12) &&
			isPlanksName(r21) {
			return makeCraftStack("shield", 1)
		}

		// Chestplate: [mat, "", mat / mat, mat, mat / mat, mat, mat]
		if r01 == "" && r00 != "" && r00 == r02 && r00 == r10 && r00 == r11 && r00 == r12 && r00 == r20 && r00 == r21 && r00 == r22 {
			if pref := armorPrefixForMaterial(r00); pref != "" {
				return makeCraftStack(pref+"_chestplate", 1)
			}
		}

		// Leggings: [mat, mat, mat / mat, "", mat / mat, "", mat]
		if r11 == "" && r21 == "" && r00 != "" && r00 == r01 && r00 == r02 && r00 == r10 && r00 == r12 && r00 == r20 && r00 == r22 {
			if pref := armorPrefixForMaterial(r00); pref != "" {
				return makeCraftStack(pref+"_leggings", 1)
			}
		}

		// Stairs: 6 matching blocks in stair shape (left or right)
		if nonEmpty == 6 {
			leftStair := r01 == "" && r02 == "" && r12 == "" && r00 != "" && r00 == r10 && r00 == r11 && r00 == r20 && r00 == r21 && r00 == r22
			rightStair := r00 == "" && r01 == "" && r10 == "" && r02 != "" && r02 == r11 && r02 == r12 && r02 == r20 && r02 == r21 && r02 == r22
			if leftStair || rightStair {
				mat := r20
				if wood := woodTypeFromPlanks(mat); wood != "" {
					return makeCraftStack(wood+"_stairs", 4)
				}
				switch mat {
				case "cobblestone":
					return makeCraftStack("cobblestone_stairs", 4)
				case "stone":
					return makeCraftStack("stone_stairs", 4)
				case "stone_bricks":
					return makeCraftStack("stone_brick_stairs", 4)
				case "sandstone":
					return makeCraftStack("sandstone_stairs", 4)
				}
			}
		}
	}

	return player.ItemStack{}
}

