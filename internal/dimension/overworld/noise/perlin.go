package noise

import (
	"math"
	"strconv"
)

// This file implements Mojang's improved-Perlin noise stack bit-faithfully:
//
//   - improvedNoise  : net.minecraft.world.level.levelgen.synth.ImprovedNoise —
//     a single gradient-noise octave with an RNG-drawn lattice origin and the
//     fixed 16-vector SimplexNoise.GRADIENT table.
//   - Perlin         : a thin public wrapper around one improvedNoise (kept for
//     the carver / ore / pipeline callers that want a single seeded field).
//   - PerlinNoise    : net.minecraft.world.level.levelgen.synth.PerlinNoise —
//     the octave stack, with per-octave streams forked positionally via
//     fromHashOf("octave_N") and the lowest-freq input/value factors.

// perlinGradient is SimplexNoise.GRADIENT — the 16 gradient vectors Mojang dots
// against (dx,dy,dz). Indexed by hash&15.
var perlinGradient = [16][3]float64{
	{1, 1, 0}, {-1, 1, 0}, {1, -1, 0}, {-1, -1, 0},
	{1, 0, 1}, {-1, 0, 1}, {1, 0, -1}, {-1, 0, -1},
	{0, 1, 1}, {0, -1, 1}, {0, 1, -1}, {0, -1, -1},
	{1, 1, 0}, {0, -1, 1}, {-1, 1, 0}, {0, -1, -1},
}

func gradDot(hash int, x, y, z float64) float64 {
	g := perlinGradient[hash&15]
	return g[0]*x + g[1]*y + g[2]*z
}

// smoothstep is Mth.smoothstep: 6t^5 - 15t^4 + 10t^3 (the quintic fade).
func smoothstep(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }

func lerp(t, a, b float64) float64 { return a + t*(b-a) }

func lerp2(tx, ty, d00, d10, d01, d11 float64) float64 {
	return lerp(ty, lerp(tx, d00, d10), lerp(tx, d01, d11))
}

func lerp3(tx, ty, tz, d000, d100, d010, d110, d001, d101, d011, d111 float64) float64 {
	return lerp(tz, lerp2(tx, ty, d000, d100, d010, d110), lerp2(tx, ty, d001, d101, d011, d111))
}

// improvedNoise is one octave of Mojang's ImprovedNoise.
type improvedNoise struct {
	xo, yo, zo float64
	p          [256]uint8
}

// newImprovedNoise builds an octave from a random source, drawing the lattice
// origin (xo,yo,zo) and the permutation table exactly as ImprovedNoise(RandomSource).
func newImprovedNoise(r PRNG) *improvedNoise {
	n := &improvedNoise{
		xo: r.NextDouble() * 256,
		yo: r.NextDouble() * 256,
		zo: r.NextDouble() * 256,
	}
	for i := range n.p {
		n.p[i] = uint8(i)
	}
	for i := 0; i < 256; i++ {
		j := int(r.NextIntBounded(int32(256 - i)))
		n.p[i], n.p[i+j] = n.p[i+j], n.p[i]
	}
	return n
}

func (n *improvedNoise) pAt(i int) int { return int(n.p[i&255]) }

// noise samples this octave at (x,y,z) in roughly [-1, 1].
func (n *improvedNoise) noise(x, y, z float64) float64 {
	dx := x + n.xo
	dy := y + n.yo
	dz := z + n.zo
	ix := int(math.Floor(dx))
	iy := int(math.Floor(dy))
	iz := int(math.Floor(dz))
	fx := dx - float64(ix)
	fy := dy - float64(iy)
	fz := dz - float64(iz)
	return n.sampleAndLerp(ix, iy, iz, fx, fy, fz)
}

func (n *improvedNoise) sampleAndLerp(gx, gy, gz int, dx, dy, dz float64) float64 {
	i := n.pAt(gx)
	j := n.pAt(gx + 1)
	k := n.pAt(i + gy)
	l := n.pAt(i + gy + 1)
	m := n.pAt(j + gy)
	nn := n.pAt(j + gy + 1)
	d := gradDot(n.pAt(k+gz), dx, dy, dz)
	e := gradDot(n.pAt(m+gz), dx-1, dy, dz)
	f := gradDot(n.pAt(l+gz), dx, dy-1, dz)
	g := gradDot(n.pAt(nn+gz), dx-1, dy-1, dz)
	h := gradDot(n.pAt(k+gz+1), dx, dy, dz-1)
	o := gradDot(n.pAt(m+gz+1), dx-1, dy, dz-1)
	q := gradDot(n.pAt(l+gz+1), dx, dy-1, dz-1)
	s := gradDot(n.pAt(nn+gz+1), dx-1, dy-1, dz-1)
	return lerp3(smoothstep(dx), smoothstep(dy), smoothstep(dz), d, e, f, g, h, o, q, s)
}

// Perlin is a single-octave gradient field used directly by the carver, ore,
// and pipeline samplers. Output is roughly [-1, 1].
type Perlin struct{ n *improvedNoise }

// NewPerlin builds a Perlin from a 64-bit seed (the RNG draws the lattice origin
// and permutation). Two runs of the same seed produce identical fields.
func NewPerlin(seed int64) *Perlin {
	return &Perlin{n: newImprovedNoise(NewXoroshiro(uint64(seed)))}
}

// Noise2D samples 2D Perlin noise in roughly [-1, 1].
func (p *Perlin) Noise2D(x, y float64) float64 { return p.n.noise(x, y, 0) }

// Noise3D samples 3D Perlin noise in roughly [-1, 1].
func (p *Perlin) Noise3D(x, y, z float64) float64 { return p.n.noise(x, y, z) }

// Octaves2D sums octaves layers of 2D noise (fractal Brownian motion),
// normalized to roughly [-1, 1]. persistence scales amplitude per octave;
// lacunarity scales frequency per octave.
func (p *Perlin) Octaves2D(x, y float64, octaves int, persistence, lacunarity float64) float64 {
	if octaves < 1 {
		return 0
	}
	var total, max float64
	amp, freq := 1.0, 1.0
	for i := 0; i < octaves; i++ {
		total += p.Noise2D(x*freq, y*freq) * amp
		max += amp
		amp *= persistence
		freq *= lacunarity
	}
	if max == 0 {
		return 0
	}
	return total / max
}

// PerlinNoise is Mojang's octave stack. octaves[i] is nil when amplitudes[i]==0.
type PerlinNoise struct {
	octaves               []*improvedNoise
	amplitudes            []float64
	lowestFreqInputFactor float64
	lowestFreqValueFactor float64
}

// NewPerlinNoise builds an octave stack matching PerlinNoise.create: each
// non-zero octave gets its own ImprovedNoise forked positionally from the
// source via fromHashOf("octave_<n>"). firstOctave is typically negative.
func NewPerlinNoise(r *Xoroshiro, firstOctave int, amplitudes []float64) *PerlinNoise {
	pn := &PerlinNoise{
		octaves:    make([]*improvedNoise, len(amplitudes)),
		amplitudes: amplitudes,
	}
	pf := r.ForkPositional()
	for i := range amplitudes {
		if amplitudes[i] != 0 {
			pn.octaves[i] = newImprovedNoise(pf.FromHashOf("octave_" + strconv.Itoa(firstOctave+i)))
		}
	}
	n := len(amplitudes)
	pn.lowestFreqInputFactor = math.Pow(2, float64(firstOctave))
	pn.lowestFreqValueFactor = math.Pow(2, float64(n-1)) / (math.Pow(2, float64(n)) - 1)
	return pn
}

// perlinWrap is PerlinNoise.wrap: keeps coordinates within a 2^25 window so the
// double lattice indices stay exact.
func perlinWrap(v float64) float64 {
	return v - math.Floor(v/3.3554432e7+0.5)*3.3554432e7
}

// GetValue samples the octave stack at (x,y,z).
func (pn *PerlinNoise) GetValue(x, y, z float64) float64 {
	var total float64
	inFactor := pn.lowestFreqInputFactor
	valFactor := pn.lowestFreqValueFactor
	for i, oct := range pn.octaves {
		if oct != nil {
			g := oct.noise(perlinWrap(x*inFactor), perlinWrap(y*inFactor), perlinWrap(z*inFactor))
			total += pn.amplitudes[i] * g * valFactor
		}
		inFactor *= 2
		valFactor /= 2
	}
	return total
}
