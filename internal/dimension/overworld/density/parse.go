package density

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

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

type parser struct {
	ctx      Context
	settings NoiseSettings
	cache    map[string]Function
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
	case "add", "mul", "min", "max":
		return p.parseBinary(typ, obj)
	case "abs":
		return p.parseUnary("argument", obj, func(a Function) Function { return Abs{A: a} })
	case "square":
		return p.parseUnary("argument", obj, func(a Function) Function { return Square{A: a} })
	case "cube":
		return p.parseUnary("argument", obj, func(a Function) Function { return Cube{A: a} })
	case "half_negative":
		return p.parseUnary("argument", obj, func(a Function) Function { return HalfNegative{A: a} })
	case "quarter_negative":
		return p.parseUnary("argument", obj, func(a Function) Function { return QuarterNegative{A: a} })
	case "invert":
		return p.parseUnary("argument", obj, func(a Function) Function { return Invert{A: a} })
	case "squeeze":
		return p.parseUnary("argument", obj, func(a Function) Function { return Squeeze{A: a} })
	case "clamp":
		return p.parseClamp(obj)
	case "range_choice":
		return p.parseRangeChoice(obj)
	case "noise":
		return p.parseNoise(obj)
	case "shifted_noise":
		return p.parseShiftedNoise(obj)
	case "shift_a":
		return p.parseShift("argument", obj, func(n string) Function { return ShiftA{Noise: n} })
	case "shift_b":
		return p.parseShift("argument", obj, func(n string) Function { return ShiftB{Noise: n} })
	case "y_clamped_gradient":
		return p.parseYClampedGradient(obj)
	case "y":
		return Y{}, nil
	case "spline":
		return p.parseSpline(obj["spline"])
	case "old_blended_noise":
		return p.parseOldBlendedNoise(obj)
	case "blend_density":
		return p.parseUnary("argument", obj, func(a Function) Function { return BlendDensity{A: a} })
	case "blend_alpha":
		return BlendAlpha{Constant{Value: 1}}, nil
	case "blend_offset":
		return BlendOffset{Constant{Value: 0}}, nil
	case "cache_once":
		a, err := p.parseField("argument", obj)
		if err != nil {
			return nil, err
		}
		return &CacheOnce{A: a}, nil
	case "cache_2d":
		a, err := p.parseField("argument", obj)
		if err != nil {
			return nil, err
		}
		return &Cache2D{A: a}, nil
	case "flat_cache":
		a, err := p.parseField("argument", obj)
		if err != nil {
			return nil, err
		}
		return &FlatCache{A: a}, nil
	case "interpolated":
		a, err := p.parseField("argument", obj)
		if err != nil {
			return nil, err
		}
		return &Interpolated{A: a, CellWidth: p.settings.CellWidth, CellHeight: p.settings.CellHeight}, nil
	case "cache_all_in_cell":
		a, err := p.parseField("argument", obj)
		if err != nil {
			return nil, err
		}
		return CacheAllInCell{A: a}, nil
	case "interval_select":
		return p.parseIntervalSelect(obj)
	case "end_islands":
		return EndIslands{}, nil
	case "find_top_surface":
		return FindTopSurface{}, nil
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

func (p *parser) parseBinary(typ string, obj map[string]json.RawMessage) (Function, error) {
	a, err := p.parseField("argument1", obj)
	if err != nil {
		return nil, err
	}
	b, err := p.parseField("argument2", obj)
	if err != nil {
		return nil, err
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
	}
	return nil, fmt.Errorf("unknown binary type: %s", typ)
}

func (p *parser) parseUnary(field string, obj map[string]json.RawMessage, mk func(Function) Function) (Function, error) {
	a, err := p.parseField(field, obj)
	if err != nil {
		return nil, err
	}
	return mk(a), nil
}

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
	return Noise{Noise: name, XZScale: xz, YScale: y}, nil
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
	return ShiftedNoise{Noise: name, ShiftX: sx, ShiftY: sy, ShiftZ: sz, XZScale: xz, YScale: y}, nil
}

func (p *parser) parseShift(field string, obj map[string]json.RawMessage, mk func(string) Function) (Function, error) {
	var name string
	if err := json.Unmarshal(obj[field], &name); err != nil {
		return nil, err
	}
	return mk(stripNamespace(name)), nil
}

func (p *parser) parseYClampedGradient(obj map[string]json.RawMessage) (Function, error) {
	var o struct {
		FromY    int     `json:"from_y"`
		ToY      int     `json:"to_y"`
		FromValue float64 `json:"from_value"`
		ToValue   float64 `json:"to_value"`
	}
	if err := json.Unmarshal(marshalRaw(obj), &o); err != nil {
		return nil, err
	}
	return YClampedGradient{FromY: o.FromY, ToY: o.ToY, FromValue: o.FromValue, ToValue: o.ToValue}, nil
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

	var r noise.PRNG
	if p.settings.LegacyRandomSource {
		r = noise.NewLegacy(p.ctx.Seed)
	} else {
		r = noise.NewXoroshiro(uint64(p.ctx.Seed)).ForkPositional().FromHashOf("terrain")
	}
	bn := noise.NewBlendedNoise(r, o.XZScale, o.YScale, o.XZFactor, o.YFactor, o.SmearScaleMultiplier)
	return OldBlendedNoise{bn: bn}, nil
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
		return &Spline{Input: coord, Points: points}, nil
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
