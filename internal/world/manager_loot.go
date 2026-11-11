package world

import (
	"strings"

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
	m.DropBlockLootWithTool(blockID, "", x, y, z)
}

// DropBlockLootWithTool rolls the vanilla survival loot for the block at (x,y,z)
// when broken with toolName (e.g. "minecraft:shears" or "") and spawns any
// resulting item entities into the drop store.
func (m *Manager) DropBlockLootWithTool(blockID int32, toolName string, x, y, z int) {
	name := StateName(blockID)
	props := StateProperties(blockID)

	m.dropMu.Lock()
	stacks := loot.RollsWithTool(name, toolName, m.dropRNG)
	// Handle block-state-dependent drops that depend on runtime state properties.
	switch name {
	case "minecraft:cobweb":
		if strings.HasSuffix(toolName, "_sword") {
			stacks = []loot.ItemStack{{Item: "minecraft:string", Count: 1}}
		}
	case "minecraft:glow_lichen":
		if len(stacks) > 0 && len(props) > 0 {
			faces := 0
			for _, f := range [6]string{"up", "down", "north", "south", "east", "west"} {
				if props[f] == "true" {
					faces++
				}
			}
			if faces > 0 {
				stacks[0].Count = faces
			}
		}
	case "minecraft:cave_vines", "minecraft:cave_vines_plant":
		if props["berries"] == "true" {
			stacks = append(stacks, loot.ItemStack{Item: "minecraft:glow_berries", Count: 1})
		}
	case "minecraft:sweet_berry_bush":
		switch props["age"] {
		case "2":
			stacks = append(stacks, loot.ItemStack{Item: "minecraft:sweet_berries", Count: 1 + m.dropRNG.Intn(2)})
		case "3":
			stacks = append(stacks, loot.ItemStack{Item: "minecraft:sweet_berries", Count: 2 + m.dropRNG.Intn(2)})
		}
	case "minecraft:wheat":
		if props["age"] == "7" {
			stacks = []loot.ItemStack{
				{Item: "minecraft:wheat", Count: 1},
				{Item: "minecraft:wheat_seeds", Count: 1 + m.dropRNG.Intn(3)},
			}
		}
	case "minecraft:beetroots":
		if props["age"] == "3" {
			stacks = []loot.ItemStack{
				{Item: "minecraft:beetroot", Count: 1},
				{Item: "minecraft:beetroot_seeds", Count: 1 + m.dropRNG.Intn(3)},
			}
		}
	case "minecraft:carrots":
		if props["age"] == "7" {
			stacks = []loot.ItemStack{{Item: "minecraft:carrot", Count: 2 + m.dropRNG.Intn(3)}}
		}
	case "minecraft:potatoes":
		if props["age"] == "7" {
			stacks = []loot.ItemStack{{Item: "minecraft:potato", Count: 2 + m.dropRNG.Intn(3)}}
		}
	case "minecraft:nether_wart":
		if props["age"] == "3" {
			stacks = []loot.ItemStack{{Item: "minecraft:nether_wart", Count: 2 + m.dropRNG.Intn(3)}}
		}
	case "minecraft:cocoa":
		if props["age"] == "2" {
			stacks = []loot.ItemStack{{Item: "minecraft:cocoa_beans", Count: 3}}
		} else {
			stacks = []loot.ItemStack{{Item: "minecraft:cocoa_beans", Count: 1}}
		}
	}
	m.dropMu.Unlock()

	clusterCount := 0
	if len(props) > 0 {
		if props["type"] == "double" {
			clusterCount = 2
		} else {
			for _, key := range []string{"flower_amount", "segment_amount", "pickles", "candles"} {
				if v, ok := props[key]; ok {
					switch v {
					case "1":
						clusterCount = 1
					case "2":
						clusterCount = 2
					case "3":
						clusterCount = 3
					case "4":
						clusterCount = 4
					}
					break
				}
			}
		}
	}
	for _, st := range stacks {
		count := st.Count
		if clusterCount > 0 {
			count = clusterCount
		}
		m.drops.Spawn(st.Item, count, float64(x)+0.5, float64(y)+0.25, float64(z)+0.5)
	}
}

// PlayerDropItem spawns a single item stack at the player as if the player hit
// Q / Ctrl+Q. The throw arc is derived from pitch and yaw so the item visibly
// leaves the player's hand along their look vector instead of popping up.
func (m *Manager) PlayerDropItem(item string, count int, x, y, z, pitch, yaw float64) {
	m.drops.SpawnFromPlayer(item, count, x, y, z, pitch, yaw)
}
