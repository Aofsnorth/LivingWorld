package server

import "livingworld/internal/world"

// surfaceY returns the Y a player should stand on at column (x,z): one block
// above the highest non-air block. It calls LoadChunk so the column is
// generated before scanning — using GetBlock alone reads an unloaded (nil)
// chunk that reports all air, which made the player spawn embedded in terrain.
func surfaceY(wm *world.Manager, x, z int) int {
	w := wm.GetDefaultWorld()
	w.LoadChunk(x>>4, z>>4)
	// Scan from the top placeable Y (319) — MaxWorldHeight is a HEIGHT (384), not a
	// max-Y, so the old MaxWorldHeight-1 (383) scanned 64 rows above the column.
	for y := world.MinWorldHeight + world.SectionsPerChunk*16 - 1; y >= world.MinWorldHeight; y-- {
		if w.GetBlock(x, y, z).ID() != 0 {
			return y + 1
		}
	}
	return world.MinWorldHeight
}
