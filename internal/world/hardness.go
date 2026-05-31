package world

// Hardness returns the mining hardness for a block state. Hardness determines
// how long it takes to break a block with the appropriate tool. A hardness of
// -1.0 indicates an unbreakable block (bedrock, barriers, etc.).
//
// Values are sourced from vanilla Minecraft 1.21+ block properties.
func Hardness(blockID int32) float64 {
	// Out-of-range IDs are treated as air by StateName, but we want to
	// distinguish truly unknown blocks from air for default hardness.
	if !ValidStateID(blockID) {
		return 1.0
	}

	name := StateName(blockID)
	if h, ok := hardnessTable[name]; ok {
		return h
	}
	// Default hardness for blocks not in the table
	return 1.0
}

// BreakTicks calculates the number of ticks required to break a block given
// the block state, tool being used, and player status effects.
//
// Parameters:
//   - blockID: canonical world block ID (Java global state ID)
//   - toolID: namespaced tool item ID (e.g. "minecraft:diamond_pickaxe", "" for hand)
//   - inWater: whether the player is submerged in water (5× penalty)
//   - onGround: whether the player is standing on the ground (off-ground = 5× penalty)
//
// Returns the number of ticks (1 tick = 50ms at 20 TPS) to break the block.
// Instant-break blocks return 0. Unbreakable blocks return -1.
func BreakTicks(blockID int32, toolID string, inWater, onGround bool) float64 {
	hardness := Hardness(blockID)

	// Unbreakable blocks
	if hardness < 0 {
		return -1
	}

	// Instant-break blocks (hardness = 0)
	if hardness == 0 {
		return 0
	}

	// Get tool speed multiplier and whether tool can harvest this block
	speedMultiplier, canHarvest := toolEfficiency(blockID, toolID)

	// Base break time calculation
	// Formula: (hardness × 1.5) / speedMultiplier
	baseTime := (hardness * 1.5) / speedMultiplier

	// If tool cannot properly harvest the block, multiply by 5
	if !canHarvest {
		baseTime *= 5.0
	}

	// Apply status effect penalties
	if inWater {
		baseTime *= 5.0
	}
	if !onGround {
		baseTime *= 5.0
	}

	// Convert seconds to ticks (20 ticks per second)
	ticks := baseTime * 20.0

	// Minimum 1 tick to break (except instant-break)
	if ticks < 1.0 {
		return 1.0
	}

	return ticks
}

// toolEfficiency returns the speed multiplier and harvest capability for a
// given tool against a block. Returns (multiplier, canHarvest).
func toolEfficiency(blockID int32, toolID string) (float64, bool) {
	blockName := StateName(blockID)

	// Default: bare hand
	if toolID == "" {
		return 1.0, canHarvestWithHand(blockName)
	}

	// Tool speed multipliers (vanilla values)
	var speedMultiplier float64
	var canHarvest bool

	switch toolID {
	// Wooden tools
	case "minecraft:wooden_pickaxe":
		speedMultiplier = 2.0
		canHarvest = isStone(blockName) || isOre(blockName, "stone")
	case "minecraft:wooden_axe":
		speedMultiplier = 2.0
		canHarvest = isWood(blockName)
	case "minecraft:wooden_shovel":
		speedMultiplier = 2.0
		canHarvest = isDirt(blockName) || isSand(blockName)

	// Stone tools
	case "minecraft:stone_pickaxe":
		speedMultiplier = 4.0
		canHarvest = isStone(blockName) || isOre(blockName, "iron")
	case "minecraft:stone_axe":
		speedMultiplier = 4.0
		canHarvest = isWood(blockName)
	case "minecraft:stone_shovel":
		speedMultiplier = 4.0
		canHarvest = isDirt(blockName) || isSand(blockName)

	// Iron tools
	case "minecraft:iron_pickaxe":
		speedMultiplier = 6.0
		canHarvest = isStone(blockName) || isOre(blockName, "diamond")
	case "minecraft:iron_axe":
		speedMultiplier = 6.0
		canHarvest = isWood(blockName)
	case "minecraft:iron_shovel":
		speedMultiplier = 6.0
		canHarvest = isDirt(blockName) || isSand(blockName)

	// Diamond tools
	case "minecraft:diamond_pickaxe":
		speedMultiplier = 8.0
		canHarvest = isStone(blockName) || isOre(blockName, "netherite")
	case "minecraft:diamond_axe":
		speedMultiplier = 8.0
		canHarvest = isWood(blockName)
	case "minecraft:diamond_shovel":
		speedMultiplier = 8.0
		canHarvest = isDirt(blockName) || isSand(blockName)

	// Netherite tools
	case "minecraft:netherite_pickaxe":
		speedMultiplier = 9.0
		canHarvest = true // Can harvest everything
	case "minecraft:netherite_axe":
		speedMultiplier = 9.0
		canHarvest = isWood(blockName)
	case "minecraft:netherite_shovel":
		speedMultiplier = 9.0
		canHarvest = isDirt(blockName) || isSand(blockName)

	// Golden tools (fast but weak)
	case "minecraft:golden_pickaxe":
		speedMultiplier = 12.0
		canHarvest = isStone(blockName) || isOre(blockName, "stone")
	case "minecraft:golden_axe":
		speedMultiplier = 12.0
		canHarvest = isWood(blockName)
	case "minecraft:golden_shovel":
		speedMultiplier = 12.0
		canHarvest = isDirt(blockName) || isSand(blockName)

	default:
		// Unknown tool, treat as hand
		return 1.0, canHarvestWithHand(blockName)
	}

	return speedMultiplier, canHarvest
}

// Helper functions to categorize blocks by material type

func canHarvestWithHand(blockName string) bool {
	// Blocks that can be harvested with bare hands
	switch blockName {
	case "minecraft:dirt", "minecraft:grass_block", "minecraft:sand",
		"minecraft:gravel", "minecraft:clay", "minecraft:farmland",
		"minecraft:soul_sand", "minecraft:soul_soil", "minecraft:snow",
		"minecraft:snow_block", "minecraft:tnt", "minecraft:slime_block",
		"minecraft:honey_block":
		return true
	}
	return false
}

func isStone(blockName string) bool {
	// Stone-like blocks that require pickaxe
	switch blockName {
	case "minecraft:stone", "minecraft:cobblestone", "minecraft:stone_bricks",
		"minecraft:andesite", "minecraft:diorite", "minecraft:granite",
		"minecraft:deepslate", "minecraft:cobbled_deepslate",
		"minecraft:netherrack", "minecraft:end_stone", "minecraft:obsidian",
		"minecraft:bedrock", "minecraft:iron_ore", "minecraft:gold_ore",
		"minecraft:diamond_ore", "minecraft:coal_ore", "minecraft:copper_ore",
		"minecraft:iron_block", "minecraft:gold_block", "minecraft:diamond_block":
		return true
	}
	return false
}

func isWood(blockName string) bool {
	// Wood-like blocks that are best mined with axe
	switch blockName {
	case "minecraft:oak_log", "minecraft:spruce_log", "minecraft:birch_log",
		"minecraft:jungle_log", "minecraft:acacia_log", "minecraft:dark_oak_log",
		"minecraft:oak_planks", "minecraft:spruce_planks", "minecraft:birch_planks",
		"minecraft:jungle_planks", "minecraft:acacia_planks", "minecraft:dark_oak_planks",
		"minecraft:crafting_table", "minecraft:chest", "minecraft:barrel":
		return true
	}
	return false
}

func isDirt(blockName string) bool {
	switch blockName {
	case "minecraft:dirt", "minecraft:grass_block", "minecraft:podzol",
		"minecraft:mycelium", "minecraft:farmland", "minecraft:clay",
		"minecraft:soul_sand", "minecraft:soul_soil":
		return true
	}
	return false
}

func isSand(blockName string) bool {
	switch blockName {
	case "minecraft:sand", "minecraft:red_sand", "minecraft:gravel",
		"minecraft:snow", "minecraft:snow_block":
		return true
	}
	return false
}

func isOre(blockName, tier string) bool {
	// Check if block is an ore and if it can be harvested with the given tier
	switch tier {
	case "stone":
		// Stone pickaxe can mine iron and below
		switch blockName {
		case "minecraft:coal_ore", "minecraft:iron_ore", "minecraft:copper_ore",
			"minecraft:lapis_ore":
			return true
		}
	case "iron":
		// Iron pickaxe can mine diamond and below
		switch blockName {
		case "minecraft:coal_ore", "minecraft:iron_ore", "minecraft:copper_ore",
			"minecraft:lapis_ore", "minecraft:gold_ore", "minecraft:diamond_ore",
			"minecraft:redstone_ore", "minecraft:emerald_ore":
			return true
		}
	case "diamond", "netherite":
		// Diamond+ can mine everything including ancient debris
		switch blockName {
		case "minecraft:coal_ore", "minecraft:iron_ore", "minecraft:copper_ore",
			"minecraft:lapis_ore", "minecraft:gold_ore", "minecraft:diamond_ore",
			"minecraft:redstone_ore", "minecraft:emerald_ore", "minecraft:ancient_debris",
			"minecraft:obsidian":
			return true
		}
	}
	return false
}

// hardnessTable maps block names to their mining hardness values.
// Source: Minecraft Wiki (1.21+ block properties)
var hardnessTable = map[string]float64{
	// Unbreakable blocks
	"minecraft:bedrock":          -1.0,
	"minecraft:barrier":          -1.0,
	"minecraft:command_block":    -1.0,
	"minecraft:end_portal":       -1.0,
	"minecraft:end_portal_frame": -1.0,
	"minecraft:end_gateway":      -1.0,
	"minecraft:structure_block":  -1.0,
	"minecraft:jigsaw":           -1.0,

	// Instant-break blocks (hardness = 0)
	"minecraft:air":            0.0,
	"minecraft:cave_air":       0.0,
	"minecraft:void_air":       0.0,
	"minecraft:grass":          0.0,
	"minecraft:fern":           0.0,
	"minecraft:dead_bush":      0.0,
	"minecraft:seagrass":       0.0,
	"minecraft:tall_seagrass":  0.0,
	"minecraft:dandelion":      0.0,
	"minecraft:poppy":          0.0,
	"minecraft:torch":          0.0,
	"minecraft:redstone_torch": 0.0,
	"minecraft:fire":           0.0,
	"minecraft:soul_fire":      0.0,
	"minecraft:tnt":            0.0,
	"minecraft:slime_block":    0.0,
	"minecraft:honey_block":    0.0,

	// Soft blocks (hardness < 1.0)
	"minecraft:dirt":        0.5,
	"minecraft:grass_block": 0.6,
	"minecraft:sand":        0.5,
	"minecraft:red_sand":    0.5,
	"minecraft:gravel":      0.6,
	"minecraft:clay":        0.6,
	"minecraft:farmland":    0.6,
	"minecraft:soul_sand":   0.5,
	"minecraft:soul_soil":   0.5,
	"minecraft:snow":        0.1,
	"minecraft:snow_block":  0.2,
	"minecraft:powder_snow": 0.25,

	// Wood blocks (hardness = 2.0)
	"minecraft:oak_log":         2.0,
	"minecraft:spruce_log":      2.0,
	"minecraft:birch_log":       2.0,
	"minecraft:jungle_log":      2.0,
	"minecraft:acacia_log":      2.0,
	"minecraft:dark_oak_log":    2.0,
	"minecraft:mangrove_log":    2.0,
	"minecraft:cherry_log":      2.0,
	"minecraft:oak_planks":      2.0,
	"minecraft:spruce_planks":   2.0,
	"minecraft:birch_planks":    2.0,
	"minecraft:jungle_planks":   2.0,
	"minecraft:acacia_planks":   2.0,
	"minecraft:dark_oak_planks": 2.0,
	"minecraft:mangrove_planks": 2.0,
	"minecraft:cherry_planks":   2.0,
	"minecraft:crafting_table":  2.5,
	"minecraft:chest":           2.5,
	"minecraft:barrel":          2.5,

	// Stone blocks (hardness = 1.5)
	"minecraft:stone":             1.5,
	"minecraft:cobblestone":       2.0,
	"minecraft:stone_bricks":      1.5,
	"minecraft:andesite":          1.5,
	"minecraft:diorite":           1.5,
	"minecraft:granite":           1.5,
	"minecraft:deepslate":         3.0,
	"minecraft:cobbled_deepslate": 3.5,
	"minecraft:netherrack":        0.4,
	"minecraft:end_stone":         3.0,

	// Ores
	"minecraft:coal_ore":               3.0,
	"minecraft:iron_ore":               3.0,
	"minecraft:copper_ore":             3.0,
	"minecraft:gold_ore":               3.0,
	"minecraft:diamond_ore":            3.0,
	"minecraft:emerald_ore":            3.0,
	"minecraft:lapis_ore":              3.0,
	"minecraft:redstone_ore":           3.0,
	"minecraft:deepslate_coal_ore":     4.5,
	"minecraft:deepslate_iron_ore":     4.5,
	"minecraft:deepslate_copper_ore":   4.5,
	"minecraft:deepslate_gold_ore":     4.5,
	"minecraft:deepslate_diamond_ore":  4.5,
	"minecraft:deepslate_emerald_ore":  4.5,
	"minecraft:deepslate_lapis_ore":    4.5,
	"minecraft:deepslate_redstone_ore": 4.5,
	"minecraft:nether_gold_ore":        3.0,
	"minecraft:nether_quartz_ore":      3.0,
	"minecraft:ancient_debris":         30.0,

	// Metal blocks
	"minecraft:iron_block":      5.0,
	"minecraft:gold_block":      3.0,
	"minecraft:diamond_block":   5.0,
	"minecraft:emerald_block":   5.0,
	"minecraft:netherite_block": 50.0,

	// Hard blocks
	"minecraft:obsidian":         50.0,
	"minecraft:crying_obsidian":  50.0,
	"minecraft:respawn_anchor":   50.0,
	"minecraft:anvil":            5.0,
	"minecraft:enchanting_table": 5.0,
	"minecraft:ender_chest":      22.5,

	// Glass (hardness = 0.3)
	"minecraft:glass":                    0.3,
	"minecraft:glass_pane":               0.3,
	"minecraft:white_stained_glass":      0.3,
	"minecraft:orange_stained_glass":     0.3,
	"minecraft:magenta_stained_glass":    0.3,
	"minecraft:light_blue_stained_glass": 0.3,
	"minecraft:yellow_stained_glass":     0.3,
	"minecraft:lime_stained_glass":       0.3,
	"minecraft:pink_stained_glass":       0.3,
	"minecraft:gray_stained_glass":       0.3,
	"minecraft:light_gray_stained_glass": 0.3,
	"minecraft:cyan_stained_glass":       0.3,
	"minecraft:purple_stained_glass":     0.3,
	"minecraft:blue_stained_glass":       0.3,
	"minecraft:brown_stained_glass":      0.3,
	"minecraft:green_stained_glass":      0.3,
	"minecraft:red_stained_glass":        0.3,
	"minecraft:black_stained_glass":      0.3,
}
