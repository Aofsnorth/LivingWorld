// Package climate is the multi-noise climate sampler the Overworld worldgen
// uses to pick a biome for every quart block of the chunk grid. It mirrors
// vanilla's net.minecraft.world.level.biome.OverworldBiomeBuilder and the
// six-axis MultiNoiseBiomeSource that fronts it.
//
// Pipeline:
//
//   1. WorldgenContext builds six NormalNoise instances at bootstrap
//      (temperature, humidity, continentalness, erosion, depth, weirdness).
//   2. For every (blockX, blockY, blockZ) the multi-noise sampler queries
//      the six fields and returns a ClimatePoint.
//   3. OverworldBiomeBuilder.GetBiome(ClimatePoint) scans the registry
//      (internal/dimension/overworld/biome) and returns the entry whose
//      ClimatePoint is the closest weighted match — exactly the algorithm
//      Mojang ships, with the same weights (T: 1, H: 1, C: 1, E: 1, D: 1,
//      W: 1, plus the "offset" delta that lifts special biomes above
//      background matches).
//
// The sampler is deterministic in (worldSeed, x, y, z); the builder is a
// pure function of the ClimatePoint. Two runs of the same seed always
// produce the same biome map.
package climate

import (
	"livingworld/internal/dimension/overworld/biome"
	"livingworld/internal/dimension/overworld/noise"
	"math"
)

// ClimatePoint is the 6D climate vector the builder scores against. We
// re-export biome.ClimatePoint so this package's signatures are short;
// they are byte-identical, so cross-package use is transparent.
type ClimatePoint = biome.ClimatePoint

// Sampler owns the six NormalNoise instances the multi-noise source
// queries. It is built once per world (the seed is fixed), and is safe
// for concurrent use: NormalNoise.Sample takes value-receivers and is
// stateless after construction.
type Sampler struct {
	temperature  *noise.NormalNoise
	humidity     *noise.NormalNoise
	continental  *noise.NormalNoise
	erosion      *noise.NormalNoise
	weirdness    *noise.NormalNoise
	// depth is 2D in modern Mojang (vertical is implied by quart Y);
	// we keep the 3D signature for source compatibility but route Y=0
	// for the surface path and Y=1 for the cave path through
	// SampleAtQuart, which then reuses depth = floorQuart.
	depth        *noise.NormalNoise
	// xzScale is the (1/quarter_size) sample scale Mojang uses. Vanilla
	// samples at blockX/4 (a 4-block cell) for the climate field; the
	// router interpolates between four corners per block.
	xzScale float64
}

// NewSampler builds the six-field sampler from per-axis seeds. The seeds
// match Mojang's MultiNoiseBiomeSource defaults; the caller (the
// WorldgenContext builder) passes the values it loaded from the
// noise_settings JSON.
func NewSampler(seeds AxisSeeds, xzScale float64) *Sampler {
	const factor = 1.0 // vanilla 2D fields use factor=1 (Perlin output is in [-1, 1] already)
	return &Sampler{
		temperature:  noise.NewNormalNoise(seeds.TemperatureFirst, seeds.TemperatureSecond, xzScale, 1, factor),
		humidity:     noise.NewNormalNoise(seeds.HumidityFirst, seeds.HumiditySecond, xzScale, 1, factor),
		continental:  noise.NewNormalNoise(seeds.ContinentalnessFirst, seeds.ContinentalnessSecond, xzScale, 1, factor),
		erosion:      noise.NewNormalNoise(seeds.ErosionFirst, seeds.ErosionSecond, xzScale, 1, factor),
		weirdness:    noise.NewNormalNoise(seeds.WeirdnessFirst, seeds.WeirdnessSecond, xzScale, 1, factor),
		depth:        noise.NewNormalNoise(seeds.DepthFirst, seeds.DepthSecond, xzScale, 1, factor),
		xzScale:      xzScale,
	}
}

// AxisSeeds is the per-axis Perlin seed pair. Vanilla reads these from
// the noise_settings.noise block of the datapack; we keep them in a
// struct so the WorldgenContext builder can pass them in one call.
type AxisSeeds struct {
	TemperatureFirst, TemperatureSecond      int64
	HumidityFirst, HumiditySecond            int64
	ContinentalnessFirst, ContinentalnessSecond int64
	ErosionFirst, ErosionSecond              int64
	WeirdnessFirst, WeirdnessSecond          int64
	DepthFirst, DepthSecond                  int64
}

// Sample returns the 6D climate point at world coordinates (x, y, z).
// The caller is responsible for picking the right Y — surface biomes
// pass y=64 (sea level), cave biomes pass y=-30 (cave domain).
func (s *Sampler) Sample(x, y, z int) ClimatePoint {
	return ClimatePoint{
		Temperature:    s.temperature.Sample(float64(x), float64(y), float64(z)),
		Humidity:       s.humidity.Sample(float64(x), float64(y), float64(z)),
		Continentalness: s.continental.Sample(float64(x), float64(y), float64(z)),
		Erosion:        s.erosion.Sample(float64(x), float64(y), float64(z)),
		Depth:          s.depth.Sample(float64(x), float64(y), float64(z)),
		Weirdness:      s.weirdness.Sample(float64(x), float64(y), float64(z)),
	}
}

// SampleSurface returns the climate at (x, seaLevel, z). Convenience used
// by the chunk pipeline when it builds the quart grid.
func (s *Sampler) SampleSurface(x, seaLevel, z int) ClimatePoint { return s.Sample(x, seaLevel, z) }

// QuartGrid is the 4×4×4 climate point cache the chunk pipeline keeps.
// The grid covers the 16×16 block column at 4-block resolution (4×4 cells
// per side). The pipeline interpolates between the 4 quart Y values for
// the per-block Y-clamp on the surface rules stage.
type QuartGrid [4][4]ClimatePoint

// BuildQuartGrid samples a 4×4 climate point grid for one chunk column.
// cx, cz are chunk coordinates; the grid covers world blocks
// (cx*16, cz*16) to (cx*16+15, cz*16+15) at 4-block resolution.
func (s *Sampler) BuildQuartGrid(cx, cz, seaLevel int) QuartGrid {
	var g QuartGrid
	for lz := 0; lz < 4; lz++ {
		for lx := 0; lx < 4; lx++ {
			wx := cx*16 + lx*4
			wz := cz*16 + lz*4
			g[lz][lx] = s.Sample(wx, seaLevel, wz)
		}
	}
	return g
}

// PickBiome returns the Parameters whose ClimatePoint is the closest
// weighted match to the given point. The distance metric mirrors
// OverworldBiomeBuilder.getBestBiome — for each candidate we compute
//
//	d = weight*sum_axis((sampled - target)^2) - offset
//
// and pick the smallest d. The vanilla weights for Overworld are
// (1, 1, 1, 1, 1, 1) and offsets come from per-biome "shifts" in the
// Mojang data-generator report.
//
// depth=1 routes the lookup to the cave sub-set; depth<0.5 picks the
// surface set. Mixing the two would let a cave biome win on the surface,
// which vanilla prevents with the depth gate.
func PickBiome(p ClimatePoint) biome.Parameters {
	return pickBiomeWithRegistry(p, biome.All())
}

// pickBiomeWithRegistry is the test-friendly variant of PickBiome.
func pickBiomeWithRegistry(p ClimatePoint, all []biome.Parameters) biome.Parameters {
	// Cave routing: if depth > 0.5, restrict to cave biomes.
	pool := all
	if p.Depth > 0.5 {
		var caves []biome.Parameters
		for _, b := range all {
			if b.CaveRules {
				caves = append(caves, b)
			}
		}
		if len(caves) > 0 {
			pool = caves
		}
	}
	best := pool[0]
	bestD := math.Inf(1)
	for _, b := range pool {
		dt := p.Temperature - b.ClimateTarget.Temperature
		dh := p.Humidity - b.ClimateTarget.Humidity
		dc := p.Continentalness - b.ClimateTarget.Continentalness
		de := p.Erosion - b.ClimateTarget.Erosion
		dw := p.Weirdness - b.ClimateTarget.Weirdness
		dd := p.Depth - b.ClimateTarget.Depth
		d := dt*dt + dh*dh + dc*dc + de*de + dw*dw + dd*dd
		if d < bestD {
			bestD, best = d, b
		}
	}
	return best
}
