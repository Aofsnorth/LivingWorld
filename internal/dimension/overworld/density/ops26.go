package density

import "math"

// Operators added for vanilla 26.3 schema parity. Every formula here was
// verified against the 26.3 server jar bytecode (net.minecraft.world.level
// .levelgen.densityfunction.op / generator), not guessed:
//
//	UnaryFunction$Type       abs|square|cube|sqrt|half_negative|quarter_negative|
//	                         log|negate|reciprocal|sign|squeeze, field "input"
//	BinaryFunction$Type      add|sub|mul|div|min|max, fields "left"/"right"
//	BinaryFunction$DivSampler        right==0 -> 0, else left/right
//	LerpFunction$Sampler     alpha==0 -> first, alpha==1 -> second,
//	                         else Mth.lerp(alpha, first, second)
//	SliceFunction$YSampler   input.sampleValue(x, this.y, z) — the axis
//	                         coordinate is pinned
//	GradientFunction         axis, tiling (default CLAMP_TO_EDGE),
//	                         from_coordinate/to_coordinate (must differ),
//	                         from_value/to_value

// Negate returns -A (26.3 "negate", UnaryFunction$NegateSampler: fneg).
type Negate struct{ A Function }

func (o Negate) Eval(c Context, x, y, z int) float64 { return -o.A.Eval(c, x, y, z) }

// Reciprocal returns 1/A (26.3 "reciprocal": fconst_1; fdiv).
type Reciprocal struct{ A Function }

func (o Reciprocal) Eval(c Context, x, y, z int) float64 { return 1.0 / o.A.Eval(c, x, y, z) }

// Sqrt returns sqrt(A) (26.3 "sqrt": Mth.sqrt, plain — negatives yield NaN).
type Sqrt struct{ A Function }

func (o Sqrt) Eval(c Context, x, y, z int) float64 { return math.Sqrt(o.A.Eval(c, x, y, z)) }

// Log returns ln(A) (26.3 "log": Math.log).
type Log struct{ A Function }

func (o Log) Eval(c Context, x, y, z int) float64 { return math.Log(o.A.Eval(c, x, y, z)) }

// Sign returns signum(A) (26.3 "sign": Math.signum).
type Sign struct{ A Function }

func (o Sign) Eval(c Context, x, y, z int) float64 {
	v := o.A.Eval(c, x, y, z)
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	default:
		return 0 // signum(0) = 0, signum(-0.0) = -0.0; both are zero on the wire
	}
}

// Sub returns A - B (26.3 binary "sub").
type Sub struct{ A, B Function }

func (o Sub) Eval(c Context, x, y, z int) float64 {
	return o.A.Eval(c, x, y, z) - o.B.Eval(c, x, y, z)
}

// Div returns A / B, or 0 when B evaluates to 0 (verified from
// BinaryFunction$DivSampler: the divisor is compared against 0 first).
type Div struct{ A, B Function }

func (o Div) Eval(c Context, x, y, z int) float64 {
	a := o.A.Eval(c, x, y, z)
	b := o.B.Eval(c, x, y, z)
	if b == 0 {
		return 0
	}
	return a / b
}

// Lerp interpolates between First and Second by Alpha (26.3 "lerp",
// fields alpha/first/second). Alpha is NOT clamped; the exact 0 and 1
// shortcuts from LerpFunction$Sampler are preserved so First/Second are not
// evaluated when the branch is fully taken.
type Lerp struct {
	Alpha, First, Second Function
}

func (o Lerp) Eval(c Context, x, y, z int) float64 {
	a := o.Alpha.Eval(c, x, y, z)
	if a == 0 {
		return o.First.Eval(c, x, y, z)
	}
	if a == 1 {
		return o.Second.Eval(c, x, y, z)
	}
	f := o.First.Eval(c, x, y, z)
	s := o.Second.Eval(c, x, y, z)
	return f + (s-f)*a
}

// Slice evaluates Input with the coordinate along Axis pinned to Coordinate
// (26.3 "slice", SliceFunction$YSampler/XSampler/ZSampler).
type Slice struct {
	Axis       byte // 'x', 'y' or 'z'
	Coordinate int
	Input      Function
}

func (o Slice) Eval(c Context, x, y, z int) float64 {
	switch o.Axis {
	case 'x':
		return o.Input.Eval(c, o.Coordinate, y, z)
	case 'z':
		return o.Input.Eval(c, x, y, o.Coordinate)
	default:
		return o.Input.Eval(c, x, o.Coordinate, z)
	}
}

// TilingMode mirrors net.minecraft.world.level.levelgen.densityfunction
// .TilingMode: how the gradient extrapolates outside [from, to].
type TilingMode byte

// Tiling modes, in the order of the vanilla enum (CLAMP_TO_EDGE default).
const (
	TilingClampToEdge TilingMode = iota
	TilingRepeat
	TilingMirroredRepeat
)

// DistanceToPoint is the 26.3 "distance_to_point" leaf
// (DistanceToPointFunction record: point, metric). Metrics follow
// DistanceMetric.compute: euclidean = sqrt(dx²+dy²+dz²),
// euclidean_squared = dx²+dy²+dz², manhattan = |dx|+|dy|+|dz|,
// chebyshev = max(|dx|,|dy|,|dz|).
type DistanceToPoint struct {
	PX, PY, PZ int
	Metric     byte // 'e' euclidean, 's' euclidean_squared, 'm' manhattan, 'c' chebyshev
}

func (o DistanceToPoint) Eval(c Context, x, y, z int) float64 {
	dx := float64(x - o.PX)
	dy := float64(y - o.PY)
	dz := float64(z - o.PZ)
	switch o.Metric {
	case 's':
		return dx*dx + dy*dy + dz*dz
	case 'm':
		return math.Abs(dx) + math.Abs(dy) + math.Abs(dz)
	case 'c':
		m := math.Abs(dx)
		if a := math.Abs(dy); a > m {
			m = a
		}
		if a := math.Abs(dz); a > m {
			m = a
		}
		return m
	default: // euclidean
		return math.Sqrt(dx*dx + dy*dy + dz*dz)
	}
}

// Gradient is the 26.3 "gradient" leaf: value ramps linearly from
// (FromCoordinate, FromValue) to (ToCoordinate, ToValue) along Axis.
// Vanilla rejects from_coordinate == to_coordinate at parse time.
type Gradient struct {
	Axis      byte // 'x', 'y' or 'z'
	Tiling    TilingMode
	FromCoord int
	ToCoord   int
	FromValue float64
	ToValue   float64
}

func (o Gradient) Eval(c Context, x, y, z int) float64 {
	var at int
	switch o.Axis {
	case 'x':
		at = x
	case 'z':
		at = z
	default:
		at = y
	}
	t := float64(at-o.FromCoord) / float64(o.ToCoord-o.FromCoord)
	switch o.Tiling {
	case TilingRepeat:
		t -= math.Floor(t)
	case TilingMirroredRepeat:
		t = math.Abs(t)
		t -= math.Floor(t/2) * 2
		if t > 1 {
			t = 2 - t
		}
	default: // TilingClampToEdge
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
	}
	return o.FromValue + (o.ToValue-o.FromValue)*t
}
