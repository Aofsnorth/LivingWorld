package worldgen

import (
	"testing"

	"livingworld/internal/world"
	"livingworld/internal/worldgen/terrain"
)

const testSeed int64 = 0x5EED

// TestGenerateDeterministic: same seed + coords yields identical blocks.
func TestGenerateDeterministic(t *testing.T) {
	g := NewGenerator(testSeed)
	a, b := g.Generate(2, -3), g.Generate(2, -3)
	for y := 0; y < terrain.Height; y++ {
		for z := 0; z < terrain.Size; z++ {
			for x := 0; x < terrain.Size; x++ {
				if a.GetBlock(x, y, z).ID() != b.GetBlock(x, y, z).ID() {
					t.Fatalf("non-deterministic block at (%d,%d,%d)", x, y, z)
				}
			}
		}
	}
}

// TestGenerateBedrockFloor: the bottom column (terrain MinY -> chunk Y 0) is all bedrock.
func TestGenerateBedrockFloor(t *testing.T) {
	c := NewGenerator(testSeed).Generate(0, 0)
	bedrock := world.StateID("minecraft:bedrock")
	for z := 0; z < terrain.Size; z++ {
		for x := 0; x < terrain.Size; x++ {
			if got := c.GetBlock(x, 0, z).ID(); got != bedrock {
				t.Fatalf("expected bedrock at chunk-Y 0 (%d,%d), got %d", x, z, got)
			}
		}
	}
}

// TestGenerateSurfaceAndAirTop: the chunk has solid blocks, and its top row is air.
func TestGenerateSurfaceAndAirTop(t *testing.T) {
	c := NewGenerator(testSeed).Generate(0, 0)
	nonAir := 0
	for y := 0; y < terrain.Height; y++ {
		for z := 0; z < terrain.Size; z++ {
			for x := 0; x < terrain.Size; x++ {
				if c.GetBlock(x, y, z).ID() != world.AirID {
					nonAir++
				}
			}
		}
	}
	if nonAir == 0 {
		t.Fatal("generated chunk is entirely air")
	}
	top := terrain.Height - 1 // chunk-Y 383 (world-Y MaxY) is far above terrain
	for z := 0; z < terrain.Size; z++ {
		for x := 0; x < terrain.Size; x++ {
			if got := c.GetBlock(x, top, z).ID(); got != world.AirID {
				t.Fatalf("expected air at chunk top (%d,%d), got %d", x, z, got)
			}
		}
	}
}

// TestGeneratorDrivesWorld: the generator plugs into *world.World via SetGenerator
// and LoadChunk returns a populated chunk (runtime check of the structural interface).
func TestGeneratorDrivesWorld(t *testing.T) {
	w := world.NewWorld("gen-test")
	w.SetGenerator(NewGenerator(testSeed))
	c := w.LoadChunk(0, 0)
	if c == nil {
		t.Fatal("LoadChunk returned nil")
	}
	if c.GetBlock(0, 0, 0).ID() != world.StateID("minecraft:bedrock") {
		t.Fatal("world-generated chunk missing bedrock floor")
	}
}
