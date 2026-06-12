package world

import (
	"sync"

	lwworld "livingworld/internal/world"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
)

// ridCache memoizes world state ID -> Bedrock runtime ID. Chunk serialization
// resolves the RID for every non-air block, so caching avoids repeated palette
// hash lookups.
var (
	ridCacheMu sync.RWMutex
	ridCache   = map[int32]uint32{}
)

// bedrockPropOverrides supplies the Bedrock block-state properties required for
// blocks that will not resolve from their name alone (Bedrock hashes name+props).
// Most blocks resolve by name; only the property-sensitive ones need an entry.
// Java->Bedrock property fidelity for stateful blocks (stairs, slabs, logs...) is
// a known limitation and can be expanded here as needed.
var bedrockPropOverrides = map[string]map[string]any{
	"minecraft:grass_block": {"minecraft:snowy_bit": false},
	// Worldgen trees: Java's default log state is axis=y; Bedrock's
	// name-only fallback may pick a different pillar_axis.
	"minecraft:oak_log":      {"pillar_axis": "y"},
	"minecraft:birch_log":    {"pillar_axis": "y"},
	"minecraft:spruce_log":   {"pillar_axis": "y"},
	"minecraft:jungle_log":   {"pillar_axis": "y"},
	"minecraft:acacia_log":   {"pillar_axis": "y"},
	"minecraft:cherry_log":   {"pillar_axis": "y"},
	"minecraft:dark_oak_log": {"pillar_axis": "y"},
	// Bedrock leaves ship with "persistent_bit"/"update_bit"=1 by default;
	// without override the renderer treats the leaf as "no log nearby" and
	// hides it. Force bit=0 so freshly-generated trees look like trees.
	"minecraft:oak_leaves":      {"persistent_bit": false, "update_bit": false},
	"minecraft:birch_leaves":    {"persistent_bit": false, "update_bit": false},
	"minecraft:spruce_leaves":   {"persistent_bit": false, "update_bit": false},
	"minecraft:jungle_leaves":   {"persistent_bit": false, "update_bit": false},
	"minecraft:acacia_leaves":   {"persistent_bit": false, "update_bit": false},
	"minecraft:cherry_leaves":   {"persistent_bit": false, "update_bit": false},
	"minecraft:dark_oak_leaves": {"persistent_bit": false, "update_bit": false},
	"minecraft:mangrove_leaves": {"persistent_bit": false, "update_bit": false},
}

// bedrockNameRemaps translates Java block names whose Bedrock identifier
// differs. Without these the RID lookup fails and the block renders as
// whatever runtime ID 0 happens to be.
var bedrockNameRemaps = map[string]string{
	"minecraft:snow":       "minecraft:snow_layer", // Java "snow" is the thin layer
	"minecraft:snow_block": "minecraft:snow",       // Java full block is Bedrock "snow" too (1:1, just force the lookup)
	"minecraft:terracotta": "minecraft:hardened_clay",
	"minecraft:dead_bush":  "minecraft:deadbush",
	"minecraft:sugar_cane": "minecraft:reeds",
}

// LivingWorldBlockIDToBedrockRID maps a canonical world block ID (= Java global
// state ID) to a Bedrock runtime ID via the block's namespaced name.
func LivingWorldBlockIDToBedrockRID(id int32) uint32 {
	ridCacheMu.RLock()
	rid, ok := ridCache[id]
	ridCacheMu.RUnlock()
	if ok {
		return rid
	}

	name := lwworld.StateName(id)
	if remapped, ok := bedrockNameRemaps[name]; ok {
		name = remapped
	}
	if props, ok := bedrockPropOverrides[name]; ok {
		rid = BlockRID(name, props)
	} else {
		rid = BlockRID(name)
	}

	ridCacheMu.Lock()
	ridCache[id] = rid
	ridCacheMu.Unlock()
	return rid
}

// javaNameRemaps is the inverse of bedrockNameRemaps, for blocks coming
// FROM the Bedrock client (block place / break echoes).
var javaNameRemaps = map[string]string{
	"minecraft:snow_layer":    "minecraft:snow",
	"minecraft:snow":          "minecraft:snow_block",
	"minecraft:hardened_clay": "minecraft:terracotta",
	"minecraft:deadbush":      "minecraft:dead_bush",
	"minecraft:reeds":         "minecraft:sugar_cane",
}

// BedrockRIDToLivingWorldBlockID maps a Bedrock runtime ID back to a canonical
// world block ID by resolving its namespaced name in the global palette.
func BedrockRIDToLivingWorldBlockID(rid uint32) int32 {
	name, _, ok := dfchunk.RuntimeIDToState(rid)
	if !ok {
		return lwworld.AirID
	}
	if remapped, ok := javaNameRemaps[name]; ok {
		name = remapped
	}
	return lwworld.StateID(name)
}
