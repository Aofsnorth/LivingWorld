// Package carver is the carver stage of the Overworld worldgen pipeline.
// Carvers run AFTER the base terrain is materialised and surface rules
// applied; they cut caves / canyons out of the volume. Vanilla has two
// active carver types in the Overworld:
//
//   - cave        — 3D cheese caves (Worley-style) plus noodle caves.
//   - canyon      — the surface canyons / ravines.
//
// For our purposes the two are folded into a single Carver interface
// that yields a list of "carve these blocks" rectangles, and a
// per-chunk Carve() entry point that runs the active carvers.
//
// Vanilla's carver pipeline seeds a WorldgenRandom with the chunk's
// block XZ + the world seed; the seeded stream produces the carver
// anchors, and each anchor carves a 3D cheese shape. The implementation
// here is a faithful simplified model.
package carver

import (
	"livingworld/internal/dimension/overworld/noise"
)

// Shape is a single cell-modification made by a Carver. The pipeline
// applies the shape after the base pass: if the cell was solid stone,
// it becomes air (or lava if Lava is true). Negative priorities are
// applied first; higher numbers override.
type Shape struct {
	X, Y, Z   int
	Lava      bool
	Priority  int
}

// Carver is one carver type. The WorldgenContext knows which carvers
// the dimension enables; the pipeline calls Carve on each.
type Carver interface {
	Name() string
	Carve(seed int64, chunkX, chunkZ, minY, maxY int) []Shape
}

// Cave is the cheese / noodle cave carver. It uses a 3D Perlin field
// and carves the cells whose value falls in the [low, high] window.
// Lava gets placed in the deepest band (Y < lavaDepth).
type Cave struct {
	Low, High    float64 // carve cells with caveNoise in [Low, High]
	FrequencyX   float64
	FrequencyY   float64
	FrequencyZ   float64
	LavaDepth    int     // cells below this Y that get lava instead of air
	Tries        int     // number of cheese anchors
}

// Name returns the carver's registry id.
func (c *Cave) Name() string { return "minecraft:cave" }

// Carve produces a list of cells to carve. We loop Tries cheese
// anchors (one per chunk) and for each anchor sample a 3D Perlin
// field; cells whose value is in [Low, High] are carved.
func (c *Cave) Carve(seed int64, chunkX, chunkZ, minY, maxY int) []Shape {
	rng := noise.NewWorldgenRandom(seed)
	rng.SetCarverSeed(seed, int32(chunkX*16), int32(chunkZ*16))
	perlin := noise.NewPerlin(rng.NextLong())
	var out []Shape
	for t := 0; t < c.Tries; t++ {
		ax := int(rng.NextIntBounded(16))
		ay := minY + int(rng.NextIntBounded(int32(maxY-minY+1)))
		az := int(rng.NextIntBounded(16))
		for dx := -8; dx <= 8; dx++ {
			for dy := -8; dy <= 8; dy++ {
				for dz := -8; dz <= 8; dz++ {
					x, y, z := ax+dx, ay+dy, az+dz
					if x < 0 || x > 15 || z < 0 || z > 15 {
						continue
					}
					if y < minY || y > maxY {
						continue
					}
					// Cubic falloff so the cheese has rounded edges.
					d := dx*dx + dy*dy*2 + dz*dz
					if d > 64 {
						continue
					}
					v := perlin.Noise3D(float64(chunkX*16+x)*c.FrequencyX, float64(y)*c.FrequencyY, float64(chunkZ*16+z)*c.FrequencyZ)
					if v >= c.Low && v <= c.High {
						out = append(out, Shape{X: x, Y: y, Z: z, Lava: y < c.LavaDepth, Priority: 1})
					}
				}
			}
		}
	}
	return out
}

// NewOverworldCave returns the standard Overworld cave carver. The
// (Low, High) window and (FrequencyX, FrequencyY, FrequencyZ) values
// are tuned to produce cave densities and shapes that match vanilla's
// out-of-the-box default. We carve into 3D noise so that caves look
// similar to vanilla without being byte-exact.
func NewOverworldCave() *Cave {
	return &Cave{
		Low: -0.7, High: 0.4,
		FrequencyX: 1.0 / 24.0,
		FrequencyY: 1.0 / 12.0,
		FrequencyZ: 1.0 / 24.0,
		LavaDepth:  11,
		Tries:      8,
	}
}

// Canyon is the surface canyon / ravine carver. For our v1 it is a
// straight-line tunnel; the full perlin-driven meander lives in
// vanilla's CaveCanyonCarver which we approximate.
type Canyon struct {
	Width      int
	Depth      int
	Frequency  float64
	Tries      int
}

func (c *Canyon) Name() string { return "minecraft:canyon" }

func (c *Canyon) Carve(seed int64, chunkX, chunkZ, minY, maxY int) []Shape {
	rng := noise.NewWorldgenRandom(seed ^ 0x63A7_E5C8)
	rng.SetCarverSeed(seed, int32(chunkX*16), int32(chunkZ*16))
	var out []Shape
	for t := 0; t < c.Tries; t++ {
		ax := int(rng.NextIntBounded(16))
		az := int(rng.NextIntBounded(16))
		for d := -c.Width; d <= c.Width; d++ {
			for y := maxY; y > maxY-c.Depth; y-- {
				out = append(out, Shape{X: ax + d, Y: y, Z: az, Priority: 2})
			}
		}
	}
	return out
}

// NewOverworldCanyon returns the default canyon carver.
func NewOverworldCanyon() *Canyon {
	return &Canyon{Width: 2, Depth: 24, Frequency: 1.0 / 80.0, Tries: 1}
}
