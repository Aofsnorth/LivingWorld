package pipeline

import (
	"livingworld/internal/dimension/overworld/noise"
)

// octaveNoise is a fractal-Brownian-motion stack of Perlin samplers.
// Every octave gets its own permutation table AND its own coordinate
// offset. The offset is the load-bearing part: the previous generator
// sampled the climate field at integer lattice points, where Perlin
// noise is exactly zero, which collapsed the whole Overworld into one
// flat plain. Offsetting each octave by a seed-derived fraction keeps
// samples off the lattice at every frequency.
type octaveNoise struct {
	perlins []*noise.Perlin
	offX    []float64
	offY    []float64
	offZ    []float64
	freq    float64
	// inverse of the geometric amplitude sum, so output stays in ~[-1, 1]
	norm float64
}

// newOctaveNoise derives the per-octave seeds and offsets from
// (worldSeed, salt) with a Xoroshiro stream — deterministic per world.
func newOctaveNoise(worldSeed, salt int64, octaves int, freq float64) *octaveNoise {
	r := noise.NewXoroshiro(uint64(worldSeed) ^ uint64(salt)*0x9E3779B97F4A7C15)
	o := &octaveNoise{freq: freq}
	ampSum := 0.0
	amp := 1.0
	for i := 0; i < octaves; i++ {
		o.perlins = append(o.perlins, noise.NewPerlin(r.NextLong()))
		o.offX = append(o.offX, r.NextDouble()*256+0.33)
		o.offY = append(o.offY, r.NextDouble()*256+0.33)
		o.offZ = append(o.offZ, r.NextDouble()*256+0.33)
		ampSum += amp
		amp *= 0.5
	}
	o.norm = 1 / ampSum
	return o
}

// at2 samples 2D fBm at world coordinates (x, z). Output ~[-1, 1].
func (o *octaveNoise) at2(x, z float64) float64 {
	total := 0.0
	amp := 1.0
	f := o.freq
	for i, p := range o.perlins {
		total += p.Noise3D(x*f+o.offX[i], o.offY[i], z*f+o.offZ[i]) * amp
		amp *= 0.5
		f *= 2
	}
	return total * o.norm
}

// at3 samples 3D fBm at world coordinates (x, y, z). Output ~[-1, 1].
// yFreqScale stretches the Y frequency relative to XZ (caves want
// flatter, wider shapes, so they pass < 1).
func (o *octaveNoise) at3(x, y, z, yFreqScale float64) float64 {
	total := 0.0
	amp := 1.0
	f := o.freq
	for i, p := range o.perlins {
		total += p.Noise3D(x*f+o.offX[i], y*f*yFreqScale+o.offY[i], z*f+o.offZ[i]) * amp
		amp *= 0.5
		f *= 2
	}
	return total * o.norm
}

// hash01 is a cheap deterministic per-block hash in [0, 1). Used for the
// bedrock floor gradient, the stone/deepslate dither band, and other
// "random but stable" per-cell decisions that don't warrant a noise
// field.
func hash01(seed int64, x, y, z int) float64 {
	h := uint64(seed) ^ uint64(int64(x))*0x9E3779B97F4A7C15 ^ uint64(int64(y))*0xC2B2AE3D27D4EB4F ^ uint64(int64(z))*0x165667B19E3779F9
	h = noise.SplitMix64(h)
	return float64(h>>11) / float64(1<<53)
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampI(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// smooth is the smoothstep ease on t in [0, 1].
func smooth(t float64) float64 {
	t = clampF(t, 0, 1)
	return t * t * (3 - 2*t)
}

// spline is a piecewise-linear curve: for input v it interpolates
// between the surrounding (in, out) points. Inputs must be ascending.
type spline struct {
	in  []float64
	out []float64
}

func (s spline) at(v float64) float64 {
	if v <= s.in[0] {
		return s.out[0]
	}
	last := len(s.in) - 1
	if v >= s.in[last] {
		return s.out[last]
	}
	for i := 1; i <= last; i++ {
		if v < s.in[i] {
			t := (v - s.in[i-1]) / (s.in[i] - s.in[i-1])
			return s.out[i-1] + t*(s.out[i]-s.out[i-1])
		}
	}
	return s.out[last]
}
