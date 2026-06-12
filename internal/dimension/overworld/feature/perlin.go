package feature

import "math"

// perlinLocal is a 3D Perlin gradient noise sampler. It's a local copy
// of the noise.Perlin type so the feature package can run the ore
// pass without depending on the noise package (the ore package owns
// the canonical one).
type perlinLocal struct {
	perm [512]int
}

func newPerlinLocal(seed int64) *perlinLocal {
	var p perlinLocal
	var base [256]int
	for i := range base {
		base[i] = i
	}
	// Fisher–Yates with a deterministic xorshift RNG.
	var state uint64 = uint64(seed)
	if state == 0 {
		state = 0x9E3779B97F4A7C15
	}
	intn := func(n int) int {
		state += 0x9E3779B97F4A7C15
		z := state
		z ^= z >> 30
		z *= 0xBF58476D1CE4E5B9
		z ^= z >> 27
		z *= 0x94D049BB133111EB
		z ^= z >> 31
		return int(z%uint64(n)) & (n - 1)
	}
	for i := 255; i > 0; i-- {
		j := intn(i + 1)
		base[i], base[j] = base[j], base[i]
	}
	for i := range p.perm {
		p.perm[i] = base[i&255]
	}
	return &p
}

func fade(t float64) float64       { return t * t * t * (t*(t*6-15) + 10) }
func lerp(t, a, b float64) float64 { return a + t*(b-a) }

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

func (p *perlinLocal) noise3(x, y, z float64) float64 {
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
