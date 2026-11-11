package loot

import (
	"math/rand"
	"testing"
)

func TestSilkTouchAndShearsGatedBlocksDoNotDropBareHanded(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	noBareHandDrop := []string{
		"minecraft:vine",
		"minecraft:glass",
		"minecraft:black_stained_glass",
		"minecraft:glass_pane",
		"minecraft:ice",
		"minecraft:packed_ice",
		"minecraft:blue_ice",
		"minecraft:spawner",
		"minecraft:trial_spawner",
		"minecraft:vault",
		"minecraft:budding_amethyst",
		"minecraft:sculk",
		"minecraft:sculk_vein",
		"minecraft:sculk_catalyst",
		"minecraft:sculk_sensor",
		"minecraft:sculk_shrieker",
		"minecraft:calibrated_sculk_sensor",
		"minecraft:infested_stone",
		"minecraft:infested_cobblestone",
		"minecraft:bee_nest",
		"minecraft:glow_lichen",
		"minecraft:seagrass",
		"minecraft:tall_seagrass",
		"minecraft:bush",
		"minecraft:nether_sprouts",
		"minecraft:hanging_roots",
		"minecraft:small_dripleaf",
		"minecraft:pale_hanging_moss",
		"minecraft:cobweb",
		"minecraft:snow",
		"minecraft:turtle_egg",
		"minecraft:frogspawn",
		"minecraft:tube_coral",
	}

	for _, name := range noBareHandDrop {
		if HasDrops(name) {
			t.Errorf("HasDrops(%q) = true, want false (requires Silk Touch or Shears in vanilla 26.3)", name)
		}
		if got := Rolls(name, rng); len(got) != 0 {
			t.Errorf("Rolls(%q) = %+v, want nil/empty bare-handed", name, got)
		}
	}
}

func TestShearsDrops(t *testing.T) {
	rng := rand.New(rand.NewSource(99))

	shearsCases := map[string]string{
		"minecraft:vine":              "minecraft:vine",
		"minecraft:oak_leaves":        "minecraft:oak_leaves",
		"minecraft:cherry_leaves":     "minecraft:cherry_leaves",
		"minecraft:short_grass":       "minecraft:short_grass",
		"minecraft:fern":              "minecraft:fern",
		"minecraft:seagrass":          "minecraft:seagrass",
		"minecraft:glow_lichen":       "minecraft:glow_lichen",
		"minecraft:dead_bush":         "minecraft:dead_bush",
		"minecraft:cobweb":            "minecraft:cobweb",
		"minecraft:hanging_roots":     "minecraft:hanging_roots",
		"minecraft:nether_sprouts":    "minecraft:nether_sprouts",
		"minecraft:pale_hanging_moss": "minecraft:pale_hanging_moss",
	}

	for blockName, wantItem := range shearsCases {
		if !HasDropsWithTool(blockName, "minecraft:shears") {
			t.Errorf("HasDropsWithTool(%q, shears) = false, want true", blockName)
		}
		got := RollsWithTool(blockName, "minecraft:shears", rng)
		if len(got) != 1 || got[0].Item != wantItem || got[0].Count != 1 {
			t.Errorf("RollsWithTool(%q, shears) = %+v, want [{%s 1}]", blockName, got, wantItem)
		}
	}
}

func TestNaturalTransformDrops(t *testing.T) {
	rng := rand.New(rand.NewSource(7))

	cases := []struct {
		block     string
		wantItem  string
		wantCount int
	}{
		{"minecraft:stone", "minecraft:cobblestone", 1},
		{"minecraft:deepslate", "minecraft:cobbled_deepslate", 1},
		{"minecraft:grass_block", "minecraft:dirt", 1},
		{"minecraft:podzol", "minecraft:dirt", 1},
		{"minecraft:mycelium", "minecraft:dirt", 1},
		{"minecraft:coal_ore", "minecraft:coal", 1},
		{"minecraft:iron_ore", "minecraft:raw_iron", 1},
		{"minecraft:gold_ore", "minecraft:raw_gold", 1},
		{"minecraft:diamond_ore", "minecraft:diamond", 1},
		{"minecraft:bookshelf", "minecraft:book", 3},
		{"minecraft:ender_chest", "minecraft:obsidian", 8},
		{"minecraft:campfire", "minecraft:charcoal", 2},
		{"minecraft:tube_coral_block", "minecraft:dead_tube_coral_block", 1},
		{"minecraft:oak_door", "minecraft:oak_door", 1},
		{"minecraft:red_bed", "minecraft:red_bed", 1},
	}

	for _, tc := range cases {
		got := Rolls(tc.block, rng)
		if len(got) != 1 || got[0].Item != tc.wantItem || got[0].Count != tc.wantCount {
			t.Errorf("Rolls(%q) = %+v, want [{%s %d}]", tc.block, got, tc.wantItem, tc.wantCount)
		}
	}
}
