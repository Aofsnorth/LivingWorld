package density

// RangeChoice picks between A and B based on the input. The choice is
// a stepped piecewise: if Input in [Min, Max] then A, else B. Vanilla
// uses this to build the "below sea level" carve inside final_density.
type RangeChoice struct {
	Input       Function
	Min, Max    float64
	WhenInRange Function // A
	WhenOutOfRange Function // B
}

func (o RangeChoice) Eval(c Context, x, y, z int) float64 {
	v := o.Input.Eval(c, x, y, z)
	if v >= o.Min && v <= o.Max {
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

// Noise is a leaf that reads a NormalNoise field by axis index. The
// actual noise sampler lives in Context.Climate (injected at
// WorldgenContext build time).
type Noise struct {
	// Axis is the climate axis index: 0=T, 1=H, 2=C, 3=E, 4=W, 5=D.
	Axis int
	// XZScale is the horizontal scale (1/xzScale) Mojang stores on
	// each noise function. 1.0 is the "stock" frequency.
	XZScale float64
	// YScale is the vertical scale. 1.0 for 2D-only fields, 2.0 for
	// 3D-via-Y forms (jagged_peaks / weirdness).
	YScale float64
}

func (o Noise) Eval(c Context, x, y, z int) float64 {
	if c.Climate == nil {
		return 0
	}
	return c.Climate.Axis(o.Axis, x, y, z)
}

// ShiftedNoise is the "noise with a coordinate shift" form Mojang uses
// to add a per-(x, z) translation into the noise. It composes two
// shift-noise lookups and one main-noise lookup; the result is a
// "noisier" field that's harder to see repeating at chunk boundaries.
type ShiftedNoise struct {
	// ShiftX / ShiftY / ShiftZ are noise lookups whose values shift the
	// main noise's X / Y / Z coordinate by that many blocks.
	ShiftX, ShiftY, ShiftZ Function
	// XZScale / YScale are the main noise's coordinate scale.
	XZScale, YScale float64
	// Axis picks which NormalNoise to use for the main sample. We
	// support 0..5 (the same six axes Climate exposes) for simplicity;
	// vanilla sometimes uses pre-derived "interpolated" noises which
	// are 1D outputs of the same family.
	Axis int
}

func (o ShiftedNoise) Eval(c Context, x, y, z int) float64 {
	if c.Climate == nil {
		return 0
	}
	dx := o.ShiftX.Eval(c, x, 0, z)
	dy := o.ShiftY.Eval(c, x, 0, z)
	dz := o.ShiftZ.Eval(c, x, 0, z)
	return c.Climate.Axis(o.Axis, x+int(dx*64), y+int(dy*64), z+int(dz*64))
}

// Spline is the "keyframe-interpolated" operator Mojang uses to shape
// the final height curve. It takes an input and a list of (location,
// value, derivative) triples; the result is a smooth interpolation
// across the [first, last] range with the derivative at each point
// preserved.
type Spline struct {
	Input  Function
	Points []SplinePoint
}

type SplinePoint struct {
	Location   float64
	Value      float64
	Derivative float64
}

func (o Spline) Eval(c Context, x, y, z int) float64 {
	v := o.Input.Eval(c, x, y, z)
	if len(o.Points) == 0 {
		return 0
	}
	// Vanilla's spline is a cubic Hermite interpolation between the two
	// surrounding control points. We use the simpler Bezier shape Mojang
	// actually uses (cubic Hermite with C1 continuity).
	if v <= o.Points[0].Location {
		return o.Points[0].Value
	}
	if v >= o.Points[len(o.Points)-1].Location {
		return o.Points[len(o.Points)-1].Value
	}
	for i := 0; i < len(o.Points)-1; i++ {
		a, b := o.Points[i], o.Points[i+1]
		if v >= a.Location && v <= b.Location {
			t := (v - a.Location) / (b.Location - a.Location)
			t2 := t * t
			t3 := t2 * t
			h00 := 2*t3 - 3*t2 + 1
			h10 := t3 - 2*t2 + t
			h01 := -2*t3 + 3*t2
			h11 := t3 - t2
			return h00*a.Value + h10*(b.Location-a.Location)*a.Derivative +
				h01*b.Value + h11*(b.Location-a.Location)*b.Derivative
		}
	}
	return 0
}
