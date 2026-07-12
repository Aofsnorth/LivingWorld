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
	case strings.HasPrefix(base, "oak_") || strings.HasPrefix(base, "spruce_") ||
		strings.HasPrefix(base, "birch_") || strings.HasPrefix(base, "jungle_") ||
		strings.HasPrefix(base, "acacia_") || strings.HasPrefix(base, "dark_oak_") ||
		strings.HasPrefix(base, "crimson_") || strings.HasPrefix(base, "warped_"):
		// Wooden blocks are opaque
		props.Opacity = 15
	case base == "grass_block" || base == "dirt" || base == "stone" || base == "cobblestone":
		props.Opacity = 15
	case base == "sand" || base == "gravel" || base == "clay":
		props.Opacity = 15
	case strings.HasSuffix(base, "_ore") || strings.HasSuffix(base, "_deepslate"):
		props.Opacity = 15
	case base == "bedrock":
		props.Opacity = 15
	}

	// Non-solid blocks (flowers, grass, torches, etc.) are transparent
	if isNonSolidBlock(base) {
		props.Opacity = 0
	}

	return props
}

// isNonSolidBlock returns true for blocks that don't fill their cube (flowers, grass, etc.).
func isNonSolidBlock(base string) bool {
	nonSolid := []string{
		"grass", "fern", "dead_bush", "seagrass", "tall_grass", "large_fern",
		"dandelion", "poppy", "blue_orchid", "allium", "azure_bluet",
		"red_tulip", "orange_tulip", "white_tulip", "pink_tulip", "oxeye_daisy",
		"cornflower", "lily_of_the_valley", "wither_rose", "sunflower", "lilac",
		"rose_bush", "peony", "lily_pad", "sweet_berry_bush",
		"wheat", "carrots", "potatoes", "beetroots", "melon_stem", "pumpkin_stem",
		"nether_wart", "chorus_flower", "chorus_plant",
		"red_mushroom", "brown_mushroom",
		"sugar_cane", "kelp", "bamboo", "vine", "weeping_vines", "twisting_vines",
		"crimson_roots", "warped_roots", "nether_sprouts",
		"torch", "wall_torch", "soul_torch", "soul_wall_torch", "redstone_torch", "redstone_wall_torch",
		"lever", "rail", "powered_rail", "detector_rail", "activator_rail",
		"stone_button", "oak_button", "spruce_button", "birch_button", "jungle_button",
		"acacia_button", "dark_oak_button", "crimson_button", "warped_button", "polished_blackstone_button",
		"stone_pressure_plate", "oak_pressure_plate", "spruce_pressure_plate", "birch_pressure_plate",
		"jungle_pressure_plate", "acacia_pressure_plate", "dark_oak_pressure_plate",
		"crimson_pressure_plate", "warped_pressure_plate", "polished_blackstone_pressure_plate",
		"light_weighted_pressure_plate", "heavy_weighted_pressure_plate",
		"tripwire_hook", "tripwire", "snow", "moss_carpet",
		"redstone_wire", "repeater", "comparator",
		"sign", "wall_sign", "oak_sign", "oak_wall_sign",
		"spruce_sign", "spruce_wall_sign", "birch_sign", "birch_wall_sign",
		"jungle_sign", "jungle_wall_sign", "acacia_sign", "acacia_wall_sign",
		"dark_oak_sign", "dark_oak_wall_sign", "crimson_sign", "crimson_wall_sign",
		"warped_sign", "warped_wall_sign",
		"item_frame", "glow_item_frame", "painting",
		"flower_pot", "potted_", "candle", "candles",
		"end_rod", "lightning_rod", "chain",
		"bell", "lantern", "soul_lantern", "campfire", "soul_campfire",
		"cake", "candle_cake",
		"brewing_stand", "cauldron", "water_cauldron", "lava_cauldron", "powder_snow_cauldron",
		"composter", "lectern", "grindstone", "stonecutter", "loom", "cartography_table",
		"fletching_table", "smithing_table", "smoker", "blast_furnace", "furnace",
		"enchanting_table", "anvil", "chipped_anvil", "damaged_anvil",
		"beacon", "conduit", "lodestone", "respawn_anchor",
		"banner", "wall_banner",
	}

	for _, ns := range nonSolid {
		if base == ns || strings.HasPrefix(base, ns) {
			return true
		}
	}

	// Check for potted plants
	if strings.HasPrefix(base, "potted_") {
		return true
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
