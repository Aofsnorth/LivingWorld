package world

import (
	"livingworld/internal/loot"
)

// OnItemPickup registers a callback invoked when a player picks up an item.
// Used by the Bedrock server to send pickup animation + inventory sync.
func (m *Manager) OnItemPickup(fn func(playerUUID [16]byte, dropEntityID int64, playerEntityID uint64)) {
	m.pickupMu.Lock()
	m.pickupCallback = fn
	m.pickupMu.Unlock()
}

// NotifyItemPickup calls the registered pickup callback if one exists.
func (m *Manager) NotifyItemPickup(playerUUID [16]byte, dropEntityID int64, playerEntityID uint64) {
	m.pickupMu.RLock()
	cb := m.pickupCallback
	m.pickupMu.RUnlock()
	if cb != nil {
		cb(playerUUID, dropEntityID, playerEntityID)
	}
}

// DropBlockLoot rolls the vanilla bare-hand loot for the block that was at
// (x,y,z) and spawns the resulting item entities into the drop store, centred on
// the block. blockID is the canonical world block id that was broken. Call this
// BEFORE replacing the block with air.
func (m *Manager) DropBlockLoot(blockID int32, x, y, z int) {
	name := StateName(blockID)
	m.dropMu.Lock()
	stacks := loot.Rolls(name, m.dropRNG)
	m.dropMu.Unlock()
	for _, st := range stacks {
		m.drops.Spawn(st.Item, st.Count, float64(x)+0.5, float64(y)+0.25, float64(z)+0.5)
	}
}

// PlayerDropItem spawns a single item stack at the player as if the player hit
// Q / Ctrl+Q. The throw arc is derived from yaw so the item visibly leaves the
// player's hand instead of just popping up at their feet.
func (m *Manager) PlayerDropItem(item string, count int, x, y, z, yaw float64) {
	m.drops.SpawnFromPlayer(item, count, x, y, z, yaw)
}
