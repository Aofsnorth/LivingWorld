package server

import (
	"livingworld/internal/world"
)

// zeroUUID returns a [16]byte of zeros. Used as a placeholder UUID when
// emitting a side-effect event whose primary subject is an entityID
// (e.g. a mob swing) and not a player.
func zeroUUID() [16]byte { return [16]byte{} }

// findLandSpawn searches outward from (x, z) for a column whose surface
// is dry land (above sea level and not water/ice). Overworld seeds can
// put the configured spawn in the middle of an ocean; vanilla performs
// the same kind of search. Scans rings every 16 blocks up to ~160
// blocks out and falls back to the original point if everything is wet.
func findLandSpawn(w *world.World, x, z int) (int, int) {
	waterID := world.WaterID
	iceID := world.StateID("minecraft:ice")
	isLand := func(px, pz int, evictIfWet bool) bool {
		cx, cz := px>>4, pz>>4
		wasLoaded := w.GetChunk(cx, cz) != nil
		y := w.HighestSolidY(px, pz) // feet Y, one above the surface block
		if y <= 63 {
			if evictIfWet && !wasLoaded {
				w.EvictChunk(cx, cz)
			}
			return false
		}
		below := w.GetBlock(px, y-1, pz).ID()
		ok := below != waterID && below != iceID
		if !ok && evictIfWet && !wasLoaded {
			w.EvictChunk(cx, cz)
		}
		return ok
	}
	if isLand(x, z, false) {
		return x, z
	}
	// Probe 8 cardinal + diagonal directions per ring in 32-block (2-chunk) steps
	// so ocean spawn searches probe at most 40 chunks instead of 441, and evict
	// rejected ocean chunks immediately to keep startup RAM low.
	dirs := [8][2]int{
		{1, 0}, {-1, 0}, {0, 1}, {0, -1},
		{1, 1}, {1, -1}, {-1, 1}, {-1, -1},
	}
	for radius := 32; radius <= 160; radius += 32 {
		for _, d := range dirs {
			px, pz := x+d[0]*radius, z+d[1]*radius
			if isLand(px, pz, true) {
				return px, pz
			}
		}
	}
	return x, z
}
