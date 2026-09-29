package pipeline

import (
	"fmt"
	"os"
	"testing"
)

// TestDumpBiomeOraclePoints writes "<x> <y> <z>" lines (y = the column's terrain
// top, the quart vanilla samples the surface biome in) for the biome-oracle
// harness, so tools/java/ClimateOracle.java can report which biome a vanilla
// 26.3 server would pick for each column.
//
//	BIOME_ORACLE_OUT=tmp/biome_points.txt go test ./internal/dimension/overworld/pipeline/ -run TestDumpBiomeOraclePoints -v
func TestDumpBiomeOraclePoints(t *testing.T) {
	out := os.Getenv("BIOME_ORACLE_OUT")
	if out == "" {
		t.Skip("set BIOME_ORACLE_OUT to dump biome-oracle points")
	}
	g := NewGenerator(12345)
	var sb []byte
	for cx := -3; cx <= 3; cx++ {
		for cz := -3; cz <= 3; cz++ {
			for _, off := range [][2]int{{0, 0}, {7, 3}, {13, 11}} {
				wx, wz := cx*64+off[0], cz*64+off[1]
				top := g.terrainTop(wx, wz)
				fmt.Fprintf(&stringBuilder{&sb}, "%d %d %d\n", wx, top, wz)
			}
		}
	}
	if err := os.WriteFile(out, sb, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d points to %s", len(sb)/12, out)
}

// stringBuilder lets Fprintf write into a []byte without importing bytes.
type stringBuilder struct{ b *[]byte }

func (s *stringBuilder) Write(p []byte) (int, error) {
	*s.b = append(*s.b, p...)
	return len(p), nil
}
