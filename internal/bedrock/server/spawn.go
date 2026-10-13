package server

import (
	"math"

	lwworld "livingworld/internal/world"
)

// safeRestoredFeet validates a saved feet position before restoring it. A save
// may predate world changes (worldgen update, terrain edits, version bump), so
// the stored Y can now sit inside solid terrain. When the saved body has solid
// ground below and two air blocks for feet/head, it is used verbatim;
// otherwise the position snaps to the column's current surface.
func safeRestoredFeet(w *lwworld.World, x, y, z float64) (float64, float64, float64) {
	bx, bz := int(math.Floor(x)), int(math.Floor(z))
	by := int(math.Floor(y))
	if by >= lwworld.MinWorldHeight && by+1 < lwworld.MinWorldHeight+lwworld.SectionsPerChunk*16 {
		below := w.GetBlock(bx, by-1, bz).ID()
		feet := w.GetBlock(bx, by, bz).ID()
		head := w.GetBlock(bx, by+1, bz).ID()
		if below != lwworld.AirID && feet == lwworld.AirID && head == lwworld.AirID {
			return x, y, z
		}
	}
	// Snap to the current surface of the column (center-padded like server.New).
	return float64(bx) + 0.5, float64(w.HighestSolidY(bx, bz)), float64(bz) + 0.5
}
