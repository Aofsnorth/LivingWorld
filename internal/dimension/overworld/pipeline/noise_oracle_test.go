package pipeline

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestNoiseOracleDiff compares our per-noise NormalNoise values against the
// vanilla 26.3 NoiseOracle dump. Skipped unless NOISE_ORACLE_FILE is set:
//
//	NOISE_ORACLE_FILE=tmp/noise_out.txt go test ./internal/dimension/overworld/pipeline/ -run TestNoiseOracleDiff -v
func TestNoiseOracleDiff(t *testing.T) {
	path := os.Getenv("NOISE_ORACLE_FILE")
	if path == "" {
		t.Skip("set NOISE_ORACLE_FILE to a NoiseOracle dump")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	g := NewGenerator(12345)
	dc := g.densTmpl.Fresh()
	sc := bufio.NewScanner(f)
	checked, mismatch := 0, 0
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) < 5 {
			continue
		}
		name := parts[0]
		if i := strings.LastIndexByte(name, ':'); i >= 0 {
			name = name[i+1:]
		}
		x, _ := strconv.Atoi(parts[1])
		y, _ := strconv.Atoi(parts[2])
		z, _ := strconv.Atoi(parts[3])
		want, _ := strconv.ParseFloat(parts[4], 64)

		n, ok := g.densTmpl.Noises[name]
		if !ok {
			t.Errorf("no noise %q in set", name)
			continue
		}
		got := n.Sample(float64(x), float64(y), float64(z))
		_ = dc
		checked++
		if diff := got - want; diff > 1e-5 || diff < -1e-5 {
			mismatch++
			if mismatch <= 12 {
				fmt.Printf("%-34s (%4d,%3d,%4d) ours=%+.7f vanilla=%+.7f delta=%+.7f\n", name, x, y, z, got, want, got-want)
			}
		}
	}
	t.Logf("checked %d noise samples, %d mismatches", checked, mismatch)
	if mismatch > 0 {
		t.Errorf("%d/%d noise samples differ from vanilla", mismatch, checked)
	}
}
