package density

import (
	"math"

	"livingworld/internal/dimension/overworld/noise"
)

// BlendedNoise26 is 26.3's minecraft:old_blended_noise
// (net.minecraft.world.level.levelgen.synth.BlendedNoise). The algorithm was
// rewritten in 26.3: instead of three ImprovedNoise octave stacks with the y
// smear baked into the sampler, it is three NoiseStacks of SmearedPerlinNoise
// layers whose per-octave y smear ("fudge") is a property of the layer, and the
// three stacks are combined with a clamped lerp:
//
//	main  = mainStack(x*(xz/xzFactor), y*(yM/yFactor), z*(xz/xzFactor))
//	t     = clamp(main + 0.5, 0, 1)
//	value = lerp(t, minLimitStack, maxLimitStack)
//
// Every constant below was read from the 26.3 bytecode (BlendedNoise,
// SmearedPerlinNoise), and the whole node is pinned against the vanilla
// TreeOracle dump.
type BlendedNoise26 struct {
	MinLimit []noise.BlendedLayer
	MaxLimit []noise.BlendedLayer
	Main     []noise.BlendedLayer

	BaseScale float64 // 684.412
	// Effective multipliers, precomputed from the JSON parameters.
	XZMultiplier float64
	YMultiplier  float64
	XZFactor     float64
	YFactor      float64
}

// NewBlendedNoise26 builds the sampler set exactly as
// BlendedNoise.createFbmSet + compileSampler do, drawing every octave from the
// positional random in the same order.
func NewBlendedNoise26(r *noise.Xoroshiro, xzScale, yScale, xzFactor, yFactor, smearScaleMultiplier float64) BlendedNoise26 {
	const baseScale = 684.412
	xzMultiplier := baseScale * xzScale
	yMultiplier := baseScale * yScale

	smear := yMultiplier * smearScaleMultiplier
	mainScale := smear / yFactor

	// createFbmSet: minLimit, maxLimit share their parameters but draw
	// sequential streams from the same random, then the main stack.
	minLimit := noise.NewBlendedFbm26(r, -15, smear, 0.9999847412109375)
	maxLimit := noise.NewBlendedFbm26(r, -15, smear, 0.9999847412109375)
	main := noise.NewBlendedFbm26(r, -7, mainScale, 12.75)

	return BlendedNoise26{
		MinLimit:     minLimit,
		MaxLimit:     maxLimit,
		Main:         main,
		BaseScale:    baseScale,
		XZMultiplier: xzMultiplier,
		YMultiplier:  yMultiplier,
		XZFactor:     xzFactor,
		YFactor:      yFactor,
	}
}

// Eval samples the blended noise at a block position.
func (b BlendedNoise26) Eval(c Context, x, y, z int) float64 {
	fx, fy, fz := float64(x), float64(y), float64(z)

	minLimit := noise.SampleBlendedFbm26(b.MinLimit, fx*b.XZMultiplier, fy*b.YMultiplier, fz*b.XZMultiplier)
	maxLimit := noise.SampleBlendedFbm26(b.MaxLimit, fx*b.XZMultiplier, fy*b.YMultiplier, fz*b.XZMultiplier)
	main := noise.SampleBlendedFbm26(b.Main,
		fx*(b.XZMultiplier/b.XZFactor), fy*(b.YMultiplier/b.YFactor), fz*(b.XZMultiplier/b.XZFactor))

	t := main + 0.5
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return minLimit + t*(maxLimit-minLimit)
}

// Interpolated26 is 26.3's minecraft:interpolated
// (InterpolatedFunction): it evaluates the input at the eight corners of the
// (cellSizeXz × cellSizeY × cellSizeXz) cell and trilinearly interpolates. It
// replaces the 1.18-era "interpolated" the density tree used for its noise
// caches.
type Interpolated26 struct {
	A             Function
	CellSizeXz    int
	CellSizeY     int
	CellSizeXzInv float64
	CellSizeYInv  float64
}

// NewInterpolated26 precomputes the reciprocal cell sizes the 26.3 sampler
// carries, so Eval is division-free like the vanilla sampler.
func NewInterpolated26(a Function, cellXz, cellY int) *Interpolated26 {
	return &Interpolated26{
		A:             a,
		CellSizeXz:    cellXz,
		CellSizeY:     cellY,
		CellSizeXzInv: 1.0 / float64(cellXz),
		CellSizeYInv:  1.0 / float64(cellY),
	}
}

func (o *Interpolated26) Eval(c Context, x, y, z int) float64 {
	ox := floorMod(x, o.CellSizeXz)
	oy := floorMod(y, o.CellSizeY)
	oz := floorMod(z, o.CellSizeXz)

	x0, y0, z0 := x-ox, y-oy, z-oz
	tx := float64(ox) * o.CellSizeXzInv
	ty := float64(oy) * o.CellSizeYInv
	tz := float64(oz) * o.CellSizeXzInv

	v000 := o.A.Eval(c, x0, y0, z0)
	if ox == 0 && oy == 0 && oz == 0 {
		return v000
	}
	var v100, v010, v110, v001, v101, v011, v111 float64
	if ox > 0 {
		v100 = o.A.Eval(c, x0+o.CellSizeXz, y0, z0)
	}
	if oy > 0 {
		v010 = o.A.Eval(c, x0, y0+o.CellSizeY, z0)
		if ox > 0 {
			v110 = o.A.Eval(c, x0+o.CellSizeXz, y0+o.CellSizeY, z0)
		}
	}
	if oz > 0 {
		v001 = o.A.Eval(c, x0, y0, z0+o.CellSizeXz)
		if ox > 0 {
			v101 = o.A.Eval(c, x0+o.CellSizeXz, y0, z0+o.CellSizeXz)
		}
		if oy > 0 {
			v011 = o.A.Eval(c, x0, y0+o.CellSizeY, z0+o.CellSizeXz)
			if ox > 0 {
				v111 = o.A.Eval(c, x0+o.CellSizeXz, y0+o.CellSizeY, z0+o.CellSizeXz)
			}
		}
	}
	return lerp3(tx, ty, tz, v000, v100, v010, v110, v001, v101, v011, v111)
}

// floorMod mirrors Math.floorMod for a positive modulus.
func floorMod(a, m int) int {
	if m == 0 {
		return 0
	}
	r := a % m
	if r < 0 {
		r += m
	}
	return r
}

// clamp01 is ClampFunction.Sampler(.., 0f, 1f).
func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }
