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
	"math"

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

// Canyon is the surface canyon / ravine carver. Vanilla ravines are
// long, narrow, winding trenches carved by a path that meanders
// through 3D Perlin noise. This implementation models a single
// meandering path per chunk that snakes horizontally, carving an
// elliptical cross-section with depth-dependent width tapering.
type Canyon struct {
	MaxLength  int     // maximum path length in blocks
	MinWidth   float64 // minimum ravine half-width (surface)
	MaxWidth   float64 // maximum ravine half-width (surface)
	MinDepth   int     // minimum depth below surface
	MaxDepth   int     // maximum depth below surface
	FrequencyX float64 // horizontal noise frequency (meander)
	FrequencyY float64 // vertical noise frequency (depth variation)
	Tries      int     // number of canyon attempts per chunk
	Threshold  float64 // 0-1 probability gate per try (vanilla ~1/50)
}

func (c *Canyon) Name() string { return "minecraft:canyon" }

// Carve produces a list of cells to carve for the canyon/ravine.
// The algorithm:
//  1. Roll Tries attempts; each has a Threshold probability of firing.
//  2. For each attempt, pick a random start point (ax, surface, az).
//  3. Pick a random horizontal direction (angle).
//  4. Walk MaxLength steps along the direction, perturbing with noise
//     to create a meandering path.
//  5. At each step, carve an elliptical cross-section: wider at the
//     top, tapering toward the bottom, with depth varying by noise.
func (c *Canyon) Carve(seed int64, chunkX, chunkZ, minY, maxY int) []Shape {
	rng := noise.NewWorldgenRandom(seed ^ 0xCAFE_1234)
	rng.SetCarverSeed(seed, int32(chunkX*16), int32(chunkZ*16))
	perlin := noise.NewPerlin(rng.NextLong())

	var out []Shape
	surfaceY := maxY - 4 // approximate surface level

	for t := 0; t < c.Tries; t++ {
		// Probability gate: vanilla ravines are rare (~1 in 50 chunks).
		if rng.NextDouble() > c.Threshold {
			continue
		}

		// Start position within the chunk.
		ax := int(rng.NextIntBounded(16))
		az := int(rng.NextIntBounded(16))

		// Random direction (radians).
		angle := rng.NextDouble() * 2 * math.Pi
		dx := math.Cos(angle)
		dz := math.Sin(angle)

		// Ravine parameters for this attempt.
		width := c.MinWidth + rng.NextDouble()*(c.MaxWidth-c.MinWidth)
		depth := c.MinDepth + int(rng.NextIntBounded(int32(c.MaxDepth-c.MinDepth+1)))
		length := c.MaxLength/2 + int(rng.NextIntBounded(int32(c.MaxLength/2)))

		// Walk the path.
		for step := 0; step < length; step++ {
			// Perturb direction with noise for meandering.
			noiseVal := perlin.Noise2D(
				float64(chunkX*16+ax+int(dx*float64(step)))*c.FrequencyX,
				float64(chunkZ*16+az+int(dz*float64(step)))*c.FrequencyX,
			)
			angle += noiseVal * 0.15 // gentle meander

			dx = math.Cos(angle)
			dz = math.Sin(angle)

			// Current position.
			cx := ax + int(dx*float64(step))
			cz := az + int(dz*float64(step))

			// Skip if outside chunk bounds.
			if cx < 0 || cx > 15 || cz < 0 || cz > 15 {
				continue
			}

			// Depth varies with noise along the path.
			depthNoise := perlin.Noise2D(float64(step)*c.FrequencyY, float64(t)*13.7)
			localDepth := depth + int(depthNoise*4)

			// Width tapers with depth (wider at top, narrower at bottom).
			for dy := 0; dy < localDepth; dy++ {
				y := surfaceY - dy
				if y < minY || y > maxY {
					continue
				}
				// Taper factor: 1.0 at surface, 0.3 at bottom.
				taper := 1.0 - 0.7*float64(dy)/float64(localDepth)
				localWidth := width * taper

				// Carve an elliptical cross-section.
				hw := int(localWidth + 0.5)
				for wx := -hw; wx <= hw; wx++ {
					for wz := -hw; wz <= hw; wz++ {
						// Elliptical distance.
						fx := float64(wx) / localWidth
						fz := float64(wz) / localWidth
						if fx*fx+fz*fz > 1.0 {
							continue
						}
						px, pz := cx+wx, cz+wz
						if px < 0 || px > 15 || pz < 0 || pz > 15 {
							continue
						}
						// Lava at the very bottom.
						isLava := y < minY+12
						out = append(out, Shape{X: px, Y: y, Z: pz, Lava: isLava, Priority: 2})
					}
				}
			}
		}
	}
	return out
}

// NewOverworldCanyon returns the default canyon/ravine carver.
// Tuned to produce rare, meandering ravines similar to vanilla.
func NewOverworldCanyon() *Canyon {
	return &Canyon{
		MaxLength:  120,
		MinWidth:   2.0,
		MaxWidth:   5.0,
		MinDepth:   20,
		MaxDepth:   60,
		FrequencyX: 1.0 / 40.0,
		FrequencyY: 1.0 / 20.0,
		Tries:      2,
		Threshold:  0.02, // ~1 in 50 chunks
	}
}
