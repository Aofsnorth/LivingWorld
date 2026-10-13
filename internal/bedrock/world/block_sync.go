package world

import (
	"log"
	"sync"

	lwworld "livingworld/internal/world"

	dfworld "github.com/df-mc/dragonfly/server/world"
)

// ridCache memoizes world state ID -> Bedrock runtime ID. Chunk serialization
// resolves the RID for every non-air block, so caching avoids repeated palette
// hash lookups. Only SUCCESSFUL resolutions are cached: a failed lookup must
// stay uncached so a later registry change (or a fixed remap) can resolve it.
//
// Note that runtime ID 0 is a VALID Bedrock block: dragonfly sorts the registry
// by FNV-64 of the block name, and the lowest hash currently belongs to
// minecraft:cyan_terracotta (verified against dragonfly v0.11.5). rid==0
// therefore never means air or failure — use the boolean from
// LivingWorldBlockIDToBedrockRID instead.
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
// differs. Without these the RID lookup fails and the block cannot be sent.
//
// Verified against dragonfly v0.11.5 (protocol 2193): "hardened_clay",
// "deadbush" and "reeds" exist in its registry while "terracotta",
// "dead_bush" and "sugar_cane" do not. Bedrock "minecraft:snow" is the full
// snow BLOCK (rid 8506) and "minecraft:snow_layer" is the thin layer
// (rid 1179), so the two Java names map 1:1 onto those two Bedrock names.
var bedrockNameRemaps = map[string]string{
	"minecraft:snow":       "minecraft:snow_layer", // Java "snow" is the thin layer
	"minecraft:snow_block": "minecraft:snow",       // Java full block is Bedrock "snow"
	"minecraft:terracotta": "minecraft:hardened_clay",
	"minecraft:dead_bush":  "minecraft:deadbush",
	"minecraft:sugar_cane": "minecraft:reeds",
}

// LivingWorldBlockIDToBedrockRID maps a canonical world block ID (= Java global
// state ID) to a Bedrock runtime ID via the block's namespaced name. The
// second return reports whether the resolution succeeded; a false result means
// the id is not representable in the current Bedrock palette (rid is then the
// registry's fallback and MUST NOT be trusted, and the result is not cached).
func LivingWorldBlockIDToBedrockRID(id int32) (uint32, bool) {
	ridCacheMu.RLock()
	rid, ok := ridCache[id]
	ridCacheMu.RUnlock()
	if ok {
		return rid, true
	}

	name := lwworld.StateName(id)
	if remapped, ok := bedrockNameRemaps[name]; ok {
		name = remapped
	}
	rid = blockRID(name, bedrockPropOverrides[name])
	if !blockRIDFound(name) {
		// Log the exact failing name (with its canonical source id) so palette
		// drift between the Java palette and dragonfly's registry is actionable
		// instead of silently rendering as whatever runtime ID 0 holds.
		log.Printf("[Bedrock] no Bedrock runtime ID for canonical id %d (%s)", id, name)
		return rid, false
	}

	ridCacheMu.Lock()
	ridCache[id] = rid
	ridCacheMu.Unlock()
	return rid, true
}

// InvalidateRIDCache drops every memoized resolution. Call after re-finalizing
// or swapping the underlying Bedrock block registry so stale mappings cannot
// survive a palette change within the process lifetime.
func InvalidateRIDCache() {
	ridCacheMu.Lock()
	ridCache = map[int32]uint32{}
	ridCacheMu.Unlock()
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
	name, _, ok := dfworld.DefaultBlockRegistry.RuntimeIDToState(rid)
	if !ok {
		return lwworld.AirID
	}
	if remapped, ok := javaNameRemaps[name]; ok {
		name = remapped
	}
	return lwworld.StateID(name)
}
