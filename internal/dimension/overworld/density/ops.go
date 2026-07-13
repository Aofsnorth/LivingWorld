package density

import (
	"math"

	"livingworld/internal/dimension/overworld/noise"
)

// Constant is a leaf whose value is fixed. It is the only leaf that doesn't
// depend on coordinates.
type Constant struct{ Value float64 }

func (c Constant) Eval(Context, int, int, int) float64 { return c.Value }

// Abs is the absolute-value operator.
type Abs struct{ A Function }

func (o Abs) Eval(c Context, x, y, z int) float64 {
	return math.Abs(o.A.Eval(c, x, y, z))
}

// Square raises the input to the second power.
type Square struct{ A Function }

func (o Square) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	return v * v
}

// Cube is the signed cube.
type Cube struct{ A Function }

func (o Cube) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	return v * v * v
}

// HalfNegative returns A if A >= 0, else A/2.
type HalfNegative struct{ A Function }

func (o HalfNegative) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	if v < 0 {
		return v * 0.5
	}
	return v
}

// QuarterNegative is the stronger negative compressor: 1/4 below zero.
type QuarterNegative struct{ A Function }

func (o QuarterNegative) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	if v < 0 {
		return v * 0.25
	}
	return v
}

// Invert returns 1 / A.
type Invert struct{ A Function }

func (o Invert) Eval(c Context, x, y, z int) float64 { return 1.0 / o.A.Eval(c, x, y, z) }

// Squeeze first clamps the input to [-1, 1] then transforms it with
// x/2 - x*x*x/24.
type Squeeze struct{ A Function }

func (o Squeeze) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	if v < -1 {
		v = -1
	} else if v > 1 {
		v = 1
	}
	return v/2 - v*v*v/24
}

// Add returns A + B.
type Add struct{ A, B Function }

func (o Add) Eval(c Context, x, y, z int) float64 {
	return o.A.Eval(c, x, y, z) + o.B.Eval(c, x, y, z)
}

// Mul returns A * B.
type Mul struct{ A, B Function }

func (o Mul) Eval(c Context, x, y, z int) float64 {
	return o.A.Eval(c, x, y, z) * o.B.Eval(c, x, y, z)
}

// Min returns min(A, B).
type Min struct{ A, B Function }

func (o Min) Eval(c Context, x, y, z int) float64 {
	return math.Min(o.A.Eval(c, x, y, z), o.B.Eval(c, x, y, z))
}

// Max returns max(A, B).
type Max struct{ A, B Function }

func (o Max) Eval(c Context, x, y, z int) float64 {
	return math.Max(o.A.Eval(c, x, y, z), o.B.Eval(c, x, y, z))
}

// Clamp clamps the input to [Min, Max].
type Clamp struct {
	A       Function
	Min     float64
	Max     float64
}

func (o Clamp) Eval(c Context, x, y, z int) float64 {
	return math.Min(math.Max(o.A.Eval(c, x, y, z), o.Min), o.Max)
}

// BlendDensity is a no-op wrapper used by the final density pipeline. The
// real blending happens in the chunk generator, not in the density tree.
type BlendDensity struct{ A Function }

func (o BlendDensity) Eval(c Context, x, y, z int) float64 {
	return o.A.Eval(c, x, y, z)
}

// BlendAlpha is vanilla's blend_alpha constant: 1.
type BlendAlpha struct{ Constant }

// BlendOffset is vanilla's blend_offset constant: 0.
type BlendOffset struct{ Constant }

// CacheAllInCell is a no-op in this evaluator. It is a vanilla hint that
// the wrapped function is queried once per cell, not once per block.
type CacheAllInCell struct{ A Function }

func (o CacheAllInCell) Eval(c Context, x, y, z int) float64 {
	return o.A.Eval(c, x, y, z)
}

// CacheOnce caches the wrapped function per (x, y, z).
type CacheOnce struct{ A Function }

func (o *CacheOnce) Eval(c Context, x, y, z int) float64 {
	return c.cached(o, x, y, z, func() float64 { return o.A.Eval(c, x, y, z) })
}

// Cache2D caches the wrapped function per (x, z) with y forced to 0.
type Cache2D struct{ A Function }

func (o *Cache2D) Eval(c Context, x, y, z int) float64 {
	return c.cached(o, x, 0, z, func() float64 { return o.A.Eval(c, x, 0, z) })
}

// FlatCache caches the wrapped function per 2D cell (x >> 2, z >> 2) at y=0.
type FlatCache struct{ A Function }

func (o *FlatCache) Eval(c Context, x, y, z int) float64 {
	x0 := x >> 2 << 2
	z0 := z >> 2 << 2
	return c.cached(o, x0, 0, z0, func() float64 { return o.A.Eval(c, x0, 0, z0) })
}

// Interpolated samples the wrapped function at the corners of the cell
// (size CellWidth x CellHeight) and trilinearly interpolates.
type Interpolated struct {
	A                  Function
	CellWidth, CellHeight int
}

func (o *Interpolated) Eval(c Context, x, y, z int) float64 {
	w := o.CellWidth
	h := o.CellHeight
	if w <= 0 {
		w = 4
	}
	if h <= 0 {
		h = 8
	}

	firstX := floorTo(x, w)
	firstY := floorTo(y, h)
	firstZ := floorTo(z, w)

	tx := float64(x-firstX) / float64(w)
	ty := float64(y-firstY) / float64(h)
	tz := float64(z-firstZ) / float64(w)

	v000 := c.cached(o.A, firstX, firstY, firstZ, func() float64 { return o.A.Eval(c, firstX, firstY, firstZ) })
	v001 := c.cached(o.A, firstX, firstY, firstZ+w, func() float64 { return o.A.Eval(c, firstX, firstY, firstZ+w) })
	v010 := c.cached(o.A, firstX, firstY+h, firstZ, func() float64 { return o.A.Eval(c, firstX, firstY+h, firstZ) })
	v011 := c.cached(o.A, firstX, firstY+h, firstZ+w, func() float64 { return o.A.Eval(c, firstX, firstY+h, firstZ+w) })
	v100 := c.cached(o.A, firstX+w, firstY, firstZ, func() float64 { return o.A.Eval(c, firstX+w, firstY, firstZ) })
	v101 := c.cached(o.A, firstX+w, firstY, firstZ+w, func() float64 { return o.A.Eval(c, firstX+w, firstY, firstZ+w) })
	v110 := c.cached(o.A, firstX+w, firstY+h, firstZ, func() float64 { return o.A.Eval(c, firstX+w, firstY+h, firstZ) })
	v111 := c.cached(o.A, firstX+w, firstY+h, firstZ+w, func() float64 { return o.A.Eval(c, firstX+w, firstY+h, firstZ+w) })

	return lerp3(tx, ty, tz, v000, v100, v010, v110, v001, v101, v011, v111)
}

func floorTo(v, size int) int {
	if size == 0 {
		return v
	}
	if v >= 0 {
		return (v / size) * size
	}
	return -((-v + size - 1) / size) * size
}

func lerp3(tx, ty, tz, v000, v100, v010, v110, v001, v101, v011, v111 float64) float64 {
	x00 := lerp(tx, v000, v100)
	x01 := lerp(tx, v001, v101)
	x10 := lerp(tx, v010, v110)
	x11 := lerp(tx, v011, v111)
	y0 := lerp(ty, x00, x10)
	y1 := lerp(ty, x01, x11)
	return lerp(tz, y0, y1)
}

func lerp(t, a, b float64) float64 { return a + t*(b-a) }

// IntervalSelect selects one of several functions based on thresholds.
// If input < thresholds[0] then functions[0], else count how many thresholds
// are crossed and use functions[index].
type IntervalSelect struct {
	Input     Function
	Functions []Function
	Thresholds []float64
}

func (o *IntervalSelect) Eval(c Context, x, y, z int) float64 {
	v := o.Input.Eval(c, x, y, z)
	idx := 0
	for _, t := range o.Thresholds {
		if v >= t {
			idx++
		}
	}
	if idx >= len(o.Functions) {
		idx = len(o.Functions) - 1
	}
	return o.Functions[idx].Eval(c, x, y, z)
}

// OldBlendedNoise is minecraft:old_blended_noise, the legacy BlendedNoise
// sampler that is the backbone of the 3D terrain base.
type OldBlendedNoise struct {
	bn *noise.BlendedNoise
}

func (o OldBlendedNoise) Eval(c Context, x, y, z int) float64 {
	return o.bn.Sample(float64(x), float64(y), float64(z))
}

// EndIslands is the end dimension island noise. Overworld final_density never
// references it, so this is a placeholder until end generation is wired.
type EndIslands struct{}

func (EndIslands) Eval(Context, int, int, int) float64 { return 0 }

// FindTopSurface is the preliminary surface level function. Overworld
// final_density does not use it, so this is a placeholder.
type FindTopSurface struct{}

func (FindTopSurface) Eval(Context, int, int, int) float64 { return 0 }

// Beardifier is the structure beardifier function. Overworld final_density
// does not use it, so this is a placeholder.
type Beardifier struct{}

func (Beardifier) Eval(Context, int, int, int) float64 { return 0 }

// WeirdScaledSampler is a deprecated/special function not used by the
// Overworld final_density.
type WeirdScaledSampler struct{}

func (WeirdScaledSampler) Eval(Context, int, int, int) float64 { return 0 }

// Shift is a single-axis noise shift. It is not used in the Overworld preset
// (only shift_a / shift_b are), so it is treated as a zero placeholder.
type Shift struct{}

func (Shift) Eval(Context, int, int, int) float64 { return 0 }
