package world

import (
	"math/rand"
)

// Vanilla grass spread. In 26.1 every random tick a chunk's
// `randomTick` can promote a neighbouring `dirt` to `grass_block` if the
// block above is transparent and at least one of the four horizontal
// neighbours is already `grass_block`. We approximate that with a
// per-tick budget of `grassRandomTicksPerTick` random samples per
// world — enough to keep the surface "alive" (visible spreading within
// a few seconds when you park a grass block next to a dirt patch) but
// cheap enough that the 20 Hz tick stays well under a millisecond of
// CPU even on a busy world.
const (
	// grassRandomTicksPerTick is the number of random block samples run
	// per world per 20 Hz tick. Vanilla does ~3 per sub-chunk per ~40
	// ticks; we flatten that across a whole world to a small fixed
	// number so a 1-chunk world and a 100-chunk world both spread grass
	// at a similar visible rate.
	grassRandomTicksPerTick = 64

	// grassSpreadChance is the probability that a sampled dirt block
	// that meets the spread conditions actually turns to grass. Vanilla
	// uses 1/4, which gives the "gradual" feel of watching grass creep
	// outwards.
	grassSpreadChance = 0.25
)

// grassTick is Phase 3 of the unified tick. It runs the random-tick
// budget for grass spread on every loaded world. Safe to call from the
// tick goroutine; it uses the world manager's RNG so seeded worlds stay
// deterministic.
func (m *Manager) grassTick(rng *rand.Rand) {
	m.mu.RLock()
	worlds := make([]*World, 0, len(m.worlds))
	for _, w := range m.worlds {
		worlds = append(worlds, w)
	}
	m.mu.RUnlock()
	for _, w := range worlds {
		m.grassTickWorld(rng, w)
	}
}

// grassTickWorld runs the random-tick budget for a single world. It
// samples random block positions across all loaded chunks and applies
// the vanilla grass-promotion rule. A sampled block that is not
// eligible is silently skipped so the budget effectively becomes
// "eligible samples" — with the dirt-vs-grass ratio in a real world
// that means the budget is dominated by dirt/grass blocks anyway.
func (m *Manager) grassTickWorld(rng *rand.Rand, w *World) {
	if w == nil {
		return
	}
	dirtID := StateID("minecraft:dirt")
	grassID := StateID("minecraft:grass_block")
	if dirtID == AirID || grassID == AirID {
		// Palette missing the names — nothing to do.
		return
	}
	chunkPositions := w.loadedChunkPositions()
	if len(chunkPositions) == 0 {
		return
	}
	for i := 0; i < grassRandomTicksPerTick; i++ {
		cp := chunkPositions[rng.Intn(len(chunkPositions))]
		bx := cp.WorldX + rng.Intn(ChunkSize)
		by := MinWorldHeight + rng.Intn(MaxWorldHeight-MinWorldHeight)
		bz := cp.WorldZ + rng.Intn(ChunkSize)
		// Only attempt spread on the top of a column — grass can only
		// appear where sunlight hits, which is the column surface. The
		// +1..+2 sample of `by` cheaply biases the random pool towards
		// the top of the world without needing a heightmap lookup.
		top := w.HeightmapTop(bx, bz)
		if top < 0 {
			continue
		}
		// Sample at the surface or one block below it: the grass-spread
		// check looks at "the dirt block below a transparent block", so
		// either of those two is the right cell to test.
		if by < top-1 || by > top {
			continue
		}
		// Cheap bail-out before reading the chunk: if the block is not
		// even dirt, skip the whole neighbour walk.
		if w.GetBlock(bx, by, bz).ID() != dirtID {
			continue
		}
		// The block above must be air (or otherwise transparent — we
		// model air as the only case in the worldgen output).
		if w.GetBlock(bx, by+1, bz).ID() != AirID {
			continue
		}
		// At least one horizontal neighbour must already be grass. If
		// not, the sample is wasted but the budget is small enough to
		// absorb that.
		hasGrass := false
		if w.GetBlock(bx+1, by, bz).ID() == grassID {
			hasGrass = true
		} else if w.GetBlock(bx-1, by, bz).ID() == grassID {
			hasGrass = true
		} else if w.GetBlock(bx, by, bz+1).ID() == grassID {
			hasGrass = true
		} else if w.GetBlock(bx, by, bz-1).ID() == grassID {
			hasGrass = true
		}
		if !hasGrass {
			continue
		}
		if rng.Float64() >= grassSpreadChance {
			continue
		}
		// Promote: use the world manager so the change is published
		// across the world bus (so both Java and Bedrock viewers update
		// the block visually).
		m.SetBlockAndPublish(BlockUpdateSourceServer, bx, by, bz, BlockByID(grassID))
	}
}

// chunkWorldPos is a world-coordinate top-left corner of a loaded chunk.
type chunkWorldPos struct {
	WorldX int
	WorldZ int
}

// loadedChunkPositions snapshots the world-X/Z of every loaded chunk in
// the world. It takes a read lock briefly to copy, so callers can
// iterate the slice without holding the world lock. Used by grassTick
// (and any future random-tick work) to avoid needing an iterator that
// re-locks for every sample.
func (w *World) loadedChunkPositions() []chunkWorldPos {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if len(w.chunks) == 0 {
		return nil
	}
	out := make([]chunkWorldPos, 0, len(w.chunks))
	for pos := range w.chunks {
		out = append(out, chunkWorldPos{
			WorldX: pos.X * ChunkSize,
			WorldZ: pos.Z * ChunkSize,
		})
	}
	return out
}

// HeightmapTop returns the highest non-air world-Y at column (x, z), or
// -1 if the column has no loaded chunks yet. Cheaper than walking
// every Y just to know which layer to sample.
func (w *World) HeightmapTop(x, z int) int {
	cx, cz := ChunkCoord(float64(x)), ChunkCoord(float64(z))
	c := w.GetChunk(int(cx), int(cz))
	if c == nil {
		return -1
	}
	// GetHeightmap uses chunk-local X/Z (0..15), so reduce.
	lx, lz := x&15, z&15
	if lx < 0 {
		lx += ChunkSize
	}
	if lz < 0 {
		lz += ChunkSize
	}
	h := c.GetHeightmap(lx, lz)
	return int(h)
}
