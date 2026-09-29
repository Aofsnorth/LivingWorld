package density

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"livingworld/internal/dimension/overworld/noise"
	"livingworld/internal/dimension/overworld/vanilladata"
)

// NoiseSettings is the subset of a noise_settings object that the density
// parser needs while building the final_density tree.
type NoiseSettings struct {
	CellWidth, CellHeight int
	LegacyRandomSource    bool
}

// ParseFinalDensity parses a full noise_settings JSON and returns the
// compiled final_density tree. The supplied context provides the world seed
// and the noise set.
func ParseFinalDensity(ctx Context, raw json.RawMessage) (Function, error) {
	var top struct {
		LegacyRandomSource bool            `json:"legacy_random_source"`
		Noise              json.RawMessage `json:"noise"`
		NoiseRouter        json.RawMessage `json:"noise_router"`
	}
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, err
	}

	var noiseSub struct {
		MinY           int `json:"min_y"`
		Height         int `json:"height"`
		SizeHorizontal int `json:"size_horizontal"`
		SizeVertical   int `json:"size_vertical"`
	}
	if err := json.Unmarshal(top.Noise, &noiseSub); err != nil {
		return nil, err
	}

	var router struct {
		FinalDensity json.RawMessage `json:"final_density"`
	}
	if err := json.Unmarshal(top.NoiseRouter, &router); err != nil {
		return nil, err
	}

	settings := NoiseSettings{
		CellWidth:          noiseSub.SizeHorizontal * 4,
		CellHeight:         noiseSub.SizeVertical * 4,
		LegacyRandomSource: top.LegacyRandomSource,
	}

	p := parser{
		ctx:      ctx,
		settings: settings,
		cache:    make(map[string]Function),
	}
	return p.parse(router.FinalDensity)
}

// ParseNamed parses one embedded vanilla density function by its id (e.g.
// "overworld/ridges", "shift_x"). It exists for the tree-oracle diff harness
// and for callers that need a single function rather than the whole router.
func ParseNamed(ctx Context, name string) (Function, error) {
	raw, ok := vanilladata.LoadDensityFunction(name)
	if !ok {
		return nil, fmt.Errorf("density function %q not found", name)
	}
	p := parser{
		ctx:      ctx,
		settings: NoiseSettings{CellWidth: 4, CellHeight: 8},
		cache:    make(map[string]Function),
	}
	return p.parse(raw)
}

// ClimateFunctions are the six noise-router fields the biome source reads.
// Humidity is the router's "vegetation" function and Weirdness its "ridges"
// function — the naming follows Climate.Sampler (NoiseRouter.createClimateSampler).
type ClimateFunctions struct {
	Temperature     Function
	Humidity        Function
	Continentalness Function
	Erosion         Function
	Depth           Function
	Weirdness       Function
}

// ParseClimate parses a noise_settings JSON's climate router fields
// (temperature, vegetation, continents, erosion, depth, ridges) with one
// shared parser, so referenced functions are compiled once.
func ParseClimate(ctx Context, raw json.RawMessage) (ClimateFunctions, error) {
	var top struct {
		NoiseRouter json.RawMessage `json:"noise_router"`
	}
	if err := json.Unmarshal(raw, &top); err != nil {
		return ClimateFunctions{}, err
	}
	var router struct {
		Temperature json.RawMessage `json:"temperature"`
		Vegetation  json.RawMessage `json:"vegetation"`
		Continents  json.RawMessage `json:"continents"`
		Erosion     json.RawMessage `json:"erosion"`
		Depth       json.RawMessage `json:"depth"`
		Ridges      json.RawMessage `json:"ridges"`
	}
	if err := json.Unmarshal(top.NoiseRouter, &router); err != nil {
		return ClimateFunctions{}, err
	}

	p := parser{
		ctx:      ctx,
		settings: NoiseSettings{CellWidth: 4, CellHeight: 8},
		cache:    make(map[string]Function),
	}
	var out ClimateFunctions
	for _, pair := range []struct {
		raw json.RawMessage
		fn  *Function
	}{
		{router.Temperature, &out.Temperature},
		{router.Vegetation, &out.Humidity},
		{router.Continents, &out.Continentalness},
		{router.Erosion, &out.Erosion},
		{router.Depth, &out.Depth},
		{router.Ridges, &out.Weirdness},
	} {
		f, err := p.parse(pair.raw)
		if err != nil {
			return ClimateFunctions{}, err
		}
		*pair.fn = f
	}
	return out, nil
}

type parser struct {
	ctx      Context
	settings NoiseSettings
	cache    map[string]Function
}

// cacheIDSequence hands out memo-namespace ids that are unique across every
// parser in the process. Several trees (the final_density tree and the six
// climate router fields) share ONE evaluation Context — and therefore one
// memo map — so per-parser counters would let a terrain node's cached value be
// served to a climate node at the same coordinates.
var cacheIDSequence atomic.Uint32

// cacheID allocates a fresh namespace id for one caching node's memo entries.
func (p *parser) cacheID() uint32 {
	return cacheIDSequence.Add(1)
}

func (p *parser) parse(raw json.RawMessage) (Function, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return Constant{0}, nil
	}

	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		return p.parseRef(s)
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, err
		}
		return p.parseObject(obj)
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, err
		}
		return Constant{f}, nil
	case 'n': // null
		return Constant{0}, nil
	default:
		return nil, fmt.Errorf("unexpected density value: %s", string(raw))
	}
}

func (p *parser) parseRef(s string) (Function, error) {
	name := stripNamespace(s)
	if f, ok := p.cache[name]; ok {
		return f, nil
	}
	raw, ok := vanilladata.LoadDensityFunction(name)
	if !ok {
		return nil, fmt.Errorf("density function not found: %s", s)
	}
	f, err := p.parse(raw)
	if err != nil {
		return nil, err
	}
	p.cache[name] = f
	return f, nil
}

func (p *parser) parseObject(obj map[string]json.RawMessage) (Function, error) {
	typRaw, ok := obj["type"]
	if !ok {
		return nil, fmt.Errorf("density object missing type: %v", obj)
	}
	var typ string
	if err := json.Unmarshal(typRaw, &typ); err != nil {
		return nil, err
	}
	typ = stripNamespace(typ)

	switch typ {
	case "add", "mul", "min", "max", "sub", "div":
		return p.parseBinary(typ, obj)
	case "abs":
		return p.parseUnary("input", obj, func(a Function) Function { return Abs{A: a} })
	case "square":
		return p.parseUnary("input", obj, func(a Function) Function { return Square{A: a} })
	case "cube":
		return p.parseUnary("input", obj, func(a Function) Function { return Cube{A: a} })
	case "sqrt":
		return p.parseUnary("input", obj, func(a Function) Function { return Sqrt{A: a} })
	case "log":
		return p.parseUnary("input", obj, func(a Function) Function { return Log{A: a} })
	case "negate":
		return p.parseUnary("input", obj, func(a Function) Function { return Negate{A: a} })
	case "reciprocal":
		return p.parseUnary("input", obj, func(a Function) Function { return Reciprocal{A: a} })
	case "sign":
		return p.parseUnary("input", obj, func(a Function) Function { return Sign{A: a} })
	case "half_negative":
		return p.parseUnary("input", obj, func(a Function) Function { return HalfNegative{A: a} })
	case "quarter_negative":
		return p.parseUnary("input", obj, func(a Function) Function { return QuarterNegative{A: a} })
	case "invert":
		return p.parseUnary("input", obj, func(a Function) Function { return Invert{A: a} })
	case "squeeze":
		return p.parseUnary("input", obj, func(a Function) Function { return Squeeze{A: a} })
	case "clamp":
		return p.parseClamp(obj)
	case "range_choice":
		return p.parseRangeChoice(obj)
	case "noise":
		return p.parseNoise(obj)
	case "shifted_noise":
		return p.parseShiftedNoise(obj)
	case "shift_a":
		return p.parseShift("noise", obj, func(n string, s *noise.NormalNoise) Function {
			return ShiftA{Noise: n, Sampler: s}
		})
	case "shift_b":
		return p.parseShift("noise", obj, func(n string, s *noise.NormalNoise) Function {
			return ShiftB{Noise: n, Sampler: s}
		})
	case "y_clamped_gradient":
		return p.parseYClampedGradient(obj)
	case "gradient":
		return p.parseGradient(obj)
	case "distance_to_point":
		return p.parseDistanceToPoint(obj)
	case "slice":
		return p.parseSlice(obj)
	case "lerp":
		return p.parseLerp(obj)
	case "y":
		return Y{}, nil
	case "spline":
		return p.parseSpline(obj["spline"])
	case "old_blended_noise":
		return p.parseOldBlendedNoise(obj)
	case "blend_density":
		return p.parseUnary("input", obj, func(a Function) Function { return BlendDensity{A: a} })
	case "blend_alpha":
		return BlendAlpha{Constant{Value: 1}}, nil
	case "blend_offset":
		return BlendOffset{Constant{Value: 0}}, nil
	case "cache", "cache_once":
		a, err := p.parseRequired("input", obj)
		if err != nil {
			return nil, err
		}
		return &CacheOnce{A: a, ID: p.cacheID()}, nil
	case "cache_2d":
		a, err := p.parseRequired("input", obj)
		if err != nil {
			return nil, err
		}
		return &Cache2D{A: a, ID: p.cacheID()}, nil
	case "flat_cache":
		a, err := p.parseRequired("input", obj)
		if err != nil {
			return nil, err
		}
		return &FlatCache{A: a, ID: p.cacheID()}, nil
	case "interpolated":
		a, err := p.parseRequired("input", obj)
		if err != nil {
			return nil, err
		}
		// 26.3 spells the cell size out (cell_size_xz / cell_size_y); the
		// 1.18-era form fell back to the noise-settings cell size.
		var cellXz, cellY int
		if raw, ok := obj["cell_size_xz"]; ok {
			if err := json.Unmarshal(raw, &cellXz); err != nil {
				return nil, err
			}
		} else {
			cellXz = p.settings.CellWidth
		}
		if raw, ok := obj["cell_size_y"]; ok {
			if err := json.Unmarshal(raw, &cellY); err != nil {
				return nil, err
			}
		} else {
			cellY = p.settings.CellHeight
		}
		// The corner values must be memoized per position and shared across
		// nested interpolations — wrapping the child in CacheOnce achieves
		// that with integer cache keys (see Context.cached).
		wrapped := &CacheOnce{A: a, ID: p.cacheID()}
		return NewInterpolated26(wrapped, cellXz, cellY), nil
	case "cache_all_in_cell":
		a, err := p.parseRequired("input", obj)
		if err != nil {
			return nil, err
		}
		return CacheAllInCell{A: a}, nil
	case "interval_select":
		return p.parseIntervalSelect(obj)
	case "end_islands", "end_outer_islands":
		// End-dimension leaf (26.3 "end_outer_islands", EndIslandFunction).
		// LivingWorld generates the overworld only; the node exists so every
		// vanilla density function parses (the audit gate) but evaluates as a
		// constant outside the end.
		return EndIslands{}, nil
	case "find_top_surface":
		return p.parseFindTopSurface(obj)
	case "beardifier":
		return Beardifier{}, nil
	case "weird_scaled_sampler":
		return WeirdScaledSampler{}, nil
	case "shift":
		return Shift{}, nil
	case "constant":
		var v float64
		if err := json.Unmarshal(obj["argument"], &v); err != nil {
			return nil, err
		}
		return Constant{Value: v}, nil
	default:
		return nil, fmt.Errorf("unknown density function type: %s", typ)
	}
}

// parseBinary parses the 26.3 binary operators. Vanilla's JSON fields are
// "left" and "right" (BinaryFunction$Type codec); both are REQUIRED — the old
// silent Constant{0} default for missing fields is exactly how every binary
// node used to collapse to zero without an error.
func (p *parser) parseBinary(typ string, obj map[string]json.RawMessage) (Function, error) {
	a, err := p.parseRequired("left", obj)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", typ, err)
	}
	b, err := p.parseRequired("right", obj)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", typ, err)
	}
	switch typ {
	case "add":
		return Add{A: a, B: b}, nil
	case "mul":
		return Mul{A: a, B: b}, nil
	case "min":
		return Min{A: a, B: b}, nil
	case "max":
		return Max{A: a, B: b}, nil
	case "sub":
		return Sub{A: a, B: b}, nil
	case "div":
		return Div{A: a, B: b}, nil
	}
	return nil, fmt.Errorf("unknown binary type: %s", typ)
}

func (p *parser) parseUnary(field string, obj map[string]json.RawMessage, mk func(Function) Function) (Function, error) {
	a, err := p.parseRequired(field, obj)
	if err != nil {
		return nil, err
	}
	return mk(a), nil
}

// parseRequired parses a mandatory operand and errors when it is absent.
func (p *parser) parseRequired(field string, obj map[string]json.RawMessage) (Function, error) {
	raw, ok := obj[field]
	if !ok {
		return nil, fmt.Errorf("missing required field %q in density object", field)
	}
	return p.parse(raw)
}

// parseField keeps the lenient default for genuinely optional fields.
func (p *parser) parseField(field string, obj map[string]json.RawMessage) (Function, error) {
	raw, ok := obj[field]
	if !ok {
		return Constant{0}, nil
	}
	return p.parse(raw)
}

func (p *parser) parseClamp(obj map[string]json.RawMessage) (Function, error) {
	a, err := p.parseField("input", obj)
	if err != nil {
		return nil, err
	}
	var min, max float64
	if err := json.Unmarshal(obj["min"], &min); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(obj["max"], &max); err != nil {
		return nil, err
	}
	return Clamp{A: a, Min: min, Max: max}, nil
}

func (p *parser) parseRangeChoice(obj map[string]json.RawMessage) (Function, error) {
	input, err := p.parseField("input", obj)
	if err != nil {
		return nil, err
	}
	var min, max float64
	if err := json.Unmarshal(obj["min_inclusive"], &min); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(obj["max_exclusive"], &max); err != nil {
		return nil, err
	}
	inRange, err := p.parseField("when_in_range", obj)
	if err != nil {
		return nil, err
	}
	outRange, err := p.parseField("when_out_of_range", obj)
	if err != nil {
		return nil, err
	}
	return RangeChoice{Input: input, Min: min, Max: max, WhenInRange: inRange, WhenOutOfRange: outRange}, nil
}

func (p *parser) parseNoise(obj map[string]json.RawMessage) (Function, error) {
	var name string
	if err := json.Unmarshal(obj["noise"], &name); err != nil {
		return nil, err
	}
	name = stripNamespace(name)
	var xz, y float64
	if err := json.Unmarshal(obj["xz_scale"], &xz); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(obj["y_scale"], &y); err != nil {
		return nil, err
	}
	// 26.3's minecraft:noise takes OPTIONAL shift_x/shift_y/shift_z (the codec
	// picks NoiseFunction.ShiftedXzSampler or ShiftedXyzSampler); every vanilla
	// climate field uses them. Ignoring them silently shifted the whole
	// biome climate off vanilla's by several units.
	_, hasShiftX := obj["shift_x"]
	_, hasShiftY := obj["shift_y"]
	_, hasShiftZ := obj["shift_z"]
	if !hasShiftX && !hasShiftY && !hasShiftZ {
		return Noise{Noise: name, Sampler: p.ctx.Noises[name], XZScale: xz, YScale: y}, nil
	}
	sx, err := p.parseField("shift_x", obj)
	if err != nil {
		return nil, err
	}
	sy, err := p.parseField("shift_y", obj)
	if err != nil {
		return nil, err
	}
	sz, err := p.parseField("shift_z", obj)
	if err != nil {
		return nil, err
	}
	return ShiftedNoise{Noise: name, Sampler: p.ctx.Noises[name], ShiftX: sx, ShiftY: sy, ShiftZ: sz, XZScale: xz, YScale: y}, nil
}

func (p *parser) parseShiftedNoise(obj map[string]json.RawMessage) (Function, error) {
	var name string
	if err := json.Unmarshal(obj["noise"], &name); err != nil {
		return nil, err
	}
	name = stripNamespace(name)
	var xz, y float64
	if err := json.Unmarshal(obj["xz_scale"], &xz); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(obj["y_scale"], &y); err != nil {
		return nil, err
	}
	sx, err := p.parseField("shift_x", obj)
	if err != nil {
		return nil, err
	}
	sy, err := p.parseField("shift_y", obj)
	if err != nil {
		return nil, err
	}
	sz, err := p.parseField("shift_z", obj)
	if err != nil {
		return nil, err
	}
	return ShiftedNoise{Noise: name, Sampler: p.ctx.Noises[name], ShiftX: sx, ShiftY: sy, ShiftZ: sz, XZScale: xz, YScale: y}, nil
}

func (p *parser) parseShift(field string, obj map[string]json.RawMessage, mk func(string, *noise.NormalNoise) Function) (Function, error) {
	var name string
	if err := json.Unmarshal(obj[field], &name); err != nil {
		return nil, err
	}
	name = stripNamespace(name)
	return mk(name, p.ctx.Noises[name]), nil
}

func (p *parser) parseYClampedGradient(obj map[string]json.RawMessage) (Function, error) {
	var o struct {
		FromY     int     `json:"from_y"`
		ToY       int     `json:"to_y"`
		FromValue float64 `json:"from_value"`
		ToValue   float64 `json:"to_value"`
	}
	if err := json.Unmarshal(marshalRaw(obj), &o); err != nil {
		return nil, err
	}
	return YClampedGradient{FromY: o.FromY, ToY: o.ToY, FromValue: o.FromValue, ToValue: o.ToValue}, nil
}

// parseGradient parses the 26.3 "gradient" leaf (GradientFunction codec:
// axis, tiling [default clamp_to_edge], from_coordinate, to_coordinate,
// from_value, to_value). Vanilla rejects from_coordinate == to_coordinate.
func (p *parser) parseGradient(obj map[string]json.RawMessage) (Function, error) {
	axis, err := parseAxis(obj["axis"])
	if err != nil {
		return nil, err
	}
	tiling := TilingClampToEdge
	if raw, ok := obj["tiling"]; ok {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		switch stripNamespace(s) {
		case "clamp_to_edge":
			tiling = TilingClampToEdge
		case "repeat":
			tiling = TilingRepeat
		case "mirrored_repeat":
			tiling = TilingMirroredRepeat
		default:
			return nil, fmt.Errorf("unknown gradient tiling mode: %s", s)
		}
	}
	var o struct {
		FromCoordinate int     `json:"from_coordinate"`
		ToCoordinate   int     `json:"to_coordinate"`
		FromValue      float64 `json:"from_value"`
		ToValue        float64 `json:"to_value"`
	}
	if err := json.Unmarshal(marshalRaw(obj), &o); err != nil {
		return nil, err
	}
	if o.FromCoordinate == o.ToCoordinate {
		return nil, fmt.Errorf("from_coordinate cannot be equal to to_coordinate")
	}
	return Gradient{
		Axis: axis, Tiling: tiling,
		FromCoord: o.FromCoordinate, ToCoord: o.ToCoordinate,
		FromValue: o.FromValue, ToValue: o.ToValue,
	}, nil
}

// parseSlice parses the 26.3 "slice" operator (SliceFunction record:
// axis, coordinate, input).
func (p *parser) parseSlice(obj map[string]json.RawMessage) (Function, error) {
	axis, err := parseAxis(obj["axis"])
	if err != nil {
		return nil, err
	}
	var coord int
	if err := json.Unmarshal(obj["coordinate"], &coord); err != nil {
		return nil, err
	}
	input, err := p.parseRequired("input", obj)
	if err != nil {
		return nil, err
	}
	return Slice{Axis: axis, Coordinate: coord, Input: input}, nil
}

// parseLerp parses the 26.3 "lerp" operator (LerpFunction record:
// alpha, first, second).
func (p *parser) parseLerp(obj map[string]json.RawMessage) (Function, error) {
	alpha, err := p.parseRequired("alpha", obj)
	if err != nil {
		return nil, err
	}
	first, err := p.parseRequired("first", obj)
	if err != nil {
		return nil, err
	}
	second, err := p.parseRequired("second", obj)
	if err != nil {
		return nil, err
	}
	return Lerp{Alpha: alpha, First: first, Second: second}, nil
}

// parseDistanceToPoint parses the 26.3 "distance_to_point" leaf
// (DistanceToPointFunction record: point, metric).
func (p *parser) parseDistanceToPoint(obj map[string]json.RawMessage) (Function, error) {
	var point []int
	if err := json.Unmarshal(obj["point"], &point); err != nil {
		return nil, err
	}
	if len(point) != 3 {
		return nil, fmt.Errorf("distance_to_point: point must have 3 coordinates, got %d", len(point))
	}
	var metric string
	if err := json.Unmarshal(obj["metric"], &metric); err != nil {
		return nil, err
	}
	var m byte
	switch stripNamespace(metric) {
	case "euclidean":
		m = 'e'
	case "euclidean_squared":
		m = 's'
	case "manhattan":
		m = 'm'
	case "chebyshev":
		m = 'c'
	default:
		return nil, fmt.Errorf("unknown distance metric: %s", metric)
	}
	return DistanceToPoint{PX: point[0], PY: point[1], PZ: point[2], Metric: m}, nil
}

// parseAxis reads a Direction.Axis JSON value ("x", "y" or "z").
func parseAxis(raw json.RawMessage) (byte, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, err
	}
	switch stripNamespace(s) {
	case "x":
		return 'x', nil
	case "y":
		return 'y', nil
	case "z":
		return 'z', nil
	}
	return 0, fmt.Errorf("unknown axis: %s", s)
}

func (p *parser) parseIntervalSelect(obj map[string]json.RawMessage) (Function, error) {
	input, err := p.parseField("input", obj)
	if err != nil {
		return nil, err
	}
	var funcs []json.RawMessage
	if err := json.Unmarshal(obj["functions"], &funcs); err != nil {
		return nil, err
	}
	var thresholds []float64
	if err := json.Unmarshal(obj["thresholds"], &thresholds); err != nil {
		return nil, err
	}
	functions := make([]Function, len(funcs))
	for i, raw := range funcs {
		f, err := p.parse(raw)
		if err != nil {
			return nil, err
		}
		functions[i] = f
	}
	return &IntervalSelect{Input: input, Functions: functions, Thresholds: thresholds}, nil
}

func (p *parser) parseOldBlendedNoise(obj map[string]json.RawMessage) (Function, error) {
	var o struct {
		XZScale              float64 `json:"xz_scale"`
		YScale               float64 `json:"y_scale"`
		XZFactor             float64 `json:"xz_factor"`
		YFactor              float64 `json:"y_factor"`
		SmearScaleMultiplier float64 `json:"smear_scale_multiplier"`
	}
	if err := json.Unmarshal(marshalRaw(obj), &o); err != nil {
		return nil, err
	}

	// BlendedNoise's octaves are drawn from
	// DensityFunction$CompileContext.createRandom(Identifier.withDefaultNamespace("terrain")),
	// i.e. the MD5 hash of the namespaced "minecraft:terrain" — matching the
	// same seed stream vanilla uses for the legacy/modern terrain noise.
	r := noise.NewXoroshiro(uint64(p.ctx.Seed)).ForkPositional().FromHashOf("minecraft:terrain")
	bn := NewBlendedNoise26(r, o.XZScale, o.YScale, o.XZFactor, o.YFactor, o.SmearScaleMultiplier)
	return BlendedNoise26Eval{bn: bn}, nil
}

// BlendedNoise26Eval adapts BlendedNoise26 to the Function interface.
type BlendedNoise26Eval struct{ bn BlendedNoise26 }

func (o BlendedNoise26Eval) Eval(c Context, x, y, z int) float64 {
	return o.bn.Eval(c, x, y, z)
}

// parseFindTopSurface parses minecraft:find_top_surface.
func (p *parser) parseFindTopSurface(obj map[string]json.RawMessage) (Function, error) {
	densityFn, err := p.parseRequired("density", obj)
	if err != nil {
		return nil, err
	}
	upper, err := p.parseRequired("upper_bound", obj)
	if err != nil {
		return nil, err
	}
	var lower, cellHeight int
	if err := json.Unmarshal(obj["lower_bound"], &lower); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(obj["cell_height"], &cellHeight); err != nil {
		return nil, err
	}
	return FindTopSurface{
		Density:    densityFn,
		UpperBound: upper,
		LowerBound: lower,
		CellHeight: cellHeight,
	}, nil
}

func (p *parser) parseSpline(raw json.RawMessage) (Function, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return Constant{0}, nil
	}
	if raw[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, err
		}
		coord, err := p.parse(obj["coordinate"])
		if err != nil {
			return nil, err
		}
		pointsRaw, ok := obj["points"]
		if !ok {
			return Constant{0}, nil
		}
		var pts []json.RawMessage
		if err := json.Unmarshal(pointsRaw, &pts); err != nil {
			return nil, err
		}
		points := make([]SplinePoint, 0, len(pts))
		for _, pr := range pts {
			var pobj map[string]json.RawMessage
			if err := json.Unmarshal(pr, &pobj); err != nil {
				return nil, err
			}
			var loc float64
			if err := json.Unmarshal(pobj["location"], &loc); err != nil {
				return nil, err
			}
			var deriv float64
			if err := json.Unmarshal(pobj["derivative"], &deriv); err != nil {
				return nil, err
			}
			valFn, err := p.parseSpline(pobj["value"])
			if err != nil {
				return nil, err
			}
			points = append(points, SplinePoint{Location: loc, Value: valFn, Derivative: deriv})
		}
		locs := make([]float64, len(points))
		for i, sp := range points {
			locs[i] = float64(fround(float32(sp.Location)))
		}
		return &Spline{Input: coord, Points: points, locs: locs}, nil
	}
	if raw[0] == '-' || (raw[0] >= '0' && raw[0] <= '9') {
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, err
		}
		return Constant{Value: f}, nil
	}
	return nil, fmt.Errorf("unexpected spline value: %s", string(raw))
}

func stripNamespace(s string) string {
	if i := strings.Index(s, ":"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// marshalRaw is a small helper that re-encodes a raw JSON object so that
// json.Unmarshal can populate a struct. It avoids an extra copy for the
// common case.
func marshalRaw(obj map[string]json.RawMessage) json.RawMessage {
	b, _ := json.Marshal(obj)
	return b
}
