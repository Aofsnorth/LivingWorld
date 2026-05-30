package world

import (
	"math"
	"testing"
)

func TestHardness(t *testing.T) {
	tests := []struct {
		name     string
		blockID  int32
		expected float64
	}{
		{"air", StateID("minecraft:air"), 0.0},
		{"stone", StateID("minecraft:stone"), 1.5},
		{"dirt", StateID("minecraft:dirt"), 0.5},
		{"obsidian", StateID("minecraft:obsidian"), 50.0},
		{"bedrock", StateID("minecraft:bedrock"), -1.0},
		{"oak_log", StateID("minecraft:oak_log"), 2.0},
		{"diamond_ore", StateID("minecraft:diamond_ore"), 3.0},
		{"netherite_block", StateID("minecraft:netherite_block"), 50.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Hardness(tt.blockID)
			if got != tt.expected {
				t.Errorf("Hardness(%s) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestHardness_UnknownBlock(t *testing.T) {
	// Unknown blocks should default to 1.0
	unknownID := int32(99999)
	got := Hardness(unknownID)
	if got != 1.0 {
		t.Errorf("Hardness(unknown) = %v, want 1.0", got)
	}
}

func TestBreakTicks_InstantBreak(t *testing.T) {
	// Air and other instant-break blocks should return 0
	airID := StateID("minecraft:air")
	ticks := BreakTicks(airID, "", false, true)
	if ticks != 0 {
		t.Errorf("BreakTicks(air) = %v, want 0", ticks)
	}

	torchID := StateID("minecraft:torch")
	ticks = BreakTicks(torchID, "", false, true)
	if ticks != 0 {
		t.Errorf("BreakTicks(torch) = %v, want 0", ticks)
	}
}

func TestBreakTicks_Unbreakable(t *testing.T) {
	// Bedrock should return -1
	bedrockID := StateID("minecraft:bedrock")
	ticks := BreakTicks(bedrockID, "", false, true)
	if ticks != -1 {
		t.Errorf("BreakTicks(bedrock) = %v, want -1", ticks)
	}

	barrierID := StateID("minecraft:barrier")
	ticks = BreakTicks(barrierID, "", false, true)
	if ticks != -1 {
		t.Errorf("BreakTicks(barrier) = %v, want -1", ticks)
	}
}

func TestBreakTicks_BareHand(t *testing.T) {
	// Breaking stone with bare hand should be slow
	stoneID := StateID("minecraft:stone")
	ticks := BreakTicks(stoneID, "", false, true)

	// Stone hardness = 1.5
	// Formula: (1.5 × 1.5) / 1.0 × 5 (can't harvest) × 20 = 225 ticks
	expected := 225.0
	if ticks != expected {
		t.Errorf("BreakTicks(stone, bare hand) = %v, want %v", ticks, expected)
	}
}

func TestBreakTicks_WithCorrectTool(t *testing.T) {
	// Breaking stone with stone pickaxe
	stoneID := StateID("minecraft:stone")
	ticks := BreakTicks(stoneID, "minecraft:stone_pickaxe", false, true)

	// Stone hardness = 1.5
	// Formula: (1.5 × 1.5) / 4.0 × 20 = 11.25 ticks
	expected := 11.25
	if math.Abs(ticks-expected) > 0.01 {
		t.Errorf("BreakTicks(stone, stone_pickaxe) = %v, want %v", ticks, expected)
	}
}

func TestBreakTicks_DiamondPickaxe(t *testing.T) {
	// Breaking obsidian with diamond pickaxe
	obsidianID := StateID("minecraft:obsidian")
	ticks := BreakTicks(obsidianID, "minecraft:diamond_pickaxe", false, true)

	// Obsidian hardness = 50.0
	// Formula: (50.0 × 1.5) / 8.0 × 20 = 187.5 ticks
	expected := 187.5
	if math.Abs(ticks-expected) > 0.01 {
		t.Errorf("BreakTicks(obsidian, diamond_pickaxe) = %v, want %v", ticks, expected)
	}
}

func TestBreakTicks_WrongTool(t *testing.T) {
	// Breaking stone with axe (wrong tool, can't harvest)
	stoneID := StateID("minecraft:stone")
	ticks := BreakTicks(stoneID, "minecraft:diamond_axe", false, true)

	// Stone hardness = 1.5
	// Diamond axe speed = 8.0, but can't harvest stone
	// Formula: (1.5 × 1.5) / 8.0 × 5 (can't harvest) × 20 = 28.125 ticks
	expected := 28.125
	if math.Abs(ticks-expected) > 0.01 {
		t.Errorf("BreakTicks(stone, diamond_axe) = %v, want %v", ticks, expected)
	}
}

func TestBreakTicks_InWater(t *testing.T) {
	// Breaking stone with stone pickaxe while in water
	stoneID := StateID("minecraft:stone")
	ticks := BreakTicks(stoneID, "minecraft:stone_pickaxe", true, true)

	// Stone hardness = 1.5
	// Formula: (1.5 × 1.5) / 4.0 × 5 (in water) × 20 = 56.25 ticks
	expected := 56.25
	if math.Abs(ticks-expected) > 0.01 {
		t.Errorf("BreakTicks(stone, stone_pickaxe, in water) = %v, want %v", ticks, expected)
	}
}

func TestBreakTicks_NotOnGround(t *testing.T) {
	// Breaking stone with stone pickaxe while not on ground
	stoneID := StateID("minecraft:stone")
	ticks := BreakTicks(stoneID, "minecraft:stone_pickaxe", false, false)

	// Stone hardness = 1.5
	// Formula: (1.5 × 1.5) / 4.0 × 5 (not on ground) × 20 = 56.25 ticks
	expected := 56.25
	if math.Abs(ticks-expected) > 0.01 {
		t.Errorf("BreakTicks(stone, stone_pickaxe, not on ground) = %v, want %v", ticks, expected)
	}
}

func TestBreakTicks_InWaterAndNotOnGround(t *testing.T) {
	// Breaking stone with stone pickaxe while in water AND not on ground
	stoneID := StateID("minecraft:stone")
	ticks := BreakTicks(stoneID, "minecraft:stone_pickaxe", true, false)

	// Stone hardness = 1.5
	// Formula: (1.5 × 1.5) / 4.0 × 5 (in water) × 5 (not on ground) × 20 = 281.25 ticks
	expected := 281.25
	if math.Abs(ticks-expected) > 0.01 {
		t.Errorf("BreakTicks(stone, stone_pickaxe, in water + not on ground) = %v, want %v", ticks, expected)
	}
}

func TestBreakTicks_GoldenPickaxe(t *testing.T) {
	// Golden pickaxe is fast but weak (can only mine stone-tier)
	stoneID := StateID("minecraft:stone")
	ticks := BreakTicks(stoneID, "minecraft:golden_pickaxe", false, true)

	// Stone hardness = 1.5
	// Golden pickaxe speed = 12.0
	// Formula: (1.5 × 1.5) / 12.0 × 20 = 3.75 ticks
	expected := 3.75
	if math.Abs(ticks-expected) > 0.01 {
		t.Errorf("BreakTicks(stone, golden_pickaxe) = %v, want %v", ticks, expected)
	}
}

func TestBreakTicks_WoodWithAxe(t *testing.T) {
	// Breaking oak log with iron axe
	oakLogID := StateID("minecraft:oak_log")
	ticks := BreakTicks(oakLogID, "minecraft:iron_axe", false, true)

	// Oak log hardness = 2.0
	// Iron axe speed = 6.0
	// Formula: (2.0 × 1.5) / 6.0 × 20 = 10 ticks
	expected := 10.0
	if math.Abs(ticks-expected) > 0.01 {
		t.Errorf("BreakTicks(oak_log, iron_axe) = %v, want %v", ticks, expected)
	}
}

func TestBreakTicks_DirtWithShovel(t *testing.T) {
	// Breaking dirt with diamond shovel
	dirtID := StateID("minecraft:dirt")
	ticks := BreakTicks(dirtID, "minecraft:diamond_shovel", false, true)

	// Dirt hardness = 0.5
	// Diamond shovel speed = 8.0
	// Formula: (0.5 × 1.5) / 8.0 × 20 = 1.875 ticks (rounds to 1.875)
	expected := 1.875
	if math.Abs(ticks-expected) > 0.01 {
		t.Errorf("BreakTicks(dirt, diamond_shovel) = %v, want %v", ticks, expected)
	}
}

func TestBreakTicks_MinimumTicks(t *testing.T) {
	// Very soft blocks should still take at least 1 tick
	snowID := StateID("minecraft:snow")

	// Snow is instant-break (hardness = 0), should return 0
	ticks := BreakTicks(snowID, "", false, true)
	if ticks != 0 {
		t.Errorf("BreakTicks(snow) = %v, want 0 (instant break)", ticks)
	}

	// But snow_block has hardness 0.2
	snowBlockID := StateID("minecraft:snow_block")
	ticks = BreakTicks(snowBlockID, "minecraft:diamond_shovel", false, true)

	// Formula: (0.2 × 1.5) / 8.0 × 20 = 0.75 ticks → minimum 1 tick
	if ticks < 1.0 {
		t.Errorf("BreakTicks(snow_block, diamond_shovel) = %v, want >= 1.0", ticks)
	}
}

func TestToolEfficiency_Categorization(t *testing.T) {
	tests := []struct {
		name        string
		blockName   string
		toolID      string
		wantSpeed   float64
		wantHarvest bool
	}{
		{"stone with wooden pickaxe", "minecraft:stone", "minecraft:wooden_pickaxe", 2.0, true},
		{"iron ore with stone pickaxe", "minecraft:iron_ore", "minecraft:stone_pickaxe", 4.0, true},
		{"diamond ore with iron pickaxe", "minecraft:diamond_ore", "minecraft:iron_pickaxe", 6.0, true},
		{"obsidian with diamond pickaxe", "minecraft:obsidian", "minecraft:diamond_pickaxe", 8.0, true},
		{"oak log with iron axe", "minecraft:oak_log", "minecraft:iron_axe", 6.0, true},
		{"dirt with diamond shovel", "minecraft:dirt", "minecraft:diamond_shovel", 8.0, true},
		{"stone with bare hand", "minecraft:stone", "", 1.0, false},
		{"dirt with bare hand", "minecraft:dirt", "", 1.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockID := StateID(tt.blockName)
			speed, harvest := toolEfficiency(blockID, tt.toolID)

			if speed != tt.wantSpeed {
				t.Errorf("toolEfficiency(%s, %s) speed = %v, want %v",
					tt.blockName, tt.toolID, speed, tt.wantSpeed)
			}
			if harvest != tt.wantHarvest {
				t.Errorf("toolEfficiency(%s, %s) harvest = %v, want %v",
					tt.blockName, tt.toolID, harvest, tt.wantHarvest)
			}
		})
	}
}
