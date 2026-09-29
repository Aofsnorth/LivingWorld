package density

import (
	"fmt"
	"sort"
	"testing"

	"livingworld/internal/dimension/overworld/vanilladata"
)

// Vanilla 26.3 parity audit for the density parser.
//
// The contract is 1:1 with vanilla: every density function shipped in vanilla
// 26.3's worldgen data must parse with this parser, and every operator must
// read the exact field names vanilla's JSON uses. These tests are the audit
// gate: they fail whenever the parser drifts from the vanilla schema.

func auditContext() Context {
	return NewContext(12345, NoiseSet{})
}

// TestVanillaDensityFunctionsAllParse parses every density function embedded
// from vanilla 26.3 (vanilladata) and reports the ones the parser rejects.
func TestVanillaDensityFunctionsAllParse(t *testing.T) {
	names := make([]string, 0, len(vanilladata.DensityFunctions))
	for name := range vanilladata.DensityFunctions {
		names = append(names, name)
	}
	sort.Strings(names)

	var failed []string
	for _, name := range names {
		p := parser{ctx: auditContext(), settings: NoiseSettings{CellWidth: 4, CellHeight: 8}, cache: map[string]Function{}}
		if _, perr := p.parse([]byte(fmt.Sprintf("%q", name))); perr != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", name, perr))
		}
	}
	if len(failed) > 0 {
		for _, f := range failed {
			t.Errorf("vanilla density function does not parse: %s", f)
		}
		t.Fatalf("%d/%d vanilla density functions rejected by the parser", len(failed), len(names))
	}
	t.Logf("all %d vanilla density functions parse", len(names))
}

// TestOverworldFinalDensityParses runs the exact entry point the world
// pipeline uses (context.go calls ParseFinalDensity and panics on error).
func TestOverworldFinalDensityParses(t *testing.T) {
	raw, ok := vanilladata.LoadNoiseSettings("overworld")
	if !ok {
		t.Fatal("embedded overworld noise_settings missing")
	}
	if _, err := ParseFinalDensity(auditContext(), raw); err != nil {
		t.Fatalf("ParseFinalDensity(overworld) failed: %v", err)
	}
}

// TestBinaryOperatorsUseLeftRightFields pins vanilla 26.3's binary-op field
// names: add/mul/min/max/div/sub are {left, right}. The parser once read
// "argument1"/"argument2", which silently evaluated every binary node as
// Constant{0}+Constant{0} because parseField defaults missing fields to zero.
func TestBinaryOperatorsUseLeftRightFields(t *testing.T) {
	p := parser{ctx: auditContext(), settings: NoiseSettings{CellWidth: 4, CellHeight: 8}, cache: map[string]Function{}}
	for _, tc := range []struct {
		typ  string
		want float64
	}{
		{"add", 3}, {"mul", 2}, {"min", 1}, {"max", 2}, {"sub", -1}, {"div", 0.5},
	} {
		raw := []byte(fmt.Sprintf(`{"type":"minecraft:%s","left":1.0,"right":2.0}`, tc.typ))
		f, err := p.parse(raw)
		if err != nil {
			t.Errorf("%s: parse error: %v", tc.typ, err)
			continue
		}
		if got := f.Eval(auditContext(), 0, 0, 0); got != tc.want {
			t.Errorf("%s(1,2) = %v, want %v (field names must be left/right)", tc.typ, got, tc.want)
		}
	}
}

// TestUnaryOperatorsUseInputField pins vanilla's unary field name: abs, cube,
// square, squeeze, half_negative, quarter_negative, blend_density take "input".
func TestUnaryOperatorsUseInputField(t *testing.T) {
	p := parser{ctx: auditContext(), settings: NoiseSettings{CellWidth: 4, CellHeight: 8}, cache: map[string]Function{}}
	raw := []byte(`{"type":"minecraft:abs","input":-4.0}`)
	f, err := p.parse(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got := f.Eval(auditContext(), 0, 0, 0); got != 4 {
		t.Errorf("abs(-4) = %v, want 4 (field name must be input)", got)
	}
}

// TestCacheOperatorSupported pins the 26.3 "cache" operator (memoization).
func TestCacheOperatorSupported(t *testing.T) {
	p := parser{ctx: auditContext(), settings: NoiseSettings{CellWidth: 4, CellHeight: 8}, cache: map[string]Function{}}
	raw := []byte(`{"type":"minecraft:cache","input":2.5}`)
	f, err := p.parse(raw)
	if err != nil {
		t.Fatalf("minecraft:cache must parse (32 uses in vanilla 26.3 data): %v", err)
	}
	if got := f.Eval(auditContext(), 0, 0, 0); got != 2.5 {
		t.Errorf("cache(2.5) = %v, want 2.5", got)
	}
}

// TestGradientOperatorSupported pins the 26.3 "gradient" operator shape
// ({axis, from_coordinate, from_value, to_coordinate, to_value}).
func TestGradientOperatorSupported(t *testing.T) {
	p := parser{ctx: auditContext(), settings: NoiseSettings{CellWidth: 4, CellHeight: 8}, cache: map[string]Function{}}
	raw := []byte(`{"type":"minecraft:gradient","axis":"y","from_coordinate":0,"from_value":0.0,"to_coordinate":10,"to_value":1.0}`)
	f, err := p.parse(raw)
	if err != nil {
		t.Fatalf("minecraft:gradient must parse (23 uses in vanilla 26.3 data): %v", err)
	}
	if got := f.Eval(auditContext(), 0, 5, 0); got != 0.5 {
		t.Errorf("gradient at y=5 = %v, want 0.5", got)
	}
}

// TestLerpOperatorSupported pins the 26.3 "lerp" operator (LerpFunction
// record: alpha, first, second; Mth.lerp(alpha, first, second)).
func TestLerpOperatorSupported(t *testing.T) {
	p := parser{ctx: auditContext(), settings: NoiseSettings{CellWidth: 4, CellHeight: 8}, cache: map[string]Function{}}
	raw := []byte(`{"type":"minecraft:lerp","alpha":0.5,"first":0.0,"second":10.0}`)
	f, err := p.parse(raw)
	if err != nil {
		t.Fatalf("minecraft:lerp must parse (14 uses in vanilla 26.3 data): %v", err)
	}
	if got := f.Eval(auditContext(), 0, 0, 0); got != 5 {
		t.Errorf("lerp(alpha=0.5, first=0, second=10) = %v, want 5", got)
	}
}
