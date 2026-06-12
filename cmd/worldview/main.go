package main

import (
	"fmt"

	"livingworld/internal/dimension/overworld/biome"
	"livingworld/internal/dimension/overworld/pipeline"
	"livingworld/internal/world"
)

func main() {
	g := pipeline.NewGenerator(12345)
	bedrock := world.StateID("minecraft:bedrock")
	water := world.StateID("minecraft:water")
	grass := world.StateID("minecraft:grass_block")
	oreSet := map[int32]string{
		world.StateID("minecraft:coal_ore"):              "coal",
		world.StateID("minecraft:iron_ore"):              "iron",
		world.StateID("minecraft:copper_ore"):            "copper",
		world.StateID("minecraft:gold_ore"):              "gold",
		world.StateID("minecraft:deepslate_coal_ore"):    "dcoal",
		world.StateID("minecraft:deepslate_iron_ore"):    "diron",
		world.StateID("minecraft:deepslate_diamond_ore"): "diam",
		world.StateID("minecraft:emerald_ore"):           "emer",
	}
	biomeCounts := map[biome.ID]int{}
	oreCounts := map[string]int{}
	tops := map[string]int{}
	heights := []int{}
	for cx := -32; cx <= 32; cx++ {
		for cz := -32; cz <= 32; cz++ {
			c := g.Generate(cx, cz)
			for lx := 0; lx < 16; lx++ {
				for lz := 0; lz < 16; lz++ {
					h := int(c.GetHeightmap(lx, lz))
					heights = append(heights, h)
					// Top non-air block — the surface.
					bid := c.GetBlock(lx, h, lz).ID()
					topName := world.StateName(bid)
					tops[topName]++
					_ = biomeCounts
				}
			}
			for lx := 0; lx < 16; lx++ {
				for y := -60; y < 60; y++ {
					if n, ok := oreSet[c.GetBlock(lx, y, 8).ID()]; ok {
						oreCounts[n]++
					}
				}
			}
		}
	}
	fmt.Println("=== Heights ===")
	minH, maxH := heights[0], heights[0]
	for _, h := range heights {
		if h < minH {
			minH = h
		}
		if h > maxH {
			maxH = h
		}
	}
	sumH := 0
	for _, h := range heights {
		sumH += h
	}
	fmt.Printf("min=%d max=%d avg=%.1f\n", minH, maxH, float64(sumH)/float64(len(heights)))

	fmt.Println("=== Top block census ===")
	for k, v := range tops {
		if k == "" {
			continue
		}
		fmt.Printf("  %-40s %d\n", k, v)
	}
	fmt.Println("=== Biome census ===")
	for k, v := range biomeCounts {
		fmt.Printf("  %-40s %d\n", string(k), v)
	}
	fmt.Println("=== Ore census ===")
	for k, v := range oreCounts {
		fmt.Printf("  %-10s %d\n", k, v)
	}
	_ = bedrock
	_ = water
	_ = grass
}
