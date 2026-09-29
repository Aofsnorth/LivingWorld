package density

import "livingworld/internal/dimension/overworld/noise"

// RangeChoice picks between WhenInRange and WhenOutOfRange based on Input.
// The choice is Input >= Min && Input < Max.
type RangeChoice struct {
	Input          Function
	Min, Max       float64
	WhenInRange    Function
	WhenOutOfRange Function
}

func (o RangeChoice) Eval(c Context, x, y, z int) float64 {
	v := o.Input.Eval(c, x, y, z)
	if v >= o.Min && v < o.Max {
		if o.WhenInRange != nil {
			return o.WhenInRange.Eval(c, x, y, z)
		}
		return 0
	}
	if o.WhenOutOfRange != nil {
		return o.WhenOutOfRange.Eval(c, x, y, z)
	}
	return 0
}

// Noise samples a named NormalNoise from the context's noise set. The parser
// pre-resolves Sampler from the context so the hot path skips the name map
// lookup.
type Noise struct {
	Noise   string
	Sampler *noise.NormalNoise
	XZScale float64
	YScale  float64
}

func (o Noise) Eval(c Context, x, y, z int) float64 {
	n := o.Sampler
	if n == nil {
		n = c.Noises[o.Noise]
		if n == nil {
			return 0
		}
	}
	fx := float64(x) * o.XZScale
	fy := float64(y) * o.YScale
	fz := float64(z) * o.XZScale
	return n.Sample(fx, fy, fz)
}

// ShiftedNoise samples a named NormalNoise with a coordinate shift.
// shift_x / shift_y / shift_z are density functions whose values are added
// to the scaled coordinates before sampling.
type ShiftedNoise struct {
	Noise                  string
	Sampler                *noise.NormalNoise
	ShiftX, ShiftY, ShiftZ Function
	XZScale, YScale        float64
}

func (o ShiftedNoise) Eval(c Context, x, y, z int) float64 {
	n := o.Sampler
	if n == nil {
		n = c.Noises[o.Noise]
		if n == nil {
			return 0
		}
	}
	dx := o.ShiftX.Eval(c, x, y, z)
	dy := o.ShiftY.Eval(c, x, y, z)
	dz := o.ShiftZ.Eval(c, x, y, z)
	fx := float64(x)*o.XZScale + dx
	fy := float64(y)*o.YScale + dy
	fz := float64(z)*o.XZScale + dz
	return n.Sample(fx, fy, fz)
}

// ShiftA samples the named noise at (x/4, 0, z/4) and multiplies by 4.
type ShiftA struct {
	Noise   string
	Sampler *noise.NormalNoise
}

func (o ShiftA) Eval(c Context, x, y, z int) float64 {
	n := o.Sampler
	if n == nil {
		n = c.Noises[o.Noise]
		if n == nil {
			return 0
		}
	}
	return n.Sample(float64(x)/4, 0, float64(z)/4) * 4
}

// ShiftB samples the named noise at (z/4, x/4, 0) and multiplies by 4.
type ShiftB struct {
	Noise   string
	Sampler *noise.NormalNoise
}

func (o ShiftB) Eval(c Context, x, y, z int) float64 {
	n := o.Sampler
	if n == nil {
		n = c.Noises[o.Noise]
		if n == nil {
			return 0
		}
	}
	return n.Sample(float64(z)/4, float64(x)/4, 0) * 4
}

// Y returns the block Y coordinate as a density value.
type Y struct{}

func (Y) Eval(c Context, x, y, z int) float64 { return float64(y) }

// YClampedGradient maps world Y from [FromY, ToY] to [FromValue, ToValue]
// and clamps outside that range.
type YClampedGradient struct {
	FromY, ToY         int
	FromValue, ToValue float64
}

func (o YClampedGradient) Eval(c Context, x, y, z int) float64 {
	if y <= o.FromY {
		return o.FromValue
	}
	if y >= o.ToY {
		return o.ToValue
	}
	t := float64(y-o.FromY) / float64(o.ToY-o.FromY)
	return o.FromValue + t*(o.ToValue-o.FromValue)
}

// SplinePoint is one control point of a CubicSpline. Value can be a nested
// Spline or a Constant.
type SplinePoint struct {
	Location   float64
	Value      Function
	Derivative float64
}

// Spline is a multi-point cubic spline with C1 continuity. The input value
// selects the bracket and the interpolated value is computed with float32
// fidelity to match Java/deepslate. locs holds the float32-rounded knot
// locations, precomputed at parse time: the evaluation loop is hot and must
// not allocate.
type Spline struct {
	Input  Function
	Points []SplinePoint
	locs   []float64
}

func (o Spline) Eval(c Context, x, y, z int) float64 {
	coordinate := float64(fround(float32(o.Input.Eval(c, x, y, z))))
	n := len(o.Points)
	if n == 0 {
		return 0
	}

	locs := o.locs
	if len(locs) != n {
		// Parser always fills locs; tolerate hand-built splines in tests.
		locs = make([]float64, n)
		for i, p := range o.Points {
			locs[i] = float64(fround(float32(p.Location)))
		}
	}

	i := binarySearch(coordinate, locs) - 1
	last := n - 1

	if i < 0 {
		v := o.Points[0].Value.Eval(c, x, y, z)
		return float64(fround(float32(v + float64(fround(float32(o.Points[0].Derivative))*float32(coordinate-locs[0])))))
	}
	if i == last {
		v := o.Points[last].Value.Eval(c, x, y, z)
		return float64(fround(float32(v + float64(fround(float32(o.Points[last].Derivative))*float32(coordinate-locs[last])))))
	}

	loc0 := locs[i]
	loc1 := locs[i+1]
	der0 := o.Points[i].Derivative
	der1 := o.Points[i+1].Derivative
	f := float64(fround(float32(float64(fround(float32(coordinate-loc0))) / float64(fround(float32(loc1-loc0))))))
	val0 := o.Points[i].Value.Eval(c, x, y, z)
	val1 := o.Points[i+1].Value.Eval(c, x, y, z)

	f8 := float64(fround(float32(float64(fround(float32(der0)*float32(loc1-loc0))) - float64(fround(float32(val1-val0))))))
	f9 := float64(fround(float32(float64(fround(float32(-der1)*float32(loc1-loc0))) + float64(fround(float32(val1-val0))))))
	fl := floatLerp(float32(f), float32(val0), float32(val1))
	f10 := float64(fround(fl + fround(fround(float32(f)*fround(1.0-float32(f)))*floatLerp(float32(f), float32(f8), float32(f9)))))
	return f10
}

func fround(v float32) float32 { return v }

func floatLerp(t, a, b float32) float32 {
	diff := fround(b - a)
	prod := fround(t * diff)
	return fround(a + prod)
}

func binarySearch(coordinate float64, locations []float64) int {
	lo, hi := 0, len(locations)
	for lo < hi {
		mid := (lo + hi) / 2
		if coordinate < locations[mid] {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}
