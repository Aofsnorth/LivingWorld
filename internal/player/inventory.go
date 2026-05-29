package player

const (
	InventorySize     = 46
	HotbarSize        = 9
	MainInventorySize = 36
)

type Inventory struct {
	Items    []ItemStack
	HeldSlot int
	WindowID int32
}

type ItemStack struct {
	ID    int32
	Count int8
	Meta  int16
	NBT   []byte
}

func NewInventory() *Inventory {
	return &Inventory{
		Items:    make([]ItemStack, InventorySize),
		HeldSlot: 0,
	}
}

func (inv *Inventory) GetItem(slot int) *ItemStack {
	if slot < 0 || slot >= len(inv.Items) {
		return nil
	}
	return &inv.Items[slot]
}

func (inv *Inventory) SetItem(slot int, item ItemStack) {
	if slot < 0 || slot >= len(inv.Items) {
		return
	}
	inv.Items[slot] = item
}

func (inv *Inventory) Clear() {
	for i := range inv.Items {
		inv.Items[i] = ItemStack{}
	}
}

func (inv *Inventory) GetHeldItem() *ItemStack {
	return inv.GetItem(inv.HeldSlot)
}

func (inv *Inventory) SetHeldSlot(slot int) {
	if slot >= 0 && slot < HotbarSize {
		inv.HeldSlot = slot
	}
}

func (inv *Inventory) AddItem(item ItemStack) bool {
	for i := 0; i < len(inv.Items); i++ {
		if inv.Items[i].ID == item.ID && inv.Items[i].Count < 64 {
			canAdd := int8(64) - inv.Items[i].Count
			if canAdd >= item.Count {
				inv.Items[i].Count += item.Count
				return true
			}
			inv.Items[i].Count = 64
			item.Count -= canAdd
		}
	}
	for i := 0; i < len(inv.Items); i++ {
		if inv.Items[i].ID == 0 {
			inv.Items[i] = item
			return true
		}
	}
	return false
}

func (inv *Inventory) RemoveItem(slot int, count int8) bool {
	if slot < 0 || slot >= len(inv.Items) {
		return false
	}
	if inv.Items[slot].Count < count {
		return false
	}
	inv.Items[slot].Count -= count
	if inv.Items[slot].Count <= 0 {
		inv.Items[slot] = ItemStack{}
	}
	return true
}

type SkinData struct {
	Model         string
	SkinID        string
	URL           string
	Hash          string
	Data          []byte
	AnimationData []byte
	CapeData      []byte
	GeometryName  string
	GeometryData  []byte
}

func NewSkinData(skinID, model string, data []byte) *SkinData {
	return &SkinData{
		SkinID: skinID,
		Model:  model,
		Data:   data,
		Hash:   "",
	}
}

func (s *SkinData) IsSlim() bool {
	return s.Model == "slim" || s.Model == "alex"
}