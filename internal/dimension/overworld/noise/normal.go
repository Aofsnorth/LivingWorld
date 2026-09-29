package noise

import (
	"math"
	"strconv"
)

// NormalNoise is Mojang's net.minecraft.world.level.levelgen.synth.NormalNoise:
// the unit-ish-distribution noise the density / climate routers actually read.
//
// 26.3 changed the synthesis: a NormalNoise is a NoiseStack of 2×octaveCount
// gradient-noise layers, each with its own per-octave random stream
// (PositionalRandomFactory.fromHashOf("octave_<index>")) taken from TWO
// positional factories forked off the noise's own random, and its own
// frequency/amplitude pair. The value is the plain float sum of the layers —
// the old 1.18 "sum of two stacks × valueFactor" shape no longer exists.
//
// Legacy (pre-26.3) construction is still available for the old climate shim
// via NewNormalNoise; the density/climate router path uses NewNormalNoise26.
type NormalNoise struct {
	first, second   *PerlinNoise
	valueFactor     float64
	xzScale, yScale float64 // coordinate pre-scaling (1 for the faithful path)

	// layers is the 26.3 NoiseStack; when non-empty Sample uses it.
	layers []noiseLayer
}

// noiseLayer is one NoiseStack layer: a gradient-noise field, the frequency its
// input coordinates are scaled by, and the amplitude its output is scaled by.
type noiseLayer struct {
	n    *improvedNoise
	freq float64
	amp  float32
}

// normalInputFactor is Mojang's NormalNoise INPUT_FACTOR: the second layer of
// every octave samples its input at this multiple so the two decorrelate.
const normalInputFactor = 1.0181268882175227

// perlinStdDev is the constant NormalNoise.estimateDeviation folds into every
// octave amplitude before squaring: the standard deviation of one gradient
// noise octave on its unit lattice.
const perlinStdDev = 0.2702247831245211

// octaveInfo mirrors 26.3's NormalNoise$OctaveInfo record.
type octaveInfo struct {
	index     int
	frequency float64
	amplitude float64
}

// buildOctaves26 mirrors NormalNoise.buildOctaves. Frequency starts at
// 2^baseOctave and doubles per octave while the amplitude halves; each octave
// is scaled by its amplitude modifier (0 drops the octave entirely). The
// legacy-amplitude rescale is applied whenever normalization is enabled, which
// is the case for every vanilla noise.
func buildOctaves26(baseOctave int, baseAmplitude float64, octaveCount int, mods []float64) []octaveInfo {
	frequency := math.Pow(2, float64(baseOctave))
	amplitude := baseAmplitude
	// Normalize unless normalization is disabled; vanilla data never disables it.
	amplitude *= math.Pow(0.5, 1-float64(octaveCount)) / (math.Pow(0.5, -float64(octaveCount)) - 1)

	out := make([]octaveInfo, 0, octaveCount)
	for i := 0; i < octaveCount; i++ {
		mod := 1.0
		if len(mods) > 0 {
			mod = mods[i]
		}
		if mod != 0 {
			out = append(out, octaveInfo{index: baseOctave + i, frequency: frequency, amplitude: amplitude * mod})
		}
		frequency *= 2
		amplitude *= 0.5
	}
	return out
}

// NewNormalNoise26 builds a NormalNoise exactly the way 26.3's
// NormalNoise.create does: two positional factories forked from r, one layer
// pair per octave, each layer seeded from "octave_<index>" and scaled by the
// normalization factor.
func NewNormalNoise26(r *Xoroshiro, baseOctave int, baseAmplitude float64, octaveCount int, mods []float64) *NormalNoise {
	octaves := buildOctaves26(baseOctave, baseAmplitude, octaveCount, mods)

	var devSq, absSum float64
	for _, o := range octaves {
		dev := perlinStdDev * math.Abs(o.amplitude)
		devSq += dev * dev
		absSum += math.Abs(o.amplitude)
	}
	dev := math.Sqrt(devSq)
	normalization := 0.0
	if dev != 0 {
		normalization = (absSum * (1.0 / 3.0)) / (dev * math.Sqrt(2))
	}

	first := r.ForkPositional()
	second := r.ForkPositional()
	layers := make([]noiseLayer, 0, 2*len(octaves))
	for _, o := range octaves {
		seed := "octave_" + strconv.Itoa(o.index)
		amp := float32(normalization * o.amplitude)
		layers = append(layers,
			noiseLayer{n: newImprovedNoise(first.FromHashOf(seed)), freq: o.frequency, amp: amp},
			noiseLayer{n: newImprovedNoise(second.FromHashOf(seed)), freq: o.frequency * normalInputFactor, amp: amp},
		)
	}

	return &NormalNoise{layers: layers, valueFactor: 1, xzScale: 1, yScale: 1}
}

// NewNormalNoiseFromParams is the pre-26.3 (1.18-style) construction kept for
// callers that build a NormalNoise from a plain amplitude list: two octave
// stacks from one random, combined with a closed-form value factor.
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
	return &NormalNoise{
		first:  NewPerlinNoise(NewXoroshiro(uint64(firstSeed)), 0, amps),
		second: NewPerlinNoise(NewXoroshiro(uint64(secondSeed)), 0, amps),
		// single octave: expectedDeviation = 0.1*(1 + 1/1) = 0.2.
		valueFactor: (1.0 / 6.0) / 0.2,
		xzScale:     xzScale,
		yScale:      yScale,
	}
}

// Sample returns the normal-noise value at (x, y, z), in roughly [-1, 1].
// With the 26.3 layer stack the sum is accumulated in float32, matching
// NoiseStack.get's float arithmetic.
func (n *NormalNoise) Sample(x, y, z float64) float64 {
	if n.layers != nil {
		x, y, z = x*n.xzScale, y*n.yScale, z*n.xzScale
		var sum float32
		for i := range n.layers {
			l := &n.layers[i]
			v := float32(l.n.noise(x*l.freq, y*l.freq, z*l.freq))
			sum += l.amp * v
		}
		return float64(sum)
	}
	x, y, z = x*n.xzScale, y*n.yScale, z*n.xzScale
	return (n.first.GetValue(x, y, z) +
		n.second.GetValue(x*normalInputFactor, y*normalInputFactor, z*normalInputFactor)) * n.valueFactor
}

// Sample2D is the convenience wrapper for 2D climate fields (Y fixed at 0).
func (n *NormalNoise) Sample2D(x, z float64) float64 { return n.Sample(x, 0, z) }
