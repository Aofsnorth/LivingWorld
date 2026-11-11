package world

import (
	"strings"

	"github.com/Tnze/go-mc/level/block"
)

// BlockLightProps holds the light-related properties of a block type.
type BlockLightProps struct {
	Emission uint8 // 0-15, light emitted by this block
	Opacity  uint8 // 0-15, how much light this block absorbs
}

// blockLightProps is a per-state-ID lookup table built at init time.
// Index by canonical state ID (Java global state ID).
var blockLightProps []BlockLightProps

func init() {
	buildLightPropsTable()
}

// buildLightPropsTable populates the blockLightProps array by scanning all
// known block states and classifying them by name.
func buildLightPropsTable() {
	count := len(block.StateList)
	blockLightProps = make([]BlockLightProps, count)

	for i, state := range block.StateList {
		name := state.ID()
		blockLightProps[i] = classifyBlock(name)
	}
}

// classifyBlock returns the light properties for a block based on its namespaced name.
// Uses string matching on the block name to determine emission and opacity.
func classifyBlock(name string) BlockLightProps {
	// Strip minecraft: prefix for cleaner matching
	base := strings.TrimPrefix(name, "minecraft:")

	// Default: opaque, non-emitting
	props := BlockLightProps{Emission: 0, Opacity: 15}

	// Check for light-emitting blocks
	switch {
	case base == "torch" || base == "wall_torch":
		props.Emission = 14
	case base == "glowstone":
		props.Emission = 15
	case base == "sea_lantern":
		props.Emission = 15
	case base == "lantern":
		props.Emission = 15
	case base == "soul_lantern":
		props.Emission = 10
	case base == "soul_torch" || base == "soul_wall_torch":
		props.Emission = 10
	case base == "jack_o_lantern":
		props.Emission = 15
	case base == "beacon":
		props.Emission = 15
	case base == "end_rod":
		props.Emission = 14
	case base == "redstone_lamp" && strings.Contains(name, "lit=true"):
		props.Emission = 15
	case base == "shroomlight":
		props.Emission = 15
	case base == "amethyst_cluster":
		props.Emission = 5
	case base == "large_amethyst_bud":
		props.Emission = 4
	case base == "medium_amethyst_bud":
		props.Emission = 2
	case base == "small_amethyst_bud":
		props.Emission = 1
	case base == "crying_obsidian":
		props.Emission = 10
	case base == "respawn_anchor" && strings.Contains(name, "charges=4"):
		props.Emission = 15
	case base == "lava" || base == "flowing_lava":
		props.Emission = 15
	case base == "fire":
		props.Emission = 15
	case base == "soul_fire":
		props.Emission = 10
	case base == "magma_block":
		props.Emission = 3
	case base == "brewing_stand":
		props.Emission = 1
	case base == "brown_mushroom":
		props.Emission = 1
	case base == "ender_chest":
		props.Emission = 7
	case base == "end_portal_frame" && strings.Contains(name, "eye=true"):
		props.Emission = 1
	case base == "dragon_egg":
		props.Emission = 1
	case strings.HasPrefix(base, "redstone_ore") && strings.Contains(name, "lit=true"):
		props.Emission = 9
	case strings.HasPrefix(base, "deepslate_redstone_ore") && strings.Contains(name, "lit=true"):
		props.Emission = 9
	case base == "furnace" && strings.Contains(name, "lit=true"):
		props.Emission = 13
	case base == "blast_furnace" && strings.Contains(name, "lit=true"):
		props.Emission = 13
	case base == "smoker" && strings.Contains(name, "lit=true"):
		props.Emission = 13
	case base == "campfire" && strings.Contains(name, "lit=true"):
		props.Emission = 15
	case base == "soul_campfire" && strings.Contains(name, "lit=true"):
		props.Emission = 10
	}

	// Check for transparent/semi-transparent blocks (override opacity)
	switch {
	case base == "air" || base == "cave_air" || base == "void_air":
		props.Opacity = 0
	case base == "glass" || base == "glass_pane":
		props.Opacity = 1
	case strings.HasSuffix(base, "_stained_glass") || strings.HasSuffix(base, "_stained_glass_pane"):
		props.Opacity = 1
	case base == "water" || base == "flowing_water":
		props.Opacity = 2
	case base == "ice":
		props.Opacity = 1
	case strings.HasSuffix(base, "_leaves"):
		props.Opacity = 1
	case base == "cobweb":
		props.Opacity = 1
	case base == "slime_block" || base == "honey_block":
		props.Opacity = 1
	case base == "nether_portal" || base == "end_portal" || base == "end_gateway":
		props.Opacity = 0
	case base == "fire" || base == "soul_fire":
		props.Opacity = 0
	case base == "lava" || base == "flowing_lava":
		props.Opacity = 0
	case strings.HasSuffix(base, "_carpet"):
		props.Opacity = 0
	case strings.HasSuffix(base, "_slab"):
		// Slabs are half-blocks; treat as semi-transparent
		props.Opacity = 2
	case strings.HasSuffix(base, "_stairs"):
		props.Opacity = 2
	case base == "farmland" || base == "dirt_path" || base == "Snow":
		props.Opacity = 1
	case base == "leaves" || strings.HasSuffix(base, "_leaves") || base == "mangrove_roots":
		props.Opacity = 1
	case base == "grass_block" || base == "dirt" || base == "coarse_dirt" || base == "rooted_dirt" ||
		base == "podzol" || base == "mycelium" || base == "mud" || base == "packed_mud" ||
		base == "mud_bricks" || base == "moss_block" || base == "pale_moss_block" ||
		base == "stone" || base == "cobblestone" || base == "mossy_cobblestone" ||
		base == "deepslate" || base == "cobbled_deepslate" || base == "tuff" ||
		base == "calcite" || base == "dripstone_block" || base == "smooth_basalt" ||
		base == "granite" || base == "diorite" || base == "andesite" ||
		base == "sand" || base == "red_sand" || base == "sandstone" || base == "red_sandstone" ||
		base == "gravel" || base == "clay" || base == "terracotta" ||
		base == "snow_block" || base == "packed_ice" || base == "blue_ice" ||
		base == "bamboo_block" || base == "stripped_bamboo_block" ||
		base == "kelp_block" || base == "dried_kelp_block" ||
		base == "nether_wart_block" || base == "warped_wart_block" ||
		base == "bone_block" || base == "hay_block" || base == "melon" || base == "pumpkin" ||
		base == "carved_pumpkin" || base == "bedrock":
		props.Opacity = 15
	case strings.HasSuffix(base, "_ore") || strings.HasSuffix(base, "_deepslate") ||
		strings.HasSuffix(base, "_terracotta") || strings.HasSuffix(base, "_concrete") ||
		strings.HasSuffix(base, "_concrete_powder") || strings.HasSuffix(base, "_wool") ||
		strings.HasSuffix(base, "_planks") || strings.HasSuffix(base, "_log") ||
		strings.HasSuffix(base, "_wood") || strings.HasSuffix(base, "_stem") ||
		strings.HasSuffix(base, "_hyphae") || strings.HasSuffix(base, "_bricks"):
		props.Opacity = 15
	}

	// Non-solid blocks (flowers, grass, torches, etc.) are transparent
	if isNonSolidBlock(base) {
		props.Opacity = 0
	}

	return props
}

// isNonSolidBlock returns true for blocks that don't fill their cube (flowers, grass, etc.).
// IMPORTANT: Never use strings.HasPrefix with short plant names like "grass", "bamboo",
// "nether_wart", or "kelp" because that would falsely match solid blocks like
// "grass_block", "bamboo_block", "nether_wart_block", etc.
func isNonSolidBlock(base string) bool {
	// Never classify *_block as non-solid here.
	if strings.HasSuffix(base, "_block") {
		return false
	}

	switch base {
	case "air", "cave_air", "void_air",
		"short_grass", "grass", "tall_grass", "fern", "large_fern",
		"short_dry_grass", "tall_dry_grass", "bush", "firefly_bush", "dead_bush",
		"seagrass", "tall_seagrass", "kelp", "kelp_plant",
		"dandelion", "poppy", "blue_orchid", "allium", "azure_bluet",
		"red_tulip", "orange_tulip", "white_tulip", "pink_tulip", "oxeye_daisy",
		"cornflower", "lily_of_the_valley", "wither_rose", "sunflower", "lilac",
		"rose_bush", "peony", "lily_pad", "sweet_berry_bush",
		"torchflower", "torchflower_crop", "pitcher_plant", "pitcher_crop",
		"pink_petals", "wildflowers", "leaf_litter", "cactus_flower",
		"spore_blossom", "hanging_roots", "glow_lichen", "sculk_vein",
		"big_dripleaf", "big_dripleaf_stem", "small_dripleaf",
		"azalea", "flowering_azalea", "mangrove_propagule", "frogspawn",
		"moss_carpet", "pale_moss_carpet", "pale_hanging_moss",
		"wheat", "carrots", "potatoes", "beetroots", "melon_stem", "pumpkin_stem",
		"attached_melon_stem", "attached_pumpkin_stem",
		"nether_wart", "chorus_flower", "chorus_plant",
		"red_mushroom", "brown_mushroom", "crimson_fungus", "warped_fungus",
		"sugar_cane", "bamboo", "bamboo_sapling",
		"vine", "weeping_vines", "weeping_vines_plant",
		"twisting_vines", "twisting_vines_plant",
		"cave_vines", "cave_vines_plant",
		"crimson_roots", "warped_roots", "nether_sprouts",
		"torch", "wall_torch", "soul_torch", "soul_wall_torch",
		"redstone_torch", "redstone_wall_torch", "copper_torch", "copper_wall_torch",
		"lever", "rail", "powered_rail", "detector_rail", "activator_rail",
		"tripwire_hook", "tripwire", "snow", "ladder", "scaffolding",
		"redstone_wire", "repeater", "comparator",
		"item_frame", "glow_item_frame", "painting",
		"flower_pot", "candle", "end_rod", "lightning_rod", "chain", "iron_bars",
		"bell", "lantern", "soul_lantern", "campfire", "soul_campfire",
		"cake", "brewing_stand", "cauldron", "water_cauldron", "lava_cauldron", "powder_snow_cauldron",
		"composter", "lectern", "grindstone", "stonecutter",
		"enchanting_table", "anvil", "chipped_anvil", "damaged_anvil",
		"beacon", "conduit", "dragon_egg", "turtle_egg", "sniffer_egg", "dried_ghast":
		return true
	}

	if strings.HasPrefix(base, "potted_") {
		return true
	}

	nonSolidSuffixes := []string{
		"_sapling", "_tulip", "_carpet", "_button", "_pressure_plate",
		"_sign", "_wall_sign", "_hanging_sign", "_wall_hanging_sign",
		"_banner", "_wall_banner", "_candle", "_candle_cake",
		"_rail", "_torch", "_lantern", "_fence", "_fence_gate",
		"_wall", "_door", "_trapdoor", "_glass_pane",
		"_coral", "_coral_fan", "_coral_wall_fan",
		"_amethyst_bud", "amethyst_cluster", "_head", "_skull",
	}
	for _, suf := range nonSolidSuffixes {
		if strings.HasSuffix(base, suf) {
			return true
		}
	}

	return false
}

// GetLightProps returns the light properties for the given canonical state ID.
func GetLightProps(stateID int32) BlockLightProps {
	if stateID < 0 || int(stateID) >= len(blockLightProps) {
		return BlockLightProps{Emission: 0, Opacity: 15}
	}
	return blockLightProps[stateID]
}
