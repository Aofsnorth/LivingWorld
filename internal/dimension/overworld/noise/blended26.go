package noise

import (
	"math"
)

// BlendedLayer is one layer of a 26.3 NoiseStack used by BlendedNoise
// (net.minecraft.world.level.levelgen.synth.SmearedPerlinNoise): a gradient
// noise field, the frequency its input coordinates are scaled by, the
// amplitude its output is scaled by, and the per-octave y-smear ("fudge").
type BlendedLayer struct {
	N    *improvedNoise
	Freq float64
	Amp  float32
	// Fudge is the y-smear quantum for this octave; zero disables smearing.
	Fudge float64
}

// NewBlendedFbm26 mirrors BlendedNoise.createFbm: -firstOctave+1 SmearedPerlin
// layers whose frequency halves and amplitude doubles each step, seeded
// sequentially from the same random so the octave streams match vanilla.
func NewBlendedFbm26(r *Xoroshiro, firstOctave int, scale, factor float64) []BlendedLayer {
	octaves := -firstOctave + 1
	freq := 1.0
	amp := factor / (math.Pow(2, float64(octaves)) - 1)

	layers := make([]BlendedLayer, 0, octaves)
	for i := 0; i < octaves; i++ {
		layers = append(layers, BlendedLayer{
			N:     newImprovedNoise(r),
			Freq:  freq,
			Amp:   float32(amp),
			Fudge: scale * freq,
		})
		freq /= 2
		amp *= 2
	}
	return layers
}

// SampleBlendedFbm26 sums the layers, scaling the input coordinates by each
// layer's frequency. SmearedPerlinNoise.get applies the y fudge.
func SampleBlendedFbm26(layers []BlendedLayer, x, y, z float64) float64 {
	var sum float32
	for i := range layers {
		l := &layers[i]
		sum += l.Amp * float32(l.N.Smeared(x*l.Freq, y*l.Freq, z*l.Freq, l.Fudge))
	}
	return float64(sum)
}

// SmearedSample26 exposes a BlendedLayer's value for oracle diffing.
func SmearedSample26(l *BlendedLayer, x, y, z float64) float32 {
	return float32(l.N.Smeared(x*l.Freq, y*l.Freq, z*l.Freq, l.Fudge))
}

// PlainSample26 exposes a BlendedLayer's value without the y smear.
func PlainSample26(l *BlendedLayer, x, y, z float64) float32 {
	return float32(l.N.noise(x*l.Freq, y*l.Freq, z*l.Freq))
}

// Perm exposes a permutation entry for oracle diffing.
func (n *improvedNoise) Perm(i int) int { return int(n.p[i&255]) }

// OffsetX exposes the lattice origin for oracle diffing.
func (n *improvedNoise) OffsetX() float64 { return n.xo }

// Smeared is SmearedPerlinNoise.get: the regular gradient-noise sample with the
// y coordinate snapped to a multiple of the fudge scale. The snap target is
// computeFudgeY(yInput, fy) = (fy >= 0 && fy < yInput) ? fy : yInput, so the
// smear is bounded by the un-offset y argument the sampler was called with —
// using the fractional part instead (the pre-26.3 ImprovedNoise rule) diverges
// on the lowest-frequency octave of every BlendedNoise stack.
func (n *improvedNoise) Smeared(x, y, z, fudge float64) float64 {
	dx := perlinWrap(x) + n.xo
	dy := perlinWrap(y) + n.yo
	dz := perlinWrap(z) + n.zo
	ix := int(math.Floor(dx))
	iy := int(math.Floor(dy))
	iz := int(math.Floor(dz))
	fx := dx - float64(ix)
	fy := dy - float64(iy)
	fz := dz - float64(iz)

	var y6 float64
	if fudge != 0 {
		t := fy
		if !(fy >= 0 && fy < y) {
			t = y
		}
		y6 = math.Floor(t/fudge+1.0000000116860974e-7) * fudge
	}
	return n.sampleAndLerp(ix, iy, iz, fx, fy-y6, fz, fy)
}

// LayerCount is a diagnostic helper mirroring the vanilla octave count.
func LayerCount(firstOctave int) int { return -firstOctave + 1 }
