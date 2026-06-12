package pipeline

import (
	"testing"

	"livingworld/internal/world"
)

// TestGenerateLooksLikeOverworld is a sanity gate on the rewritten
// generator: varied heights, real surface materials (never bedrock on
// top), bedrock only at the world floor, water in oceans, and ores in
// the stone body.
func TestGenerateLooksLikeOverworld(t *testing.T) {
	g := NewGenerator(12345)
	bedrockID := world.StateID("minecraft:bedrock")
	grassID := world.StateID("minecraft:grass_block")
	waterID := world.StateID("minecraft:water")

	minH, maxH := 1<<30, -(1 << 30)
	biomes := map[string]bool{}
	waterCols := 0
	topBedrock := 0
	oreBlocks := 0
	oreIDs := map[int32]bool{
		world.StateID("minecraft:coal_ore"):              true,
		world.StateID("minecraft:iron_ore"):              true,
		world.StateID("minecraft:copper_ore"):            true,
		world.StateID("minecraft:deepslate_diamond_ore"): true,
		world.StateID("minecraft:deepslate_iron_ore"):    true,
		world.StateID("minecraft:deepslate_redstone_ore"): true,
		world.StateID("minecraft:deepslate_gold_ore"):    true,
	}
	grassTops := 0

	for cx := -16; cx <= 16; cx += 2 {
		for cz := -16; cz <= 16; cz += 2 {
			c := g.Generate(cx, cz)
			for lz := 0; lz < 16; lz++ {
				for lx := 0; lx < 16; lx++ {
					col := g.shapeColumn(cx*16+lx, cz*16+lz)
					biomes[string(col.biome.ID)] = true
					// Top block must never be bedrock (the bug in the
					// screenshot).
					topY := int(c.GetHeightmap(lx, lz))
					if topY < minH {
						minH = topY
					}
					if topY > maxH {
						maxH = topY
					}
					top := c.GetBlock(lx, topY, lz).ID()
					if top == bedrockID {
						topBedrock++
					}
					if top == grassID {
						grassTops++
					}
					if c.GetBlock(lx, 60, lz).ID() == waterID {
						waterCols++
					}
				}
			}
			// Bedrock must stay in the floor band.
			for lz := 0; lz < 16; lz += 8 {
				for lx := 0; lx < 16; lx += 8 {
					for y := -50; y < 200; y += 3 {
						if c.GetBlock(lx, y, lz).ID() == bedrockID {
							t.Fatalf("bedrock at y=%d in chunk (%d,%d)", y, cx, cz)
						}
					}
				}
			}
			// Ore census across the full stone body of one column row.
			for lx := 0; lx < 16; lx++ {
				for y := -60; y < 80; y++ {
					if oreIDs[c.GetBlock(lx, y, 8).ID()] {
						oreBlocks++
					}
				}
			}
		}
	}

	if topBedrock > 0 {
		t.Errorf("found %d columns with bedrock as the surface block", topBedrock)
	}
	if spread := maxH - minH; spread < 25 {
		t.Errorf("terrain too flat: height spread %d (min %d max %d)", spread, minH, maxH)
	}
	if len(biomes) < 5 {
		t.Errorf("too few biomes generated: %v", biomes)
	}
	if waterCols == 0 {
		t.Errorf("no ocean/river water found in sampled region")
	}
	if grassTops == 0 {
		t.Errorf("no grass-topped columns found")
	}
	if oreBlocks == 0 {
		t.Errorf("no ores found in sampled columns")
	}
	t.Logf("height %d..%d, %d biomes %v, water cols %d, grass tops %d, ores %d",
		minH, maxH, len(biomes), keys(biomes), waterCols, grassTops, oreBlocks)
}

// TestGenerateDeterministic: same seed + same chunk → identical blocks.
func TestGenerateDeterministic(t *testing.T) {
	g1 := NewGenerator(777)
	g2 := NewGenerator(777)
	a := g1.Generate(3, -2)
	b := g2.Generate(3, -2)
	for y := -64; y < 200; y++ {
		for lz := 0; lz < 16; lz++ {
			for lx := 0; lx < 16; lx++ {
				if a.GetBlock(lx, y, lz).ID() != b.GetBlock(lx, y, lz).ID() {
					t.Fatalf("mismatch at (%d,%d,%d)", lx, y, lz)
				}
			}
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
