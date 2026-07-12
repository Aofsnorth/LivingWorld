package world

// HighestSolidY returns the Y just above the highest non-air block in column
// (x,z) — a safe feet position to spawn on for any generator (superflat returns
// 4; overworld returns the terrain surface). The column's chunk is loaded
// (generated if needed); falls back to 64 for an all-air column.
func (w *World) HighestSolidY(x, z int) int {
	w.LoadChunk(x>>4, z>>4)
	// Scan the full canonical column from the top placeable Y (319) down to the
	// world floor (-64) so both overworld surfaces and any sub-0 floor are found.
	for y := MinWorldHeight + SectionsPerChunk*16 - 1; y >= MinWorldHeight; y-- {
		if w.GetBlock(x, y, z).ID() != AirID {
			return y + 1
		}
	}
	return 64
}

func (w *World) SetBlock(x, y, z int, block Block) {
	chunkX, chunkZ := x>>4, z>>4
	chunk := w.LoadChunk(chunkX, chunkZ)
	chunk.SetBlock(x&15, y, z&15, block)
	// Phase 4b: queue light recomputation for the affected chunk.
	// Light is processed during the tick (phase 3), not synchronously,
	// so multiple block changes in one tick only trigger one recompute.
	if w.light != nil {
		w.light.QueueUpdate(chunkX, chunkZ)
	}
}

func (w *World) GetBlock(x, y, z int) Block {
	// Load-on-access: a block can't be read without its chunk being present, so
	// fall through to disk/generation rather than reporting air for an unloaded
	// (but possibly edited and persisted) chunk.
	chunk := w.LoadChunk(x>>4, z>>4)
	if chunk == nil {
		return BlockAir{}
	}
	return chunk.GetBlock(x&15, y, z&15)
}

// GetSkyLight returns the sky light level (0-15) at the given world coordinates.
// Phase 4b: used by mob spawning, rendering, and gameplay systems.
func (w *World) GetSkyLight(x, y, z int) uint8 {
	chunk := w.GetChunk(x>>4, z>>4)
	if chunk == nil {
		return 15 // unloaded chunks default to full sky light
	}
	return chunk.GetSkyLight(x&15, y, z&15)
}

// GetBlockLight returns the block light level (0-15) at the given world coordinates.
// Phase 4b: used by mob spawning, rendering, and gameplay systems.
func (w *World) GetBlockLight(x, y, z int) uint8 {
	chunk := w.GetChunk(x>>4, z>>4)
	if chunk == nil {
		return 0 // unloaded chunks default to no block light
	}
	return chunk.GetBlockLight(x&15, y, z&15)
}

// SetSkyLight sets the sky light level at the given world
// coordinates. Loads the chunk if needed. M2: used by tests
// and the future M2.2 night-cycle rule helper to seed
// dark-canyon / open-sky columns.
func (w *World) SetSkyLight(x, y, z int, val uint8) {
	chunk := w.GetChunk(x>>4, z>>4)
	if chunk == nil {
		return
	}
	chunk.SetSkyLight(x&15, y, z&15, val)
}

// SetBlockLight sets the block light level at the given world
// coordinates. Loads the chunk if needed. M2: used by tests
// to seed a column with block light (e.g. to make sure the
// "RequireDark" rule still sees dark sky light even with
// block light > 0).
func (w *World) SetBlockLight(x, y, z int, val uint8) {
	chunk := w.GetChunk(x>>4, z>>4)
	if chunk == nil {
		return
	}
	chunk.SetBlockLight(x&15, y, z&15, val)
}
