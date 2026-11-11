package world

import (
	"log"
	"strings"
	"sync"

	"livingworld/internal/blockconv"
	"livingworld/internal/dimension/overworld"
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
	"minecraft:oak_leaves":              {"persistent_bit": false, "update_bit": false},
	"minecraft:birch_leaves":            {"persistent_bit": false, "update_bit": false},
	"minecraft:spruce_leaves":           {"persistent_bit": false, "update_bit": false},
	"minecraft:jungle_leaves":           {"persistent_bit": false, "update_bit": false},
	"minecraft:acacia_leaves":           {"persistent_bit": false, "update_bit": false},
	"minecraft:cherry_leaves":           {"persistent_bit": false, "update_bit": false},
	"minecraft:dark_oak_leaves":         {"persistent_bit": false, "update_bit": false},
	"minecraft:mangrove_leaves":         {"persistent_bit": false, "update_bit": false},
	"minecraft:azalea_leaves":           {"persistent_bit": false, "update_bit": false},
	"minecraft:flowering_azalea_leaves": {"persistent_bit": false, "update_bit": false},
	"minecraft:pale_oak_leaves":         {"persistent_bit": false, "update_bit": false},
	// Bedrock's default minecraft:vine state has vine_direction_bits=0 (no faces
	// enabled), which makes jungle vines completely invisible on Bedrock clients.
	"minecraft:vine": {"vine_direction_bits": int32(15)},
}

// Cross-edition name aliases live in blockconv (single source of truth,
// both directions derived from one table): blockconv.JavaNameToBedrockName
// here, blockconv.BedrockNameToJavaName for the reverse. Verified against
// dragonfly v0.11.5 (protocol 2193): "hardened_clay", "deadbush" and
// "reeds" exist in its registry while "terracotta", "dead_bush" and
// "sugar_cane" do not. Bedrock "minecraft:snow" is the full snow BLOCK
// (rid 8506) and "minecraft:snow_layer" is the thin layer (rid 1179), so
// the two Java names map 1:1 onto those two Bedrock names.

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

	origName := lwworld.StateName(id)
	name := origName
	if remapped, ok := blockconv.JavaNameToBedrockName[name]; ok {
		name = remapped
	}
	// If this state ID was recorded with explicit properties (or has non-default
	// canonical properties such as directional vines, repeaters, lamps, doors,
	// pistons, levers, etc.) and does not require a fixed worldgen override,
	// resolve via the Geyser 26.3 state table first so exact block properties
	// survive onto Bedrock.
	if _, hasOverride := bedrockPropOverrides[name]; !hasOverride || name == "minecraft:vine" {
		if mappedRID, found := overworld.LookupBedrockRID(id, lwworld.CanonicalState(id)); found {
			ridCacheMu.Lock()
			ridCache[id] = mappedRID
			ridCacheMu.Unlock()
			return mappedRID, true
		}
	}
	rid = blockRID(name, bedrockPropOverrides[name])
	if !blockRIDFound(name) {
		if mappedRID, found := overworld.LookupBedrockRID(id, origName); found {
			ridCacheMu.Lock()
			ridCache[id] = mappedRID
			ridCacheMu.Unlock()
			return mappedRID, true
		}
		if mappedRID, found := overworld.LookupBedrockRID(0, name); found {
			ridCacheMu.Lock()
			ridCache[id] = mappedRID
			ridCacheMu.Unlock()
			return mappedRID, true
		}
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

// BedrockRIDToLivingWorldBlockID maps a Bedrock runtime ID back to a
// canonical world block ID. Block-state properties are translated
// (blockconv.Translate) so stateful blocks — log axes, stair facings,
// snow-layer thickness, fluid levels — keep their state instead of
// collapsing to the Java default; untranslatable states fall back to
// the default state of the translated name.
func BedrockRIDToLivingWorldBlockID(rid uint32) int32 {
	name, props, ok := dfworld.DefaultBlockRegistry.RuntimeIDToState(rid)
	if !ok {
		return lwworld.AirID
	}
	javaName, javaProps, translated := blockconv.Translate(name, props)
	if !translated {
		return lwworld.StateID(javaName)
	}
	return lwworld.StateIDWithProperties(javaName, javaProps)
}

var (
	heightMapBlockerOnce sync.Once
	heightMapBlockerLUT  []bool
)

// nonBlockingUnknownBedrockBlocks lists Bedrock block names that are not
// registered as concrete dragonfly block.Block implementations (so dragonfly
// falls back to unknownBlock with LightDiffusionLevel=15), yet in vanilla
// Bedrock they do NOT obstruct or diffuse sky light from above (filtering=0).
var nonBlockingUnknownBedrockBlocks = map[string]struct{}{
	"minecraft:air":                          {},
	"minecraft:structure_void":               {},
	"minecraft:light_block_0":                {},
	"minecraft:light_block_1":                {},
	"minecraft:light_block_2":                {},
	"minecraft:light_block_3":                {},
	"minecraft:light_block_4":                {},
	"minecraft:light_block_5":                {},
	"minecraft:light_block_6":                {},
	"minecraft:light_block_7":                {},
	"minecraft:light_block_8":                {},
	"minecraft:light_block_9":                {},
	"minecraft:light_block_10":               {},
	"minecraft:light_block_11":               {},
	"minecraft:light_block_12":               {},
	"minecraft:light_block_13":               {},
	"minecraft:light_block_14":               {},
	"minecraft:light_block_15":               {},
	"minecraft:snow_layer":                   {},
	"minecraft:moss_carpet":                  {},
	"minecraft:pale_moss_carpet":             {},
	"minecraft:leaf_litter":                  {},
	"minecraft:pink_petals":                  {},
	"minecraft:wildflowers":                  {},
	"minecraft:vine":                         {},
	"minecraft:glow_lichen":                  {},
	"minecraft:sculk_vein":                   {},
	"minecraft:waterlily":                    {},
	"minecraft:SugarCane":                    {},
	"minecraft:reeds":                        {},
	"minecraft:tall_dry_grass":               {},
	"minecraft:short_dry_grass":              {},
	"minecraft:bush":                         {},
	"minecraft:firefly_bush":                 {},
	"minecraft:cactus_flower":                {},
	"minecraft:Pale_hanging_moss":            {},
	"minecraft:pale_hanging_moss":            {},
	"minecraft:hanging_roots":                {},
	"minecraft:spore_blossom":                {},
	"minecraft:cave_vines":                   {},
	"minecraft:cave_vines_body_with_berries": {},
	"minecraft:cave_vines_head_with_berries": {},
	"minecraft:twisting_vines":               {},
	"minecraft:weeping_vines":                {},
	"minecraft:small_dripleaf_block":         {},
	"minecraft:big_dripleaf":                 {},
}

func initHeightMapBlockerLUT() {
	heightMapBlockerOnce.Do(func() {
		// Scan all valid runtime IDs in DefaultBlockRegistry.
		// Bedrock block state tables have ~16k states.
		const maxScan = 65536
		var maxRID uint32
		for rid := uint32(0); rid < maxScan; rid++ {
			if _, _, ok := dfworld.DefaultBlockRegistry.RuntimeIDToState(rid); ok {
				maxRID = rid
			} else if rid > maxRID+256 {
				break
			}
		}
		lut := make([]bool, maxRID+1)
		for rid := uint32(0); rid <= maxRID; rid++ {
			name, _, ok := dfworld.DefaultBlockRegistry.RuntimeIDToState(rid)
			if !ok {
				continue
			}
			if _, nonBlocking := nonBlockingUnknownBedrockBlocks[name]; nonBlocking {
				continue
			}
			// Any block with light filtering > 0 (both opaque blocks with
			// filtering=15 AND light-diffusing blocks like leaves, water,
			// cobweb, tinted glass, ice with filtering >= 1) counts as a
			// heightmap light blocker for Bedrock's SubChunk heightmap.
			if dfworld.DefaultBlockRegistry.FilteringBlock(rid) > 0 {
				lut[rid] = true
			}
		}
		heightMapBlockerLUT = lut
	})
}

// AirRID returns the Bedrock runtime ID for minecraft:air.
func AirRID() uint32 {
	if rid, ok := LivingWorldBlockIDToBedrockRID(lwworld.AirID); ok {
		return rid
	}
	rid, _ := dfworld.DefaultBlockRegistry.StateToRuntimeID("minecraft:air", nil)
	return rid
}

// IsHeightMapBlockerRID reports whether the given Bedrock runtime ID diffuses
// or obstructs sky light for Bedrock sub-chunk heightmap calculation.
func IsHeightMapBlockerRID(rid uint32) bool {
	initHeightMapBlockerLUT()
	if int(rid) < len(heightMapBlockerLUT) {
		return heightMapBlockerLUT[rid]
	}
	return dfworld.DefaultBlockRegistry.FilteringBlock(rid) > 0
}

var (
	stillWaterRIDOnce sync.Once
	stillWaterRIDVal  uint32
)

// BedrockStillWaterRID returns the Bedrock runtime ID for still water (liquid_depth=0)
// used on sub-chunk storage layer 1 for waterlogged underwater blocks (seagrass, kelp, etc.).
func BedrockStillWaterRID() uint32 {
	stillWaterRIDOnce.Do(func() {
		if rid, ok := LivingWorldBlockIDToBedrockRID(lwworld.StateID("minecraft:water")); ok {
			stillWaterRIDVal = rid
		}
	})
	return stillWaterRIDVal
}

// IsWaterloggedLivingWorldBlock reports whether a canonical LivingWorld block state ID
// requires still water in Bedrock sub-chunk layer 1 so Bedrock does not render air pockets
// around underwater flora (seagrass, tall_seagrass, kelp, kelp_plant, bubble_column) or
// explicitly waterlogged blocks.
func IsWaterloggedLivingWorldBlock(id int32) bool {
	if id <= 0 {
		return false
	}
	switch lwworld.StateName(id) {
	case "minecraft:seagrass",
		"minecraft:tall_seagrass",
		"minecraft:kelp",
		"minecraft:kelp_plant",
		"minecraft:bubble_column":
		return true
	}
	return strings.Contains(lwworld.CanonicalState(id), "waterlogged=true")
}
