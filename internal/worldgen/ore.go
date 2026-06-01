// Package worldgen — ore.go
//
// Phase 4c worldgen depth: ore distribution over the existing terrain
// pipeline. Implemented as a post-Build pass on the *world.Chunk so the
// terrain package's foundation-free architecture stays intact.
//
// VANILLA-ISH HEIGHT BANDS (cited inline per Master_Plan §6 Phase 4c):
//
//   coal     Y -64..320  (common, everywhere stone exists)
//   iron     Y -64..320  (common; more common below 64)
//   gold     Y -64..32   (sparser; mesa-ish)
//   redstone Y -64..16   (sparser, technical-mining layer)
//   diamond  Y -64..16   (rare, the iconic deep-ore)
//   lapis    Y -64..64   (decorative, mid-rare)
//   emerald  Y -16..320  (very rare, mountain-only stub)
//
// The bands are *intent* matches for vanilla, not byte-level parity — the
// plan explicitly calls out that "perfect byte-level parity may require
// long-term work" (Master_Plan §10.3). What matters for parity is that
// the placement is deterministic from (seed, x, y, z) so two runs with
// the same seed produce the same chunk content.

package worldgen

import (
	"livingworld/internal/world"
	"livingworld/internal/worldgen/noise"
)

// oreConfig describes one ore vein type. Vein tries per chunk is the number
// of candidate vein anchors we attempt; once a vein is placed, we fill an
// ellipsoidal blob of `VeinSize` blocks around the anchor.
//
// ThresholdFloat is the per-block probability threshold on the absolute
// value of a 3D Perlin sample. Perlin output is in roughly [-1, 1] and
// clusters around 0, so ThresholdFloat = 0.4 accepts roughly 30% of
// stone cells. Higher = rarer.
type oreConfig struct {
	BlockName      string
	MinY, MaxY     int
	VeinTries      int     // per chunk, how many candidate vein anchors
	VeinSize       int     // approximate blocks per vein (1 = single block)
	ThresholdFloat float64 // 0..1, higher = rarer
	MountainOnly   bool
}

// oreTable is the canonical ore list. Order matters for the test pins
// (deterministic results across runs of the same seed).
var oreTable = []oreConfig{
	{BlockName: "minecraft:coal_ore", MinY: -64, MaxY: 320, VeinTries: 20, VeinSize: 8, ThresholdFloat: 0.40},
	{BlockName: "minecraft:iron_ore", MinY: -64, MaxY: 320, VeinTries: 12, VeinSize: 6, ThresholdFloat: 0.45},
	{BlockName: "minecraft:gold_ore", MinY: -64, MaxY: 32, VeinTries: 6, VeinSize: 6, ThresholdFloat: 0.55},
	{BlockName: "minecraft:redstone_ore", MinY: -64, MaxY: 16, VeinTries: 6, VeinSize: 6, ThresholdFloat: 0.55},
	{BlockName: "minecraft:diamond_ore", MinY: -64, MaxY: 16, VeinTries: 4, VeinSize: 5, ThresholdFloat: 0.65},
	{BlockName: "minecraft:lapis_ore", MinY: -64, MaxY: 64, VeinTries: 3, VeinSize: 5, ThresholdFloat: 0.60},
	// Emerald is "mountain only" in vanilla; we don't have a biome lookup
	// here, so the mountain-only flag is wired for Phase 5a to gate
	// against the biome map. Today it falls through to the generic
	// branch and is still rare.
	{BlockName: "minecraft:emerald_ore", MinY: -16, MaxY: 320, VeinTries: 2, VeinSize: 1, ThresholdFloat: 0.75, MountainOnly: true},
}

// applyOres overwrites stone blocks in c with the ore types from oreTable,
// driven by a deterministic 3D Perlin noise. The same (seed, cx, cz, x, y,
// z) always yields the same block at that position — that's the property
// the determinism test pins.
func applyOres(seed int64, cx, cz int, c *world.Chunk) {
	stoneID := world.StateID("minecraft:stone")
	// One 3D noise sampler shared by all ore types so the cost of one
	// chunk is one Perlin setup, not seven.
	ns := noise.NewPerlin(seed ^ 0x6f65_7265) // 0x6f65_7265 = "ore"
	for _, ore := range oreTable {
		// Skip unknown ore names so a future palette rename doesn't
		// crash the generator.
		oreID := world.StateID(ore.BlockName)
		if oreID == world.AirID {
			continue
		}
		for t := 0; t < ore.VeinTries; t++ {
			// Vein anchor: deterministic from (seed, ore index, t, chunk).
			// Modulo into 0..15 so the anchor is inside the chunk, not a
			// 32-block offset. (An anchor outside the chunk is fine in
			// vanilla because the world fills an ellipsoid that
			// overlaps this chunk; we just keep things local for now.)
			axRaw := ns.Noise3D(float64(cx*32+int(seed)%31+t), float64(cz*32+int(seed)%17+t*7), 0)
			ayRaw := ns.Noise3D(float64(cx*32+t*3), float64(cz*32+t*5), float64(t*11))
			azRaw := ns.Noise3D(float64(cx*32+t*2), float64(cz*32+t*13), float64(t*17))
			ax := int(axRaw*8+8) & 15 // map [-1,1] -> [0,15]
			az := int(azRaw*8+8) & 15
			ay := ore.MinY + int((float64(ore.MaxY-ore.MinY))*(ayRaw+1)/2)
			// Fill a small ellipsoid around the anchor.
			for dx := -ore.VeinSize; dx <= ore.VeinSize; dx++ {
				for dy := -ore.VeinSize; dy <= ore.VeinSize; dy++ {
					for dz := -ore.VeinSize; dz <= ore.VeinSize; dz++ {
						// Cheap ellipsoid filter.
						if dx*dx+dy*dy*2+dz*dz > ore.VeinSize*ore.VeinSize {
							continue
						}
						x, y, z := ax+dx, ay+dy, az+dz
						if !inChunk(x, y, z) {
							continue
						}
						// Replace stone only. The terrain pipeline writes
						// "minecraft:stone" everywhere underground; cave
						// carving leaves Air cells which we leave alone.
						// We probe by id (the canonical stone state id is
						// 1, which world.StateID("minecraft:stone") returns)
						// rather than by name, to keep the hot loop cheap.
						if c.GetBlock(x, y, z).ID() != stoneID {
							continue
						}
						// Place the ore here if the noise sample exceeds
						// the per-ore threshold. We treat the noise output
						// (in roughly [-1, 1]) as a probability scale: a
						// threshold of 0.4 means "top 30% of the |noise|
						// distribution" which Perlin produces in roughly
						// 30% of stone cells. The threshold is the per-ore
						// "common" line; higher = rarer.
						//
						// NB: Perlin noise returns exactly 0 at integer
						// lattice points (the gradient contributions cancel
						// by construction). Block coordinates ARE integers,
						// so we add a half-step offset to land in a
						// non-zero region of the noise field. This is a
						// standard Perlin idiom; see any reference impl.
						v := ns.Noise3D(float64(x)+0.5, float64(y)+0.5, float64(z)+0.5)
						if v < 0 {
							v = -v
						}
						if v < ore.ThresholdFloat {
							continue
						}
						c.SetBlock(x, y, z, world.BlockByID(oreID))
					}
				}
			}
		}
	}
}

// inChunk reports whether (x,y,z) is within a chunk's block range (0..15
// on x/z, canonical world-Y -64..319).
func inChunk(x, y, z int) bool {
	if x < 0 || x > 15 || z < 0 || z > 15 {
		return false
	}
	if y < world.MinWorldHeight || y > world.MinWorldHeight+16*world.SectionsPerChunk-1 {
		return false
	}
	return true
}
