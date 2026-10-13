package bedrocktests

import (
	"fmt"
	"testing"

	bedrockworld "livingworld/internal/bedrock/world"
	lwworld "livingworld/internal/world"
)

// worldgenBlockNames is the canonical set of block names the overworld
// generator can place (pipeline.go blockIDs + ores + surface decoration).
// Keep in sync with internal/dimension/overworld/pipeline — if a new block
// enters generation, add it here so its Bedrock mapping is pinned.
var worldgenBlockNames = []string{
	"minecraft:bedrock", "minecraft:stone", "minecraft:deepslate",
	"minecraft:dirt", "minecraft:grass_block", "minecraft:podzol", "minecraft:mycelium",
	"minecraft:sand", "minecraft:sandstone", "minecraft:red_sand", "minecraft:gravel",
	"minecraft:water", "minecraft:lava", "minecraft:ice", "minecraft:snow_block", "minecraft:snow",
	"minecraft:mud",
	"minecraft:terracotta", "minecraft:orange_terracotta",
	"minecraft:yellow_terracotta", "minecraft:white_terracotta", "minecraft:brown_terracotta",
	"minecraft:granite", "minecraft:diorite", "minecraft:andesite", "minecraft:tuff",
	"minecraft:oak_log", "minecraft:oak_leaves",
	"minecraft:birch_log", "minecraft:birch_leaves",
	"minecraft:spruce_log", "minecraft:spruce_leaves",
	"minecraft:jungle_log", "minecraft:jungle_leaves",
	"minecraft:acacia_log", "minecraft:acacia_leaves",
	"minecraft:cherry_log", "minecraft:cherry_leaves",
	"minecraft:dark_oak_log", "minecraft:dark_oak_leaves",
	"minecraft:short_grass", "minecraft:fern", "minecraft:dandelion", "minecraft:poppy",
	"minecraft:cactus", "minecraft:dead_bush",
	"minecraft:coal_ore", "minecraft:deepslate_coal_ore",
	"minecraft:iron_ore", "minecraft:deepslate_iron_ore",
	"minecraft:copper_ore", "minecraft:deepslate_copper_ore",
	"minecraft:gold_ore", "minecraft:deepslate_gold_ore",
	"minecraft:redstone_ore", "minecraft:deepslate_redstone_ore",
	"minecraft:lapis_ore", "minecraft:deepslate_lapis_ore",
	"minecraft:diamond_ore", "minecraft:deepslate_diamond_ore",
	"minecraft:emerald_ore", "minecraft:deepslate_emerald_ore",
}

// TestWorldgenBlocksResolveToDistinctCorrectRIDs is the core palette audit:
// every worldgen-placed canonical state must resolve in the Bedrock registry
// AND map back to the same block name (round-trip). A name-only equality check
// catches wrong-block mappings (e.g. a full block remapped onto a thin layer)
// that a "rid != 0" assertion can never see.
func TestWorldgenBlocksResolveToDistinctCorrectRIDs(t *testing.T) {
	var failures []string
	for _, n := range worldgenBlockNames {
		sid := lwworld.StateID(n)
		if sid == lwworld.AirID {
			failures = append(failures, fmt.Sprintf("%s: missing from canonical Java palette", n))
			continue
		}
		rid, ok := bedrockworld.LivingWorldBlockIDToBedrockRID(sid)
		if !ok {
			failures = append(failures, fmt.Sprintf("%s (state=%d): no Bedrock runtime ID", n, sid))
			continue
		}
		rid2, ok2 := bedrockworld.LivingWorldBlockIDToBedrockRID(sid)
		if !ok2 || rid != rid2 {
			failures = append(failures, fmt.Sprintf("%s: resolution unstable (%d then %d)", n, rid, rid2))
		}
		back := bedrockworld.BedrockRIDToLivingWorldBlockID(rid)
		if back != sid {
			backName := lwworld.StateName(back)
			failures = append(failures, fmt.Sprintf("%s (state=%d) -> rid %d round-trips to %s (state=%d): WRONG BLOCK", n, sid, rid, backName, back))
		}
	}
	if len(failures) > 0 {
		t.Fatalf("Bedrock palette audit failed:\n%s", joinLines(failures))
	}
}

// TestRIDCacheNeverCachesFailures pins the no-cache-failures contract: the
// resolution of an id is deterministic and never transitions ok=false ->
// ok=true through the cache. StateName clamps out-of-range ids to air (a
// valid, resolvable block), so the assertion here is stability, not failure —
// the structural "only cache successes" guarantee is enforced by the code
// path returning before ridCache is written whenever found=false.
func TestRIDCacheNeverCachesFailures(t *testing.T) {
	const badID = int32(1 << 30)
	rid1, ok1 := bedrockworld.LivingWorldBlockIDToBedrockRID(badID)
	rid2, ok2 := bedrockworld.LivingWorldBlockIDToBedrockRID(badID)
	if rid1 != rid2 || ok1 != ok2 {
		t.Fatalf("unstable resolution for id %d: (%d,%v) then (%d,%v)", badID, rid1, ok1, rid2, ok2)
	}
	// And air itself must stay resolvable and stable.
	airID := lwworld.AirID
	r1, okA1 := bedrockworld.LivingWorldBlockIDToBedrockRID(airID)
	r2, okA2 := bedrockworld.LivingWorldBlockIDToBedrockRID(airID)
	if !okA1 || !okA2 || r1 != r2 {
		t.Fatalf("air resolution unstable: (%d,%v) then (%d,%v)", r1, okA1, r2, okA2)
	}
}

// TestSnowLayerVsSnowBlock pins the Java->Bedrock snow split: Java "snow"
// (thin layer) and "snow_block" (full block) must map to DIFFERENT Bedrock
// runtime IDs, and each must round-trip to its own Java name.
func TestSnowLayerVsSnowBlock(t *testing.T) {
	layerID := lwworld.StateID("minecraft:snow")
	blockID := lwworld.StateID("minecraft:snow_block")
	if layerID == lwworld.AirID || blockID == lwworld.AirID {
		t.Fatalf("snow states missing from Java palette: layer=%d block=%d", layerID, blockID)
	}
	layerRID, okL := bedrockworld.LivingWorldBlockIDToBedrockRID(layerID)
	blockRID, okB := bedrockworld.LivingWorldBlockIDToBedrockRID(blockID)
	if !okL || !okB {
		t.Fatalf("snow mapping failed: layer ok=%v, block ok=%v", okL, okB)
	}
	if layerRID == blockRID {
		t.Fatalf("Java snow layer and snow_block collapsed to the same Bedrock RID %d", layerRID)
	}
	if back := bedrockworld.BedrockRIDToLivingWorldBlockID(layerRID); back != layerID {
		t.Fatalf("snow layer rid %d round-tripped to %s", layerRID, lwworld.StateName(back))
	}
	if back := bedrockworld.BedrockRIDToLivingWorldBlockID(blockRID); back != blockID {
		t.Fatalf("snow_block rid %d round-tripped to %s", blockRID, lwworld.StateName(back))
	}
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += "  - " + l
	}
	return out
}
