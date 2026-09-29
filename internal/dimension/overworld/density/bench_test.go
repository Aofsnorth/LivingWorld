package density

import (
	"testing"

	"livingworld/internal/dimension/overworld/vanilladata"
)

// BenchmarkOverworldDensityGrid sizes the real chunk workload: sampling the
// final_density tree at the 5x5x49 noise-cell corners of one chunk with a
// shared per-chunk cache.
func BenchmarkOverworldDensityGrid(b *testing.B) {
	raw, ok := vanilladata.LoadNoiseSettings("overworld")
	if !ok {
		b.Fatal("no overworld noise settings")
	}
	ctx := NewContext(12345, NoiseSet{})
	tree, err := ParseFinalDensity(ctx, raw)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dc := ctx.Fresh()
		for xi := 0; xi <= 4; xi++ {
			for zi := 0; zi <= 4; zi++ {
				for yi := 0; yi <= 48; yi++ {
					_ = tree.Eval(dc, xi*4, -64+yi*8, zi*4)
				}
			}
		}
	}
}

// BenchmarkOverworldFinalDensityEval sizes a single final_density tree
// evaluation — the unit cost of vanilla-driven terrain shaping. The caching
// strategy in the chunk pipeline is chosen around this number.
func BenchmarkOverworldFinalDensityEval(b *testing.B) {
	raw, ok := vanilladata.LoadNoiseSettings("overworld")
	if !ok {
		b.Fatal("no overworld noise settings")
	}
	ctx := NewContext(12345, NoiseSet{})
	tree, err := ParseFinalDensity(ctx, raw)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		x := i % 64
		z := (i / 64) % 64
		_ = tree.Eval(ctx, x*4, 60, z*4)
	}
}
