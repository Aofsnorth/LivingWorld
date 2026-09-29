package pipeline

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"livingworld/internal/dimension/overworld/density"
)

// TestOracleDiff compares our eight noise-router fields against the vanilla
// 26.3 oracle dump (RouterOracle.java) for the coordinates listed in the input
// file. Skipped unless ORACLE_FILE is set:
//
//	ORACLE_FILE=tmp/router_out.txt go test ./internal/dimension/overworld/pipeline/ -run TestOracleDiff -v
func TestOracleDiff(t *testing.T) {
	path := os.Getenv("ORACLE_FILE")
	if path == "" {
		t.Skip("set ORACLE_FILE to a RouterOracle dump to diff the noise router")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	g := NewGenerator(12345)
	fields := []struct {
		name string
		eval func(dc density.Context, x, y, z int) float64
	}{
		{"temperature", func(dc density.Context, x, y, z int) float64 { return g.climateF.Temperature.Eval(dc, x, y, z) }},
		{"vegetation", func(dc density.Context, x, y, z int) float64 { return g.climateF.Humidity.Eval(dc, x, y, z) }},
		{"continents", func(dc density.Context, x, y, z int) float64 { return g.climateF.Continentalness.Eval(dc, x, y, z) }},
		{"erosion", func(dc density.Context, x, y, z int) float64 { return g.climateF.Erosion.Eval(dc, x, y, z) }},
		{"depth", func(dc density.Context, x, y, z int) float64 { return g.climateF.Depth.Eval(dc, x, y, z) }},
		{"ridges", func(dc density.Context, x, y, z int) float64 { return g.climateF.Weirdness.Eval(dc, x, y, z) }},
		{"final_density", func(dc density.Context, x, y, z int) float64 { return g.dens.Eval(dc, x, y, z) }},
	}
	// The oracle line carries, after x y z: temperature, vegetation,
	// continents, erosion, depth, ridges, chunkSurfaceLevel, finalDensity.
	// chunkSurfaceLevel is a constant 32 in the overworld, so the Go-side
	// field list above maps onto the first six plus the last.

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) < 17 {
			t.Fatalf("bad oracle line: %q", line)
		}
		x, _ := strconv.Atoi(parts[0])
		y, _ := strconv.Atoi(parts[1])
		z, _ := strconv.Atoi(parts[2])
		// Oracle line: x y z shiftX shiftZ inX inY inZ noiseAtInput
		//              temperature vegetation continents erosion depth
		//              ridges chunkSurfaceLevel finalDensity
		want := make([]float64, 8)
		for i := 0; i < 8; i++ {
			want[i], _ = strconv.ParseFloat(parts[9+i], 64)
		}
		if v, err := strconv.ParseFloat(parts[5], 64); err == nil {
			// Noise value at the exact vanilla shifted input coordinates.
			inY, _ := strconv.ParseFloat(parts[6], 64)
			inZ, _ := strconv.ParseFloat(parts[7], 64)
			wantNoise, _ := strconv.ParseFloat(parts[8], 64)
			got := g.densTmpl.Noises["continentalness"].Sample(v, inY, inZ)
			status := "ok"
			if d := got - wantNoise; d > 1e-5 || d < -1e-5 {
				status = "MISMATCH"
			}
			fmt.Printf("(%d,%d) raw-noise-at-input ours=%.9f vanilla=%.9f delta=%.9f %s\n",
				x, z, got, wantNoise, got-wantNoise, status)
		}
		dc := g.densTmpl.Fresh()
		offset := g.densTmpl.Noises["offset"]
		if offset != nil {
			shiftX := offset.Sample(float64(x)*0.25, 0, float64(z)*0.25) * 4
			shiftZ := offset.Sample(float64(z)*0.25, float64(x)*0.25, 0) * 4
			sx, _ := strconv.ParseFloat(parts[3], 64)
			sz, _ := strconv.ParseFloat(parts[4], 64)
			status := "ok"
			if d := shiftX - sx; d > 1e-4 || d < -1e-4 {
				status = "MISMATCH"
			}
			if d := shiftZ - sz; d > 1e-4 || d < -1e-4 {
				status = "MISMATCH"
			}
			fmt.Printf("(%d,%d) shiftX ours=%+.6f vanilla=%+.6f shiftZ ours=%+.6f vanilla=%+.6f %s\n",
				x, z, shiftX, sx, shiftZ, sz, status)
		}
		for i, f := range fields {
			idx := i
			switch f.name {
			case "final_density":
				idx = 7 // want[6] is chunkSurfaceLevel
			}
			got := f.eval(dc, x, y, z)
			status := "ok"
			if diff := got - want[idx]; diff > 1e-3 || diff < -1e-3 {
				status = "MISMATCH"
			}
			fmt.Printf("(%d,%d,%d) %-14s ours=%+.5f vanilla=%+.5f %s\n", x, y, z, f.name, got, want[idx], status)
		}
	}
}
