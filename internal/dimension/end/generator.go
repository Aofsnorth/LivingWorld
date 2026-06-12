// Package end is the End dimension stub. The End is a single large
// island of end stone centred at (0, 0) in the dimension, surrounded
// by void. The full vanilla implementation (chorus trees, end cities,
// outer islands) is out of scope for v1; this stub produces a usable
// End that the world runtime can build chunks for.
package end

import (
	"livingworld/internal/world"
	"math"
)

// Dimension is the End chunk generator. It produces an end-stone
// surface near the centre of the dimension and a void everywhere
// else. The radius and surface-Y are the vanilla defaults.
type Dimension struct{ seed int64 }

// New returns an End dimension for the given world seed.
func New(seed int64) *Dimension { return &Dimension{seed: seed} }

// Constants for the End vertical range. Vanilla values.
const (
	MinY     = 0
	Height   = 256
	MaxY     = MinY + Height - 1 // 255
	SeaLevel = 0
	// islandRadiusBlocks is the radius of the central end-stone
	// island. Chunks inside this radius are filled with end stone
	// at sea level; chunks outside are void (air).
	islandRadiusBlocks = 1000
	// surfaceY is the Y at which the central island sits.
	surfaceY = 64
)

// Generate builds an End chunk. Inside the island radius the chunk
// is filled with end stone; outside it is air. Future work will
// replace this with the full perlin-driven island shape.
func (d *Dimension) Generate(cx, cz int) *world.Chunk {
	c := world.NewChunk()
	// Chunk centre in world coordinates.
	wcx, wcz := cx*16+8, cz*16+8
	dist := math.Sqrt(float64(wcx*wcx + wcz*wcz))
	inIsland := dist <= islandRadiusBlocks
	if !inIsland {
		// Pure void — every block is air.
		return c
	}
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			for y := 0; y < surfaceY; y++ {
				c.SetBlock(x, y, z, world.BlockByName("minecraft:end_stone"))
			}
			c.SetBlock(x, surfaceY, z, world.BlockByName("minecraft:end_stone"))
		}
	}
	return c
}
