package pipeline

import (
	"sync"

	"livingworld/internal/dimension/overworld/biome"
	"livingworld/internal/dimension/overworld/vanilladata"
)

// The biome source mirrors vanilla 26.3's MultiNoiseBiomeSource exactly:
//
//   - the six climate router functions (temperature, vegetation→humidity,
//     continents, erosion, depth, ridges→weirdness — the mapping
//     NoiseRouter.createClimateSampler applies) are sampled at the quart
//     containing the column's terrain surface;
//   - the sample is quantized the way Climate.quantizeCoord does —
//     (long)(v * 10000f) after the samplers' float cast;
//   - every entry of the 7594-point parameter list dumped from
//     OverworldBiomeBuilder is scored with vanilla's fitness: the sum over
//     the six axes of the squared clamped distance to the entry's span,
//     plus the squared offset. The first minimal entry wins ties, matching
//     Climate.ParameterList.findValueBruteForce's declaration order.
//
// The decision is a pure function of (seed, quart, surface quart-Y), so it
// is memoized per quart — one entry per 4×4 columns and each distinct
// surface quart-Y within them.

// quantizeCoord mirrors Climate.quantizeCoord: (long)(v * 10000f), with the
// double→float cast the samplers apply before quantizing.
func quantizeCoord(v float64) int64 {
	return int64(float32(v) * float32(10000))
}

// climateTarget is the quantized six-axis Climate.TargetPoint.
type climateTarget struct {
	t, h, c, e, d, w int64
}

// biomeDecision is one memoized quart decision: the selected biome plus the
// raw climate values (the column keeps them for decoration heuristics like
// ice cover and snow layers).
type biomeDecision struct {
	biome biome.Parameters
	temp  float64
	humid float64
	cont  float64
	eros  float64
}

// biomeMemo memoizes biome decisions per (quartX, surfaceQuartY, quartZ).
// Bounded; entries are dropped wholesale past the cap, which is safe
// because the decision is a pure function of the key and the world seed.
type biomeMemo struct {
	mu sync.Mutex
	m  map[[3]int]biomeDecision
}

// quartDecision selects the biome for the column at (wx, wz) whose terrain
// top is top, sampling the climate at the containing quart.
func (g *Generator) quartDecision(wx, top, wz int) biomeDecision {
	qy := top >> 2
	key := [3]int{wx >> 2, qy, wz >> 2}
	g.biomes.mu.Lock()
	if d, ok := g.biomes.m[key]; ok {
		g.biomes.mu.Unlock()
		return d
	}
	g.biomes.mu.Unlock()

	x, y, z := key[0]<<2, qy<<2, key[2]<<2
	dc := g.climateTmpl.Fresh()
	f := g.climateF
	temp := f.Temperature.Eval(dc, x, y, z)
	humid := f.Humidity.Eval(dc, x, y, z)
	cont := f.Continentalness.Eval(dc, x, y, z)
	eros := f.Erosion.Eval(dc, x, y, z)
	depth := f.Depth.Eval(dc, x, y, z)
	weird := f.Weirdness.Eval(dc, x, y, z)

	d := biomeDecision{
		biome: selectBiome(climateTarget{
			t: quantizeCoord(temp),
			h: quantizeCoord(humid),
			c: quantizeCoord(cont),
			e: quantizeCoord(eros),
			d: quantizeCoord(depth),
			w: quantizeCoord(weird),
		}),
		temp:  temp,
		humid: humid,
		cont:  cont,
		eros:  eros,
	}

	g.biomes.mu.Lock()
	if len(g.biomes.m) > 4096 {
		g.biomes.m = make(map[[3]int]biomeDecision, 1024)
	}
	g.biomes.m[key] = d
	g.biomes.mu.Unlock()
	return d
}

// selectBiome scans the vanilla parameter list for the minimal-fitness
// entry. Partial sums that already reach the current best prune the entry
// (safe: every remaining term is non-negative).
func selectBiome(q climateTarget) biome.Parameters {
	vals := [6]int64{q.t, q.h, q.c, q.e, q.d, q.w}
	best := int64(1) << 62
	bestIdx := 0
	for i := range vanilladata.BiomeParamAxes {
		ax := &vanilladata.BiomeParamAxes[i]
		fit := int64(0)
		for a := 0; a < 6; a++ {
			v := vals[a]
			mn, mx := int64(ax[2*a]), int64(ax[2*a+1])
			var d int64
			switch {
			case v > mx:
				d = v - mx
			case v < mn:
				d = mn - v
			}
			fit += d * d
			if fit >= best {
				break
			}
		}
		if fit >= best {
			continue
		}
		if o := int64(vanilladata.BiomeParamOffset[i]); o != 0 {
			fit += o * o
			if fit >= best {
				continue
			}
		}
		best, bestIdx = fit, i
	}
	return biome.ByID(vanilladata.BiomeParamNames[vanilladata.BiomeParamBiome[bestIdx]])
}
