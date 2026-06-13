package main

import (
	"fmt"

	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/dimension/overworld/pipeline"
	"livingworld/internal/world"

	dfworld "github.com/df-mc/dragonfly/server/world"
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
)

func main() {
	g := pipeline.NewGenerator(12345)
	rng := dfworld.Overworld.Range()
	fmt.Printf("dragonfly range: min=%d max=%d Height()=%d -> subChunkCount=%d\n",
		rng.Min(), rng.Max(), rng.Height(), (rng.Height()>>4)+1)

	airRID := bedrockworld.BlockRID("minecraft:air")
	fmt.Printf("airRID=%d\n", airRID)

	// Replicate SendChunk's conversion for chunk (5,7).
	wchunk := g.Generate(5, 7)
	ch := dfchunk.New(airRID, rng)
	maxY := int(rng.Max())
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			for y := int(rng.Min()); y <= maxY; y++ {
				id := wchunk.GetBlock(x, y, z).ID()
				if id == 0 {
					continue
				}
				ch.SetBlock(uint8(x), int16(y), uint8(z), 0, bedrockworld.LivingWorldBlockIDToBedrockRID(id))
			}
		}
	}

	// Count non-air per subchunk in the df chunk.
	fmt.Println("df chunk per-subchunk non-air:")
	for sec := 0; sec < 24; sec++ {
		base := int16(rng.Min()) + int16(sec*16)
		n := 0
		for y := base; y < base+16; y++ {
			for z := uint8(0); z < 16; z++ {
				for x := uint8(0); x < 16; x++ {
					if ch.Block(x, y, z, 0) != airRID {
						n++
					}
				}
			}
		}
		if n > 0 {
			fmt.Printf("  sub %2d (y %4d..%4d): %d\n", sec, base, base+15, n)
		}
	}

	// Encode and measure payload sizes.
	data := dfchunk.Encode(ch, dfchunk.NetworkEncoding)
	fmt.Printf("encoded: %d subchunks, biome bytes=%d, HighestFilledSubChunk=%d\n",
		len(data.SubChunks), len(data.Biomes), ch.HighestFilledSubChunk())
	total := 0
	for i, sub := range data.SubChunks {
		total += len(sub)
		if i < 10 || len(sub) > 2000 {
			fmt.Printf("  sub %2d: %d bytes\n", i, len(sub))
		}
	}
	fmt.Printf("total payload: %d bytes\n", total+len(data.Biomes))

	// Census the RID mapping over every distinct block the generator places.
	names := []string{
		"minecraft:bedrock", "minecraft:stone", "minecraft:deepslate", "minecraft:dirt",
		"minecraft:grass_block", "minecraft:podzol", "minecraft:mycelium", "minecraft:sand",
		"minecraft:sandstone", "minecraft:red_sand", "minecraft:gravel", "minecraft:water",
		"minecraft:lava", "minecraft:ice", "minecraft:snow_block", "minecraft:snow",
		"minecraft:mud", "minecraft:terracotta", "minecraft:orange_terracotta",
		"minecraft:yellow_terracotta", "minecraft:white_terracotta", "minecraft:brown_terracotta",
		"minecraft:granite", "minecraft:diorite", "minecraft:andesite", "minecraft:tuff",
		"minecraft:oak_log", "minecraft:oak_leaves", "minecraft:birch_log", "minecraft:birch_leaves",
		"minecraft:spruce_log", "minecraft:spruce_leaves", "minecraft:jungle_log", "minecraft:jungle_leaves",
		"minecraft:acacia_log", "minecraft:acacia_leaves", "minecraft:cherry_log", "minecraft:cherry_leaves",
		"minecraft:short_grass", "minecraft:fern", "minecraft:dandelion", "minecraft:poppy",
		"minecraft:cactus", "minecraft:dead_bush", "minecraft:coal_ore", "minecraft:iron_ore",
		"minecraft:copper_ore", "minecraft:gold_ore", "minecraft:redstone_ore", "minecraft:lapis_ore",
		"minecraft:diamond_ore", "minecraft:emerald_ore", "minecraft:deepslate_coal_ore",
		"minecraft:deepslate_iron_ore", "minecraft:deepslate_copper_ore", "minecraft:deepslate_gold_ore",
		"minecraft:deepslate_redstone_ore", "minecraft:deepslate_lapis_ore", "minecraft:deepslate_diamond_ore",
		"minecraft:deepslate_emerald_ore",
	}
	fmt.Println("RID resolution check (lwID -> RID):")
	for _, n := range names {
		lwID := world.StateID(n)
		rid := bedrockworld.LivingWorldBlockIDToBedrockRID(lwID)
		mark := ""
		if lwID == 0 && n != "minecraft:air" {
			mark = "  <-- LW STATE MISSING (resolves to air!)"
		}
		if rid == 0 {
			mark += "  <-- BEDROCK RID FAILED"
		}
		if mark != "" {
			fmt.Printf("  %-40s lw=%-6d rid=%-6d%s\n", n, lwID, rid, mark)
		}
	}
	fmt.Println("(only problems listed above)")
}
