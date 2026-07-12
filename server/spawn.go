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
	isLand := func(px, pz int) bool {
		y := w.HighestSolidY(px, pz) // feet Y, one above the surface block
		if y <= 63 {
			return false
		}
		below := w.GetBlock(px, y-1, pz).ID()
		return below != waterID && below != iceID
	}
	if isLand(x, z) {
		return x, z
	}
	for radius := 16; radius <= 160; radius += 16 {
		for dx := -radius; dx <= radius; dx += 16 {
			for dz := -radius; dz <= radius; dz += 16 {
				// Ring only: skip interior points already scanned.
				if dx > -radius && dx < radius && dz > -radius && dz < radius {
					continue
				}
				if isLand(x+dx, z+dz) {
					return x + dx, z + dz
				}
			}
		}
	}
	return x, z
}
