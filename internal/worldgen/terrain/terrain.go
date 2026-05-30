package terrain

import (
	"livingworld/internal/worldgen/biome"
	"livingworld/internal/worldgen/noise"
)

// Seed-mixing constants give each noise field an independent-but-deterministic
// stream from one world seed.
const (
	saltTemp  = 0x9E3779B1
	saltHumid = 0x85EBCA77
	saltCave  = 0xC2B2AE3D
)

// Climate maps a world column to a biome via two low-frequency noise fields.
type Climate struct{ temp, humid *noise.Perlin }

// NewClimate builds a deterministic climate sampler for seed.
func NewClimate(seed int64) Climate {
	return Climate{temp: noise.NewPerlin(seed ^ saltTemp), humid: noise.NewPerlin(seed ^ saltHumid)}
}

const climateScale = 1.0 / 512.0

func unit(n float64) float64 { return (n + 1) / 2 } // [-1,1] -> [0,1]

// At returns the biome selected for world column (x,z).
func (c Climate) At(x, z int) biome.Biome {
	t := unit(c.temp.Noise2D(float64(x)*climateScale, float64(z)*climateScale))
	h := unit(c.humid.Noise2D(float64(x)*climateScale, float64(z)*climateScale))
	return biome.Select(t, h)
}

// HeightMap is the surface Y of each of the Size×Size columns (index z*Size+x).
type HeightMap [Size * Size]int

const terrainScale = 1.0 / 128.0

// ShapeHeight computes per-column surface height and biome for a chunk:
// height = biome.BaseHeight + fBm-noise × biome.Variation. Deterministic.
func ShapeHeight(surf *noise.Perlin, clim Climate, cx, cz int) (HeightMap, [Size * Size]biome.Biome) {
	var hm HeightMap
	var bm [Size * Size]biome.Biome
	for lz := 0; lz < Size; lz++ {
		for lx := 0; lx < Size; lx++ {
			wx, wz := cx*Size+lx, cz*Size+lz
			b := clim.At(wx, wz)
			n := surf.Octaves2D(float64(wx)*terrainScale, float64(wz)*terrainScale, 4, 0.5, 2.0)
			i := lz*Size + lx
			hm[i] = int(b.BaseHeight + n*b.Variation)
			bm[i] = b
		}
	}
	return hm, bm
}

// ApplySurface is the surface-rule stage: bedrock floor, stone interior, three
// filler layers, the biome surface block on top, and water filling submerged
// columns up to sea level.
func ApplySurface(buf *Buffer, hm HeightMap, bm [Size * Size]biome.Biome) {
	for lz := 0; lz < Size; lz++ {
		for lx := 0; lx < Size; lx++ {
			i := lz*Size + lx
			h, b := hm[i], bm[i]
			for y := MinY; y <= h; y++ {
				switch {
				case y == MinY:
					buf.Set(lx, y, lz, "minecraft:bedrock")
				case y == h:
					buf.Set(lx, y, lz, b.Surface)
				case y > h-4:
					buf.Set(lx, y, lz, b.Filler)
				default:
					buf.Set(lx, y, lz, "minecraft:stone")
				}
			}
			for y := h + 1; y <= SeaLevel; y++ {
				buf.Set(lx, y, lz, "minecraft:water")
			}
		}
	}
}

const (
	caveScale     = 1.0 / 24.0
	caveThreshold = 0.6
)

// Carve is the carver stage: subsurface cells whose 3D noise exceeds
// caveThreshold become CaveAir. Only solid columns are carved (water is left
// intact so oceans don't drain).
func Carve(buf *Buffer, cave *noise.Perlin, hm HeightMap, cx, cz int) {
	for lz := 0; lz < Size; lz++ {
		for lx := 0; lx < Size; lx++ {
			h := hm[lz*Size+lx]
			wx, wz := cx*Size+lx, cz*Size+lz
			for y := MinY + 1; y < h; y++ {
				d := cave.Noise3D(float64(wx)*caveScale, float64(y)*caveScale, float64(wz)*caveScale)
				if d > caveThreshold {
					buf.Set(lx, y, lz, CaveAir)
				}
			}
		}
	}
}

// Build runs the foundation-free terrain stages (height → surface → carve) for
// one chunk and returns the name-buffer. Materializing a *world.Chunk from this
// is the deferred worldgen→world glue (until internal/** unlocks).
func Build(seed int64, cx, cz int) *Buffer {
	surf := noise.NewPerlin(seed)
	cave := noise.NewPerlin(seed ^ saltCave)
	clim := NewClimate(seed)
	buf := NewBuffer()
	hm, bm := ShapeHeight(surf, clim, cx, cz)
	ApplySurface(buf, hm, bm)
	Carve(buf, cave, hm, cx, cz)
	return buf
}
