package noise

import "math"

// Perlin is a 2D/3D Perlin gradient-noise sampler with a seed-shuffled
// permutation table. It is the building block of the NormalNoise field
// sampler, and is also used directly for ore placement and small-scale
// climate perturbations.
//
// Output range is approximately [-1, 1]; the empirical distribution of
// |noise| peaks around 0.4, with rare excursions near 1. The implementation
// follows Ken Perlin's "improved noise" (2002): quintic fade function, the
// 12-edge gradient table, and the (a, b, c, d) lattice-sampling shape from
// the reference pseudocode.
type Perlin struct {
	perm [512]int
}

// NewPerlin builds a Perlin whose gradients are shuffled with a Xoroshiro
// stream derived from seed. Two runs of the same seed produce identical
// fields (this is the property the worldgen determinism contract depends
// on).
func NewPerlin(seed int64) *Perlin {
	var p Perlin
	var base [256]int
	for i := range base {
		base[i] = i
	}
	r := NewXoroshiro(uint64(seed))
	for i := 255; i > 0; i-- {
		j := int(r.NextIntBounded(int32(i + 1)))
		base[i], base[j] = base[j], base[i]
	}
	for i := range p.perm {
		p.perm[i] = base[i&255]
	}
	return &p
}

func fade(t float64) float64       { return t * t * t * (t*(t*6-15) + 10) }
func lerp(t, a, b float64) float64 { return a + t*(b-a) }

// grad3 is Ken Perlin's improved-noise gradient. Used for both 2D and 3D
// (the 2D case just passes z=0).
func grad3(h int, x, y, z float64) float64 {
	h &= 15
	u := x
	if h >= 8 {
		u = y
	}
	v := z
	if h < 4 {
		v = y
	} else if h == 12 || h == 14 {
		v = x
	}
	if h&1 != 0 {
		u = -u
	}
	if h&2 != 0 {
		v = -v
	}
	return u + v
}

// Noise2D samples 2D Perlin noise in roughly [-1, 1].
func (p *Perlin) Noise2D(x, y float64) float64 { return p.Noise3D(x, y, 0) }

// Noise3D samples 3D Perlin noise in roughly [-1, 1].
func (p *Perlin) Noise3D(x, y, z float64) float64 {
	xi := int(math.Floor(x)) & 255
	yi := int(math.Floor(y)) & 255
	zi := int(math.Floor(z)) & 255
	xf := x - math.Floor(x)
	yf := y - math.Floor(y)
	zf := z - math.Floor(z)
	u, v, w := fade(xf), fade(yf), fade(zf)

	a := p.perm[xi] + yi
	aa := p.perm[a] + zi
	ab := p.perm[a+1] + zi
	b := p.perm[xi+1] + yi
	ba := p.perm[b] + zi
	bb := p.perm[b+1] + zi

	x1 := lerp(u, grad3(p.perm[aa], xf, yf, zf), grad3(p.perm[ba], xf-1, yf, zf))
	x2 := lerp(u, grad3(p.perm[ab], xf, yf-1, zf), grad3(p.perm[bb], xf-1, yf-1, zf))
	y1 := lerp(v, x1, x2)
	x3 := lerp(u, grad3(p.perm[aa+1], xf, yf, zf-1), grad3(p.perm[ba+1], xf-1, yf, zf-1))
	x4 := lerp(u, grad3(p.perm[ab+1], xf, yf-1, zf-1), grad3(p.perm[bb+1], xf-1, yf-1, zf-1))
	y2 := lerp(v, x3, x4)
	return lerp(w, y1, y2)
}

// Octaves2D sums octaves layers of 2D Perlin noise (fractal Brownian
// motion), normalized to roughly [-1, 1]. persistence scales amplitude
// per octave; lacunarity scales frequency per octave. octaves < 1 returns
// 0. The reference contract is the same one used by Biome climate sampling.
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
