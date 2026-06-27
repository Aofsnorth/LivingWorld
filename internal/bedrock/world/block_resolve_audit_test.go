package world

import (
	"fmt"
	"testing"

	lwworld "livingworld/internal/world"
)

// TestAuditOverworldBlockResolution resolves every block the overworld
// generator places to a Bedrock runtime ID and reports any that fail (return
// 0 = air). A solid block that fails to resolve renders as air on Bedrock,
// which makes the client's locally-computed light leak through and is a
// likely root cause of "weird lighting / generation" reports.
func TestAuditOverworldBlockResolution(t *testing.T) {
	names := []string{
		"minecraft:bedrock", "minecraft:stone", "minecraft:deepslate",
		"minecraft:dirt", "minecraft:grass_block", "minecraft:podzol", "minecraft:mycelium",
		"minecraft:sand", "minecraft:sandstone", "minecraft:red_sand", "minecraft:gravel",
		"minecraft:water", "minecraft:lava", "minecraft:ice", "minecraft:snow_block", "minecraft:snow",
		"minecraft:mud", "minecraft:terracotta", "minecraft:orange_terracotta",
		"minecraft:yellow_terracotta", "minecraft:white_terracotta", "minecraft:brown_terracotta",
		"minecraft:granite", "minecraft:diorite", "minecraft:andesite", "minecraft:tuff",
		"minecraft:oak_log", "minecraft:oak_leaves", "minecraft:birch_log", "minecraft:birch_leaves",
		"minecraft:spruce_log", "minecraft:spruce_leaves", "minecraft:jungle_log", "minecraft:jungle_leaves",
		"minecraft:acacia_log", "minecraft:acacia_leaves", "minecraft:cherry_log", "minecraft:cherry_leaves",
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
		"minecraft:dark_oak_log", "minecraft:dark_oak_leaves",
	}
	var failed []string
	for _, n := range names {
		sid := lwworld.StateID(n)
		if sid == 0 && n != "minecraft:air" {
			failed = append(failed, fmt.Sprintf("%s: not in Java palette", n))
			continue
		}
		rid := LivingWorldBlockIDToBedrockRID(sid)
		if rid == 0 {
			failed = append(failed, fmt.Sprintf("%s (state=%d): did not resolve to Bedrock RID", n, sid))
		}
	}
	if len(failed) > 0 {
		t.Fatalf("blocks that failed to resolve to Bedrock runtime IDs (render as air → light leaks):\n%v", failed)
	}
}
