// Package nether generates the Nether dimension. The Nether has a
// vertical scale of 128 blocks (Y 0..127), a lava sea at Y=31,
// and a bedrock ceiling / floor. This generator uses 3D Perlin noise
// to carve caves and create varied terrain (lava pockets, air pockets,
// soul sand patches, gravel patches, glowstone ceiling clusters).
//
// The full vanilla implementation (soul sand valleys, crimson forest,
// warped forest, basalt deltas, fortresses, bastion remnants) is
// planned for a future phase; this produces a navigable Nether with
// natural-looking caves and pockets that match vanilla's feel.
package nether

import (
	"livingworld/internal/dimension/overworld/noise"
	"livingworld/internal/world"
)

// Dimension is the Nether chunk generator.
type Dimension struct {
	seed   int64
	perlin *noise.Perlin
	// secondPerlin is used for lava pocket / air pocket detection
	// at a different frequency for variety.
	secondPerlin *noise.Perlin
}

// New returns a Nether dimension for the given world seed.
func New(seed int64) *Dimension {
	rng := noise.NewWorldgenRandom(seed ^ 0x4E65_7468) // "Neth" XOR
	return &Dimension{
		seed:         seed,
		perlin:       noise.NewPerlin(rng.NextLong()),
		secondPerlin: noise.NewPerlin(rng.NextLong()),
	}
}

// Constants for the Nether vertical range. Vanilla values.
const (
	MinY     = 0
	Height   = 128
	MaxY     = MinY + Height - 1 // 127
	SeaLevel = 31
)

// Noise parameters for the Nether cave carving.
const (
	caveFreqX    = 1.0 / 16.0  // horizontal cave frequency
	caveFreqY    = 1.0 / 8.0   // vertical cave frequency
	caveFreqZ    = 1.0 / 16.0  // horizontal cave frequency
	caveThreshLo = -0.3         // low noise threshold for caves
	caveThreshHi = 0.3          // high noise threshold for caves
	pocketFreq   = 1.0 / 24.0   // frequency for lava/air pockets
	pocketThresh = 0.55          // threshold for pocket detection
)

// Generate builds a Nether chunk with noise-carved caves, lava pockets,
// and varied terrain.
func (d *Dimension) Generate(cx, cz int) *world.Chunk {
	c := world.NewChunk()

	// Pre-compute per-column ceiling height variation (bedrock isn't flat).
	ceilY := make([]int, 16*16)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			wx := float64(cx*16 + x)
			wz := float64(cz*16 + z)
			// Ceiling varies ±4 blocks around MaxY-4.
			ceilNoise := d.perlin.Noise2D(wx/40.0, wz/40.0)
			ceilY[x*16+z] = MaxY - 4 + int(ceilNoise*4)
			if ceilY[x*16+z] > MaxY {
				ceilY[x*16+z] = MaxY
			}
			if ceilY[x*16+z] < MaxY-8 {
				ceilY[x*16+z] = MaxY - 8
			}
		}
	}

	// Pre-compute per-column floor height variation.
	floorY := make([]int, 16*16)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			wx := float64(cx*16 + x)
			wz := float64(cz*16 + z)
			floorNoise := d.perlin.Noise2D(wx/32.0+1000, wz/32.0+1000)
			floorY[x*16+z] = 4 + int(floorNoise*3)
			if floorY[x*16+z] < 1 {
				floorY[x*16+z] = 1
			}
			if floorY[x*16+z] > 8 {
				floorY[x*16+z] = 8
			}
		}
	}

	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			wx := float64(cx*16 + x)
			wz := float64(cz*16 + z)
			fy := floorY[x*16+z]
			cy := ceilY[x*16+z]

			for y := MinY; y <= MaxY; y++ {
				block := d.netherBlockAt(x, y, z, wx, float64(y), wz, fy, cy)
				if block.ID() != 0 { // not air
					c.SetBlock(x, y, z, block)
				}
			}
		}
	}
	return c
}

// netherBlockAt determines what block goes at a given position in the Nether.
func (d *Dimension) netherBlockAt(lx, y, lz int, wx, wy, wz float64, floorY, ceilY int) world.Block {
	// --- Bedrock floor (Y 0..floorY) ---
	if y <= 0 {
		return world.BlockByName("minecraft:bedrock")
	}
	if y <= floorY {
		// Random bedrock in the floor transition zone.
		bedrockNoise := d.secondPerlin.Noise3D(wx*0.5, wy*0.5, wz*0.5)
		fraction := float64(y) / float64(floorY)
		if bedrockNoise > fraction-0.5 {
			return world.BlockByName("minecraft:bedrock")
		}
		return world.BlockByName("minecraft:netherrack")
	}

	// --- Bedrock ceiling (ceilY..MaxY) ---
	if y >= MaxY {
		return world.BlockByName("minecraft:bedrock")
	}
	if y >= ceilY {
		ceilFraction := float64(y-ceilY) / float64(MaxY-ceilY)
		ceilNoise := d.secondPerlin.Noise3D(wx*0.5+500, wy*0.5, wz*0.5+500)
		if ceilNoise > ceilFraction-0.5 {
			return world.BlockByName("minecraft:bedrock")
		}
		return world.BlockByName("minecraft:netherrack")
	}

	// --- Cave carving: 3D Perlin noise ---
	caveNoise := d.perlin.Noise3D(wx*caveFreqX, wy*caveFreqY, wz*caveFreqZ)
	if caveNoise > caveThreshLo && caveNoise < caveThreshHi {
		// This cell is inside a cave.
		// Below lava sea: fill with lava.
		if y <= SeaLevel {
			return world.BlockByName("minecraft:lava")
		}
		// Air cave.
		return world.BlockByID(0) // air
	}

	// --- Solid terrain ---
	// Lava pockets (small pockets of lava embedded in netherrack).
	if y > SeaLevel && y < SeaLevel+20 {
		pocketNoise := d.secondPerlin.Noise3D(wx*pocketFreq, wy*pocketFreq, wz*pocketFreq)
		if pocketNoise > pocketThresh {
			return world.BlockByName("minecraft:lava")
		}
	}

	// Soul sand patches near lava sea (Y 29..33).
	if y >= SeaLevel-2 && y <= SeaLevel+2 {
		soulNoise := d.perlin.Noise2D(wx/12.0, wz/12.0)
		if soulNoise > 0.4 {
			return world.BlockByName("minecraft:soul_sand")
		}
	}

	// Gravel patches (scattered throughout).
	if y > floorY+2 && y < ceilY-2 {
		gravelNoise := d.secondPerlin.Noise3D(wx/10.0+200, wy/6.0, wz/10.0+200)
		if gravelNoise > 0.6 {
			return world.BlockByName("minecraft:gravel")
		}
	}

	// Glowstone on the ceiling underside (stalactite-like clusters).
	if y == ceilY-1 {
		glowNoise := d.perlin.Noise2D(wx/8.0+300, wz/8.0+300)
		if glowNoise > 0.45 {
			return world.BlockByName("minecraft:glowstone")
		}
	}

	// Magma blocks near the bottom (Y floorY+1..floorY+3).
	if y <= floorY+3 && y > floorY {
		magmaNoise := d.secondPerlin.Noise2D(wx/6.0+400, wz/6.0+400)
		if magmaNoise > 0.5 {
			return world.BlockByName("minecraft:magma_block")
		}
	}

	// Default: netherrack.
	return world.BlockByName("minecraft:netherrack")
}
