package pipeline

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"livingworld/internal/dimension/overworld/vanilladata"
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
		world.StateID("minecraft:coal_ore"):               true,
		world.StateID("minecraft:iron_ore"):               true,
		world.StateID("minecraft:copper_ore"):             true,
		world.StateID("minecraft:deepslate_diamond_ore"):  true,
		world.StateID("minecraft:deepslate_iron_ore"):     true,
		world.StateID("minecraft:deepslate_redstone_ore"): true,
		world.StateID("minecraft:deepslate_gold_ore"):     true,
	}
	grassTops := 0

	for cx := -16; cx <= 16; cx += 2 {
		for cz := -16; cz <= 16; cz += 2 {
			c := g.Generate(cx, cz)
			for lz := 0; lz < 16; lz++ {
				for lx := 0; lx < 16; lx++ {
					biomes[string(g.BiomeAt(cx*16+lx, cz*16+lz))] = true
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

// TestGenerateCavesAndOverhangs proves the terrain body is the vanilla
// final_density sign field, not a filled column with a post-hoc carver:
//
//   - every block below the decoration band is solid exactly where the
//     interpolated tree is positive (and air where it is not, above the
//     lava level);
//   - underground air pockets (caves) exist below the surface;
//   - solid ceilings with cave air directly under them (overhangs)
//     exist below the column top.
func TestGenerateCavesAndOverhangs(t *testing.T) {
	g := NewGenerator(12345)

	// Decoration blocks may legally occupy negative-density cells (a
	// canopy reaching into a cave mouth), so they are skipped by the
	// solidity invariant.
	deco := map[int32]bool{
		world.StateID("minecraft:oak_leaves"):    true,
		world.StateID("minecraft:birch_leaves"):  true,
		world.StateID("minecraft:spruce_leaves"): true,
		world.StateID("minecraft:jungle_leaves"): true,
		world.StateID("minecraft:acacia_leaves"): true,
		world.StateID("minecraft:cherry_leaves"): true,
		world.StateID("minecraft:oak_log"):       true,
		world.StateID("minecraft:birch_log"):     true,
		world.StateID("minecraft:spruce_log"):    true,
		world.StateID("minecraft:jungle_log"):    true,
		world.StateID("minecraft:acacia_log"):    true,
		world.StateID("minecraft:cherry_log"):    true,
		world.StateID("minecraft:snow"):          true,
	}

	caves, overhangs, checked := 0, 0, 0
	var solid [maxY + 1 - minY]bool
	for cx := -4; cx <= 4; cx++ {
		for cz := -4; cz <= 4; cz++ {
			c := g.Generate(cx, cz)
			dens := g.buildChunkDensity(cx, cz)
			for lz := 0; lz < 16; lz++ {
				for lx := 0; lx < 16; lx++ {
					dens.scanColumn(lx, lz, solid[:])
					top := minY
					for y := maxY; y >= minY; y-- {
						if solid[y-minY] {
							top = y
							break
						}
					}
					// [minY+6, top-3] stays below the surface band, the
					// bedrock gradient, and any decoration.
					for y := minY + 6; y <= top-3; y++ {
						id := c.GetBlock(lx, y, lz).ID()
						checked++
						if solid[y-minY] {
							if id == world.AirID {
								t.Errorf("(%d,%d) y=%d: air in a positive-density cell", lx, lz, y)
							}
							if y > minY+7 && !solid[y-1-minY] {
								overhangs++ // cave ceiling: solid with air below
							}
						} else if y > lavaLevel {
							if id != world.AirID && !deco[id] {
								t.Errorf("(%d,%d) y=%d: block %d in a negative-density cell", lx, lz, y, id)
							}
							if id == world.AirID {
								caves++ // dry cave air below the surface
							}
						}
					}
				}
			}
		}
	}

	if caves == 0 {
		t.Errorf("no cave air pockets found below the surface in 9x9 chunks")
	}
	if overhangs == 0 {
		t.Errorf("no overhang ceilings found below the surface in 9x9 chunks")
	}
	t.Logf("%d cells checked: %d cave cells, %d overhang ceilings", checked, caves, overhangs)
}

// TestBiomeParameterListIntegrity checks the embedded vanilla parameter
// list decodes to the dumped shape: 7594 entries over 56 biomes with every
// span ordered.
func TestBiomeParameterListIntegrity(t *testing.T) {
	if got := len(vanilladata.BiomeParamAxes); got != 7594 {
		t.Errorf("parameter entries = %d, want 7594", got)
	}
	if got := len(vanilladata.BiomeParamNames); got != 56 {
		t.Errorf("biome names = %d, want 56", got)
	}
	for i, ax := range vanilladata.BiomeParamAxes {
		for a := 0; a < 6; a++ {
			if ax[2*a] > ax[2*a+1] {
				t.Fatalf("entry %d axis %d: min %d > max %d", i, a, ax[2*a], ax[2*a+1])
			}
		}
	}
}

// TestSelectBiomeVanillaFitness pins vanilla's fitness semantics on a
// hand-checkable case: the list's first entry (mushroom_fields — full
// t/h/e/w spans, continentalness [-1.2, -1.05], depth exactly 0) wins with
// zero fitness for a target inside its spans, and no entry can score below
// zero so the tie-break keeps the first.
func TestSelectBiomeVanillaFitness(t *testing.T) {
	got := selectBiome(climateTarget{t: 0, h: 0, c: -11000, e: 0, d: 0, w: 0})
	if got.ID != "minecraft:mushroom_fields" {
		t.Errorf("inside-span target selected %s, want minecraft:mushroom_fields", got.ID)
	}
}

// TestBiomeSelectionMatchesVanilla pins our biome selection to the vanilla
// 26.3 answer: every column in testdata/biome_oracle_seed12345.txt (x, surface
// y, z, biome) was resolved by tools/java/ClimateOracle.java running vanilla's
// own RandomState.createClimateSampler + MultiNoiseBiomeSource, and our
// BiomeAt must return the same biome. This subsumes the old "underwater columns
// get ocean biomes" heuristic: vanilla picks cave families for deep ocean
// floors (the depth parameter at a y=6 surface quart lands in the cave span),
// and a heuristic list would have rejected the correct answer.
func TestBiomeSelectionMatchesVanilla(t *testing.T) {
	data, err := os.ReadFile("testdata/biome_oracle_seed12345.txt")
	if err != nil {
		t.Fatal(err)
	}
	g := NewGenerator(12345)
	checked, mismatched := 0, 0
	families := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 4 {
			t.Fatalf("bad pin line: %q", line)
		}
		x, _ := strconv.Atoi(fields[0])
		y, _ := strconv.Atoi(fields[1])
		z, _ := strconv.Atoi(fields[2])
		want := fields[3]
		got := g.BiomeAt(x, z)
		checked++
		families[want]++
		if got != want {
			mismatched++
			if mismatched <= 10 {
				t.Errorf("column (%d,%d) surface y=%d: ours=%s vanilla=%s", x, z, y, got, want)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no pins checked")
	}
	if mismatched > 0 {
		t.Errorf("%d/%d columns disagree with vanilla", mismatched, checked)
	}
	// The oracle sample must stay diverse; a single-biome result would mean
	// the selection collapsed onto one parameter entry.
	if len(families) < 4 {
		t.Errorf("oracle sample only covers %d biome families: %v", len(families), families)
	}
	t.Logf("%d columns checked, %d biome families: %v", checked, len(families), families)
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
