package player

// Vanilla armor slot indices in the player inventory
// (the bottom 4 slots of the 46-slot array, before
// offhand at slot 45). Mirrors the Mojang client
// "Inventory" widget slot layout.
const (
	SlotBoots      = 36
	SlotLeggings   = 37
	SlotChestplate = 38
	SlotHelmet     = 39
	SlotOffhand    = 45
)

// Vanilla gold armor item ids. Piglins treat any of these
// equipped as "the player is gold-armored" → neutral
// behaviour. Item ids are the canonical Java 1.21 set;
// matching the registry in third_party/go-mc/data/item/item.go.
const (
	ItemGoldHelmet     int32 = 770
	ItemGoldChestplate int32 = 771
	ItemGoldLeggings   int32 = 772
	ItemGoldBoots      int32 = 773
)

// WearingGold reports whether the player has at least one
// piece of gold armor equipped. Piglins treat such players
// as neutral. v1: any one piece is enough (vanilla rule —
// at least one piece is the threshold; multiple pieces
// doesn't change the AI behaviour).
func (p *Player) WearingGold() bool {
	if p == nil || p.Inventory == nil {
		return false
	}
	goldIDs := map[int32]bool{
		ItemGoldHelmet:     true,
		ItemGoldChestplate: true,
		ItemGoldLeggings:   true,
		ItemGoldBoots:      true,
	}
	for _, slot := range []int{SlotBoots, SlotLeggings, SlotChestplate, SlotHelmet} {
		it := p.Inventory.GetItem(slot)
		if it != nil && goldIDs[it.ID] {
			return true
		}
	}
	return false
}
