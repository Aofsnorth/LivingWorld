package inventory

import (
	_ "embed"

	"github.com/sandertv/gophertunnel/minecraft/nbt"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// vanilla_items.nbt mirrors the file shipped by df-mc/dragonfly@v0.10.13 at
// server/world/vanilla_items.nbt. When the dragonfly dependency is bumped,
// re-copy this asset from the matching module-cache path so the registry
// stays in sync with the protocol that gophertunnel/dragonfly target.

//go:embed data/vanilla_items.nbt
var vanillaItemsData []byte

var vanillaItems = map[string]struct {
	RuntimeID      int32          `nbt:"runtime_id"`
	ComponentBased bool           `nbt:"component_based"`
	Version        int32          `nbt:"version"`
	Data           map[string]any `nbt:"data,omitempty"`
}{}

func init() {
	if err := nbt.Unmarshal(vanillaItemsData, &vanillaItems); err != nil {
		panic("bedrock: failed to load vanilla item registry: " + err.Error())
	}
}

// VanillaItemEntries returns the complete vanilla item registry as
// []protocol.ItemEntry, ready to assign to minecraft.GameData.Items.
// Mirrors dragonfly's private Server.itemEntries() (server/server.go:627-641).
func VanillaItemEntries() []protocol.ItemEntry {
	entries := make([]protocol.ItemEntry, 0, len(vanillaItems))
	for name, e := range vanillaItems {
		entries = append(entries, protocol.ItemEntry{
			Name:           name,
			RuntimeID:      int16(e.RuntimeID),
			ComponentBased: e.ComponentBased,
			Version:        e.Version,
			Data:           e.Data,
			// No hardcoding, to make it easier to maintain in the future
		})
	}
	return entries
}
