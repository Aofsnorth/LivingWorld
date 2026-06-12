// Package nether is the Nether dimension stub. The Nether has a
// vertical scale of 128 blocks (Y 0..127), a flat lava sea at Y=31,
// and netherrack everywhere else. The full vanilla implementation
// (soul sand valleys, crimson forest, warped forest, basalt deltas,
// fortresses, bastion remnants) is out of scope for v1; this stub
// produces a usable, navigable Nether that the world runtime can
// build chunks for.
package nether

import "livingworld/internal/world"

// Dimension is the Nether chunk generator. It produces a flat
// netherrack floor with a lava sea at sea level (Y=31) and a bedrock
// ceiling / floor.
type Dimension struct{ seed int64 }

// New returns a Nether dimension for the given world seed.
func New(seed int64) *Dimension { return &Dimension{seed: seed} }

// Constants for the Nether vertical range. Vanilla values.
const (
	MinY     = 0
	Height   = 128
	MaxY     = MinY + Height - 1 // 127
	SeaLevel = 31
)

// Generate builds a Nether chunk: bedrock floor (0..4), netherrack
// to the lava sea (5..30), lava sea (31), netherrack above (32..123),
// bedrock ceiling (124..127).
func (d *Dimension) Generate(cx, cz int) *world.Chunk {
	c := world.NewChunk()
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			// Floor: bedrock 0..4.
			for y := 0; y <= 4; y++ {
				c.SetBlock(x, y, z, world.BlockByName("minecraft:bedrock"))
			}
			// Netherrack layer 5..30.
			for y := 5; y < SeaLevel; y++ {
				c.SetBlock(x, y, z, world.BlockByName("minecraft:netherrack"))
			}
			// Lava sea 31.
			c.SetBlock(x, SeaLevel, z, world.BlockByName("minecraft:lava"))
			// Netherrack 32..123.
			for y := SeaLevel + 1; y <= MaxY-4; y++ {
				c.SetBlock(x, y, z, world.BlockByName("minecraft:netherrack"))
			}
			// Bedrock ceiling 124..127.
			for y := MaxY - 3; y <= MaxY; y++ {
				c.SetBlock(x, y, z, world.BlockByName("minecraft:bedrock"))
			}
		}
	}
	return c
}
