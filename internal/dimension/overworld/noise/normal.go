package noise

import "math"

// NormalNoise is Mojang's net.minecraft.world.level.levelgen.synth.NormalNoise:
// the unit-ish-distribution noise the density / climate routers actually read.
// It combines TWO PerlinNoise octave stacks — the second sampled at coordinates
// scaled by INPUT_FACTOR to decorrelate it from the first — and multiplies the
// sum by a valueFactor derived from the octave count so the output sits near
// [-1, 1].
type NormalNoise struct {
	first, second   *PerlinNoise
	valueFactor     float64
	xzScale, yScale float64 // coordinate pre-scaling (1 for the faithful path)
}

// normalInputFactor is Mojang's NormalNoise INPUT_FACTOR: the second sampler is
// queried at coordinates scaled by this so its octaves don't align with the
// first's.
const normalInputFactor = 1.0181268882175227

// NewNormalNoiseFromParams builds a NormalNoise from a random source and a noise
// parameter set (firstOctave + per-octave amplitudes), matching
// NormalNoise.create. The two PerlinNoise stacks are forked from the SAME source
// sequentially, so the second's octave streams follow the first's.
func NewNormalNoiseFromParams(r *Xoroshiro, firstOctave int, amplitudes []float64) *NormalNoise {
	first := NewPerlinNoise(r, firstOctave, amplitudes)
	second := NewPerlinNoise(r, firstOctave, amplitudes)

	minIdx, maxIdx := math.MaxInt, math.MinInt
	for i, a := range amplitudes {
		if a != 0 {
			if i < minIdx {
				minIdx = i
			}
			if i > maxIdx {
				maxIdx = i
			}
		}
	}
	expectedDeviation := 0.1 * (1.0 + 1.0/float64(maxIdx-minIdx+1))
	return &NormalNoise{
		first:       first,
		second:      second,
		valueFactor: (1.0 / 6.0) / expectedDeviation,
		xzScale:     1,
		yScale:      1,
	}
}

// NewNormalNoise is a compatibility shim for the (currently dead) climate
// sampler, which builds one NormalNoise per climate axis from a pair of seeds
// and an XZ scale. It constructs single-octave stacks; the multi-octave climate
// parameters land in Phase 3 when the real noise router is wired. factor (the
// old sqrt(3) clamp boundary) is no longer used — the valueFactor handles range.
func NewNormalNoise(firstSeed, secondSeed int64, xzScale, yScale, factor float64) *NormalNoise {
	amps := []float64{1.0}
	n := &NormalNoise{
		first:  NewPerlinNoise(NewXoroshiro(uint64(firstSeed)), 0, amps),
		second: NewPerlinNoise(NewXoroshiro(uint64(secondSeed)), 0, amps),
		// single octave: expectedDeviation = 0.1*(1 + 1/1) = 0.2.
		valueFactor: (1.0 / 6.0) / 0.2,
		xzScale:     xzScale,
		yScale:      yScale,
	}
	return n
}

// Sample returns the normal-noise value at (x, y, z), in roughly [-1, 1].
func (n *NormalNoise) Sample(x, y, z float64) float64 {
	x, y, z = x*n.xzScale, y*n.yScale, z*n.xzScale
	return (n.first.GetValue(x, y, z) +
		n.second.GetValue(x*normalInputFactor, y*normalInputFactor, z*normalInputFactor)) * n.valueFactor
}

// Sample2D is the convenience wrapper for 2D climate fields (Y fixed at 0).
func (n *NormalNoise) Sample2D(x, z float64) float64 { return n.Sample(x, 0, z) }
