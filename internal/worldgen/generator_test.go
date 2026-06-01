// Package worldgen tests: Phase 4c worldgen depth (ore + determinism).
//
// The pinning test is the seed-determinism one: same seed → same chunk
// content. That is the Advance.md §10.4 contract for "cross-edition chunk
// equality" and the foundation for the Phase 7c parity harness.
package worldgen

import (
	"testing"
)

// TestGeneratorDeterminism: the same seed must produce the same chunk at
// (cx, cz) on two independent runs. We compare the block IDs at a sample
// of positions; one mismatch fails the test.
func TestGeneratorDeterminism(t *testing.T) {
	const seed = int64(12345)
	g1 := NewGenerator(seed)
	g2 := NewGenerator(seed)

	c1 := g1.Generate(0, 0)
	c2 := g2.Generate(0, 0)

	mismatch := 0
	for y := -64; y < 320; y++ {
		for z := 0; z < 16; z++ {
			for x := 0; x < 16; x++ {
				if c1.GetBlock(x, y, z).ID() != c2.GetBlock(x, y, z).ID() {
					mismatch++
				}
			}
		}
	}
	if mismatch > 0 {
		t.Errorf("Generator.Generate: same seed produced different chunks (%d block mismatches at chunk (0,0))", mismatch)
	}
}

// TestGeneratorDifferentChunks: different chunk coords must produce
// different content. This is the negative control: if the generator
// returned the same chunk for every cx/cz, the determinism test above
// would pass for the wrong reason.
//
// We don't pin an exact fraction of mismatches because the underground
// is largely contiguous stone that varies slowly with the noise; we
// only assert that the two chunks are not byte-identical, which would
// indicate the chunk coordinates are being ignored.
func TestGeneratorDifferentChunks(t *testing.T) {
	const seed = int64(12345)
	g := NewGenerator(seed)
	c1 := g.Generate(0, 0)
	c2 := g.Generate(1, 1)

	mismatch := 0
	for y := -64; y < 320; y++ {
		for z := 0; z < 16; z++ {
			for x := 0; x < 16; x++ {
				if c1.GetBlock(x, y, z).ID() != c2.GetBlock(x, y, z).ID() {
					mismatch++
				}
			}
		}
	}
	if mismatch == 0 {
		t.Errorf("chunks (0,0) and (1,1) are byte-identical — generator is ignoring chunk coordinates")
	}
}

// TestOresPresent: at least one of the documented ore types should be
// present in a generated chunk from seed 12345. The exact count varies
// with the noise; we just assert "some ore is there" so a regression
// that disables applyOres is caught.
func TestOresPresent(t *testing.T) {
	const seed = int64(12345)
	g := NewGenerator(seed)
	c := g.Generate(0, 0)

	oreCounts := map[string]int{}
	for y := -64; y < 320; y++ {
		for z := 0; z < 16; z++ {
			for x := 0; x < 16; x++ {
				name := stateNameForTest(c.GetBlock(x, y, z).ID())
				switch name {
				case "minecraft:coal_ore", "minecraft:iron_ore", "minecraft:gold_ore",
					"minecraft:redstone_ore", "minecraft:diamond_ore", "minecraft:lapis_ore",
					"minecraft:emerald_ore":
					oreCounts[name]++
				}
			}
		}
	}
	total := 0
	for _, n := range oreCounts {
		total += n
	}
	if total == 0 {
		t.Errorf("no ores found in chunk (0,0) for seed 12345 — applyOres may not be wired")
	}
	t.Logf("ore counts in (0,0) for seed 12345: %v (total=%d)", oreCounts, total)
}

// stateNameForTest is a thin wrapper around world.StateName so the test
// doesn't import the world package directly. The import cycle would be
// manageable, but the test is more readable this way.
func stateNameForTest(id int32) string {
	// We can't import world here without a cycle, so use the package's
	// own lookup. NewGenerator returns a *Generator which can answer
	// block names indirectly through the world package — but the
	// Generator itself doesn't expose a name lookup. Round-trip via
	// the chunk's existing accessor. Simpler: just defer to a tiny
	// helper we add to this file. Implementation note: world.StateName
	// is the canonical inverse of world.StateID, so we route through
	// it via a one-line helper. For the test, we accept the indirection.
	// The real name lookup is a single map[string]int32 in world/.
	return worldStateName(id)
}