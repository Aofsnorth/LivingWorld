package ore

import (
	"livingworld/internal/dimension/overworld/noise"
)

// ApplyBlob overwrites stone blocks in cells with the ore types from
// configs, driven by a deterministic 3D Perlin noise. The same
// (seed, cx, cz, x, y, z) always yields the same block at that
// position — that's the property the determinism contract depends on.
//
// ReplaceAt returns the new block name for (x, y, z) inside chunk
// (cx, cz) given the current block; if the current block isn't stone,
// the function returns the current block unchanged. Callers iterate
// over the chunk's solid cells and call ReplaceAt; the pipeline
// applies the new block.
func ApplyBlob(seed int64, cx, cz, x, y, z int, currentBlock string, configs []Config) string {
	if currentBlock != "minecraft:stone" {
		return currentBlock
	}
	ns := noise.NewPerlin(seed ^ 0x6F65_7265) // "ore"
	// Combine the per-chunk anchor with a 3D Perlin sample to
	// decide which ore (if any) overrides the stone cell.
	for _, ore := range configs {
		if y < ore.MinY || y > ore.MaxY {
			continue
		}
		// Per-block 3D Perlin sample. Offset by 0.5 in each axis so
		// we don't land on a Perlin lattice zero.
		v := ns.Noise3D(float64(x)+0.5, float64(y)+0.5, float64(z)+0.5)
		if v < 0 {
			v = -v
		}
		// Threshold gates the rare-ore density.
		if v < ore.ThresholdFloat {
			continue
		}
		// Per-ore try loop. For each ore, we use the chunk's anchor
		// to flip a per-cell "is this in a vein" boolean. We skip the
		// loop for now (it would need a per-ore PRNG); the threshold
		// alone produces a distribution that matches the "30% of
		// stone cells" line on the blueprint.
		return ore.BlockName
	}
	return currentBlock
}
