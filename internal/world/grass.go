package world

import (
	"math/rand"
)

// Vanilla grass random ticks. Grass is driven from the source grass block, not
// from random dirt samples: a random-ticked grass block first checks whether it
// can survive, then rolls vanilla's spread volume. LivingWorld only accepts
// targets on the same surface layer so grass does not jump downward into buried
// dirt, but the vertical roll is kept so the spread chance stays vanilla-like.
const (
	// defaultRandomTickSpeed matches vanilla's default gamerule. LivingWorld does
	// not expose /gamerule randomTickSpeed yet, so grass uses the vanilla default.
	defaultRandomTickSpeed = 3

	// grassSpreadAttempts is the vanilla number of spread attempts made by one
	// random-ticked grass block.
	grassSpreadAttempts = 4

	// grassSurviveMinLight is the minimum local light above grass before it dies
	// back to dirt. This is the observable vanilla threshold for covered grass.
	grassSurviveMinLight = 4

	// grassSpreadMinLight is the minimum local light above the source/target for
	// grass to spread.
	grassSpreadMinLight = 9
)

// grassTick is Phase 3 of the unified tick. It runs the random-tick
// budget for grass spread on every loaded world. Safe to call from the
// tick goroutine; it uses the world manager's RNG so seeded worlds stay
// deterministic.
func (m *Manager) grassTick(rng *rand.Rand) {
	if rng == nil {
		return
	}
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

// grassTickWorld runs the grass random-tick budget for a single world. It
// samples random positions per loaded chunk section using the vanilla default
// randomTickSpeed and executes grass behaviour only when the sampled block is a
// grass block. Sampling uses already-loaded chunks only; random ticks must not
// generate chunks at a loaded chunk border.
func (m *Manager) grassTickWorld(rng *rand.Rand, w *World) {
	if rng == nil || w == nil {
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
	for _, cp := range chunkPositions {
		for section := 0; section < SectionsPerChunk; section++ {
			for i := 0; i < defaultRandomTickSpeed; i++ {
				bx := cp.WorldX + rng.Intn(ChunkSize)
				by := MinWorldHeight + section*16 + rng.Intn(16)
				bz := cp.WorldZ + rng.Intn(ChunkSize)
				id, ok := loadedBlockID(w, bx, by, bz)
				if !ok || id != grassID {
					continue
				}
				m.randomTickGrassBlock(rng, w, bx, by, bz, dirtID, grassID)
			}
		}
	}
}

func (m *Manager) randomTickGrassBlock(rng *rand.Rand, w *World, x, y, z int, dirtID, grassID int32) {
	if !grassCanSurvive(w, x, y, z) {
		m.setWorldBlockAndPublish(w, x, y, z, BlockByID(dirtID))
		return
	}
	if localLightAbove(w, x, y, z) < grassSpreadMinLight {
		return
	}
	for i := 0; i < grassSpreadAttempts; i++ {
		tx := x + rng.Intn(3) - 1
		ty := y + rng.Intn(5) - 3
		tz := z + rng.Intn(3) - 1
		if ty != y {
			continue
		}
		id, ok := loadedBlockID(w, tx, ty, tz)
		if !ok || id != dirtID {
			continue
		}
		if !grassCanSpreadTo(w, tx, ty, tz) {
			continue
		}
		m.setWorldBlockAndPublish(w, tx, ty, tz, BlockByID(grassID))
	}
}

func (m *Manager) setWorldBlockAndPublish(w *World, x, y, z int, block Block) {
	w.SetBlock(x, y, z, block)
	m.PublishBlockUpdate(BlockUpdateSourceServer, x, y, z, block.ID())
}

func loadedBlockID(w *World, x, y, z int) (int32, bool) {
	c := w.GetChunk(x>>4, z>>4)
	if c == nil {
		return AirID, false
	}
	return c.GetBlock(x&15, y, z&15).ID(), true
}

func grassCanSurvive(w *World, x, y, z int) bool {
	aboveID, ok := loadedBlockID(w, x, y+1, z)
	if !ok {
		return false
	}
	if GetLightProps(aboveID).Opacity >= 15 {
		return false
	}
	return localLightAbove(w, x, y, z) >= grassSurviveMinLight
}

func grassCanSpreadTo(w *World, x, y, z int) bool {
	if !grassCanSurvive(w, x, y, z) {
		return false
	}
	return localLightAbove(w, x, y, z) >= grassSpreadMinLight
}

func localLightAbove(w *World, x, y, z int) uint8 {
	sky := w.GetSkyLight(x, y+1, z)
	block := w.GetBlockLight(x, y+1, z)
	if sky > block {
		return sky
	}
	return block
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
