package density

// Constant is a leaf whose value is a fixed float64. It is the only leaf
// that doesn't depend on coordinates.
type Constant struct{ Value float64 }

func (c Constant) Eval(Context, int, int, int) float64 { return c.Value }

// Abs is the absolute-value operator. Vanilla uses it on a number of
// intermediate axes to symmetrise the field.
type Abs struct{ A Function }

func (o Abs) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	if v < 0 {
		return -v
	}
	return v
}

// Square raises the input to the second power (sign-preserving).
type Square struct{ A Function }

func (o Square) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	if v < 0 {
		return -v
	}
	return v * v
}

// Cube is the "signed cube": keeps the sign of A, scales the magnitude
// cubed. Vanilla uses this to amplify axial distortions without changing
// sign.
type Cube struct{ A Function }

func (o Cube) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	return v * v * v
}

// HalfNegative returns A if A >= 0, else A/2. Vanilla's "compress negative
// lobe" operator — the shape that flattens ocean basins without lifting
// the surface.
type HalfNegative struct{ A Function }

func (o HalfNegative) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	if v < 0 {
		return v * 0.5
	}
	return v
}

// QuarterNegative is the "stronger" version: 1/4 below zero.
type QuarterNegative struct{ A Function }

func (o QuarterNegative) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	if v < 0 {
		return v * 0.25
	}
	return v
}

// Invert returns 1 / A. Used for the "noise" transform Mojang uses to
// invert axes like temperature (so cold=high, hot=low) without losing
// resolution near the equator.
type Invert struct{ A Function }

func (o Invert) Eval(c Context, x, y, z int) float64 { return 1.0 / o.A.Eval(c, x, y, z) }

// Squeeze stretches the input into [-1, 1] by the sign-preserving
// transform (3A - 4A^3). It's a quick way to amplify small changes in
// the climate field without changing its sign.
type Squeeze struct{ A Function }

func (o Squeeze) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	return 3*v - 4*v*v*v
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
	a, b := o.A.Eval(c, x, y, z), o.B.Eval(c, x, y, z)
	if a < b {
		return a
	}
	return b
}

// Max returns max(A, B).
type Max struct{ A, B Function }

func (o Max) Eval(c Context, x, y, z int) float64 {
	a, b := o.A.Eval(c, x, y, z), o.B.Eval(c, x, y, z)
	if a > b {
		return a
	}
	return b
}

// Clamp clamps the input to [Min, Max].
type Clamp struct {
	A       Function
	Min     float64
	Max     float64
}

func (o Clamp) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	if v < o.Min {
		return o.Min
	}
	if v > o.Max {
		return o.Max
	}
	return v
}

// BlendAlpha returns A when input >= 0, B otherwise. Vanilla's "binary
// mask" — the cheap way to gate a sub-tree behind a sign test.
type BlendAlpha struct {
	A, B Function
}

func (o BlendAlpha) Eval(c Context, x, y, z int) float64 {
	if o.A.Eval(c, x, y, z) >= 0 {
		return o.B.Eval(c, x, y, z)
	}
	return 0
}

// BlendOffset returns A + max(B, 0). Vanilla uses it for "lift one field
// by another where the second field is non-negative".
type BlendOffset struct {
	A, B Function
}

func (o BlendOffset) Eval(c Context, x, y, z int) float64 {
	b := o.B.Eval(c, x, y, z)
	if b < 0 {
		b = 0
	}
	return o.A.Eval(c, x, y, z) + b
}
