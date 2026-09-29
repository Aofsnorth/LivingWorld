package pipeline

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"livingworld/internal/dimension/overworld/density"
)

// TestTreeOracleDiff diffs our parsed density-function tree against the vanilla
// 26.3 TreeOracle dump, function by function, so the first diverging
// sub-function is visible. Skipped unless TREE_ORACLE_FILE is set:
//
//	TREE_ORACLE_FILE=tmp/tree_out.txt go test ./internal/dimension/overworld/pipeline/ -run TestTreeOracleDiff -v
func TestTreeOracleDiff(t *testing.T) {
	path := os.Getenv("TREE_ORACLE_FILE")
	if path == "" {
		t.Skip("set TREE_ORACLE_FILE to a TreeOracle dump")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	g := NewGenerator(12345)
	sc := bufio.NewScanner(f)
	perFn := map[string][2]int{} // name -> [match, mismatch]
	var firstBad []string
	for sc.Scan() {
		parts := strings.Split(sc.Text(), "\t")
		if len(parts) < 5 {
			continue
		}
		name := strings.TrimPrefix(parts[0], "minecraft:")
		x, _ := strconv.Atoi(parts[1])
		y, _ := strconv.Atoi(parts[2])
		z, _ := strconv.Atoi(parts[3])
		want, _ := strconv.ParseFloat(parts[4], 64)

		fn, err := density.ParseNamed(g.densTmpl, name)
		if err != nil || fn == nil {
			continue
		}
		dc := g.densTmpl.Fresh()
		got := fn.Eval(dc, x, y, z)
		stat := perFn[name]
		if diff := got - want; diff > 1e-4 || diff < -1e-4 {
			stat[1]++
			if len(firstBad) < 40 {
				firstBad = append(firstBad, fmt.Sprintf("%-45s (%d,%d,%d) ours=%+.7f vanilla=%+.7f", name, x, y, z, got, want))
			}
		} else {
			stat[0]++
		}
		perFn[name] = stat
	}
	// Print the mismatching functions only.
	for name, stat := range perFn {
		if stat[1] > 0 {
			fmt.Printf("MISMATCH %-45s %d/%d samples differ\n", name, stat[1], stat[0]+stat[1])
		}
	}
	for _, line := range firstBad {
		fmt.Println("   ", line)
	}
}
