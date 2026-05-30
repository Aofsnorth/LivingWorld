package terrain

import (
	"testing"

	"livingworld/internal/worldgen/biome"
	"livingworld/internal/worldgen/noise"
)

func TestBuildDeterministic(t *testing.T) {
	a, b := Build(1234, 2, -3).Blocks(), Build(1234, 2, -3).Blocks()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("same seed/chunk diverged at %d: %q != %q", i, a[i], b[i])
		}
	}
}

func TestBuildSeedVaries(t *testing.T) {
	a, b := Build(1, 0, 0).Blocks(), Build(2, 0, 0).Blocks()
	for i := range a {
		if a[i] != b[i] {
			return // found a difference: good
		}
	}
	t.Fatal("different seeds produced identical chunk")
}

func TestBedrockFloorEverywhere(t *testing.T) {
	buf := Build(7, 0, 0)
	for z := 0; z < Size; z++ {
		for x := 0; x < Size; x++ {
			if got := buf.Get(x, MinY, z); got != "minecraft:bedrock" {
				t.Fatalf("floor at (%d,%d) = %q, want bedrock", x, z, got)
			}
		}
	}
}

func TestApplySurfaceRule(t *testing.T) {
	const h = 40 // below sea level
	var hm HeightMap
	var bm [Size * Size]biome.Biome
	for i := range hm {
		hm[i], bm[i] = h, biome.Plains
	}
	buf := NewBuffer()
	ApplySurface(buf, hm, bm)

	checks := []struct {
		y    int
		want string
	}{
		{MinY, "minecraft:bedrock"},
		{h, biome.Plains.Surface},
		{h - 1, biome.Plains.Filler},
		{h - 10, "minecraft:stone"},
		{SeaLevel, "minecraft:water"},
		{SeaLevel + 1, Air},
	}
	for _, c := range checks {
		if got := buf.Get(0, c.y, 0); got != c.want {
			t.Errorf("y=%d: got %q, want %q", c.y, got, c.want)
		}
	}
}

func TestCarvingProducesCaveAir(t *testing.T) {
	count := 0
	for cx := 0; cx < 4; cx++ {
		for cz := 0; cz < 4; cz++ {
			for _, name := range Build(99, cx, cz).Blocks() {
				if name == CaveAir {
					count++
				}
			}
		}
	}
	if count == 0 {
		t.Fatal("no cave_air carved across 16 chunks")
	}
}

func TestShapeHeightInBounds(t *testing.T) {
	hm, _ := ShapeHeight(noise.NewPerlin(3), NewClimate(3), 5, 5)
	for i, h := range hm {
		if h <= MinY || h >= MaxY {
			t.Fatalf("column %d height %d out of [%d,%d]", i, h, MinY, MaxY)
		}
	}
}
