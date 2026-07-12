package world

// lightFingerprint is an FNV-1a hash over a chunk's sky and block light, used to
// detect whether a recompute actually changed anything (cheap relative to the
// BFS, ~98 KB per chunk).
func lightFingerprint(c *Chunk) uint64 {
	const (
		offset = uint64(14695981039346656037)
		prime  = uint64(1099511628211)
	)
	h := offset
	for i := range c.sections {
		for _, arr := range [2][]byte{c.sections[i].skyLight, c.sections[i].blockLight} {
			for _, b := range arr {
				h = (h ^ uint64(b)) * prime
			}
		}
	}
	return h
}

// lightPos is a world-coordinate position used during BFS propagation.
type lightPos struct {
	x, y, z int
}

// stepLoss is the sky/block light lost crossing into a block of the given
// opacity: at least 1 per block, more for translucent blocks. Matches vanilla's
// max(1, opacity).
func stepLoss(opacity uint8) uint8 {
	if opacity < 1 {
		return 1
	}
	return opacity
}

// lightOffsets are the 6 cardinal neighbour directions used by light BFS. The
// last entry {0,-1,0} (straight down) is special-cased for sky light.
var lightOffsets = [6][3]int{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {0, 1, 0}, {0, -1, 0}}

const lightTopY = MinWorldHeight + SectionsPerChunk*16 - 1 // highest canonical Y (319)

// getChunkUnlocked retrieves a chunk without acquiring locks.
// Used internally when the caller already holds the appropriate lock.
func (le *LightEngine) getChunkUnlocked(cx, cz int) *Chunk {
	return le.world.chunks[ChunkPos{cx, cz}]
}

// Helper methods for world-coordinate light access.
// These use getChunkUnlocked to avoid deadlocks when called from
// ComputeChunkLight (which is called with locks already held).

func (le *LightEngine) getSkyLightWorld(x, y, z int) uint8 {
	cx, cz := x>>4, z>>4
	chunk := le.getChunkUnlocked(cx, cz)
	if chunk == nil {
		return 0 // unloaded chunk
	}
	return chunk.GetSkyLight(x&15, y, z&15)
}

func (le *LightEngine) setSkyLightWorld(x, y, z int, val uint8) {
	cx, cz := x>>4, z>>4
	chunk := le.getChunkUnlocked(cx, cz)
	if chunk == nil {
		return // unloaded chunk
	}
	chunk.SetSkyLight(x&15, y, z&15, val)
}

func (le *LightEngine) getBlockLightWorld(x, y, z int) uint8 {
	cx, cz := x>>4, z>>4
	chunk := le.getChunkUnlocked(cx, cz)
	if chunk == nil {
		return 0 // unloaded chunk
	}
	return chunk.GetBlockLight(x&15, y, z&15)
}

func (le *LightEngine) setBlockLightWorld(x, y, z int, val uint8) {
	cx, cz := x>>4, z>>4
	chunk := le.getChunkUnlocked(cx, cz)
	if chunk == nil {
		return // unloaded chunk
	}
	chunk.SetBlockLight(x&15, y, z&15, val)
}

func (le *LightEngine) getBlockIDWorld(x, y, z int) int32 {
	cx, cz := x>>4, z>>4
	chunk := le.getChunkUnlocked(cx, cz)
	if chunk == nil {
		return AirID // unloaded chunk treated as air
	}
	return chunk.GetBlock(x&15, y, z&15).ID()
}

// computeHeightmap scans the chunk and records the highest non-air block per column.
func (le *LightEngine) computeHeightmap(chunk *Chunk, cx, cz int) {
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			highest := MinWorldHeight - 1
			for y := MinWorldHeight + SectionsPerChunk*16 - 1; y >= MinWorldHeight; y-- {
				blockID := chunk.GetBlock(x, y, z).ID()
				if blockID != AirID {
					highest = y
					break
				}
			}
			chunk.SetHeightmap(x, z, int32(highest))
		}
	}
}

// computeSkyLight recomputes sky light for a chunk from scratch using a single
// seeded BFS, matching vanilla semantics:
//   - every block above its column heightmap has direct sky access (15);
//   - light propagates losing max(1, opacity) per step, EXCEPT straight down into
//     a fully transparent block, which loses nothing — a sunlit vertical shaft
//     stays at 15 until something attenuating blocks it.
//
// Every one of the chunk's own sky-light cells is reinitialised here (15 above
// the heightmap, 0 below) before the BFS fills the rest, so a recompute after a
// block change correctly lowers stale-bright cells. Cross-chunk reads pull from
// already-loaded neighbours via the world-coord helpers, so a shared edge that
// was lit before its neighbour existed settles correctly once the neighbour is
// queued for relight (see World.queueNeighborRelight).
func (le *LightEngine) computeSkyLight(chunk *Chunk, cx, cz int) {
	var queue []lightPos

	// Seed: reinitialise the column and collect every sky-exposed source. Seeding
	// all exposed cells (not just the lowest) is required so light still reaches
	// under overhangs/floating terrain from the side.
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			heightY := int(chunk.GetHeightmap(x, z))
			worldX, worldZ := cx*16+x, cz*16+z
			for y := lightTopY; y > heightY; y-- {
				chunk.SetSkyLight(x, y, z, 15)
				queue = append(queue, lightPos{worldX, y, worldZ})
			}
			for y := heightY; y >= MinWorldHeight; y-- {
				chunk.SetSkyLight(x, y, z, 0)
			}
		}
	}

	// BFS: monotonically raise neighbour light until nothing can be brightened.
	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]
		cur := le.getSkyLightWorld(pos.x, pos.y, pos.z)
		if cur == 0 {
			continue
		}
		for _, off := range lightOffsets {
			nx, ny, nz := pos.x+off[0], pos.y+off[1], pos.z+off[2]
			if ny < MinWorldHeight || ny > lightTopY {
				continue
			}
			opacity := GetLightProps(le.getBlockIDWorld(nx, ny, nz)).Opacity

			var newLight uint8
			if off == [3]int{0, -1, 0} && cur == 15 && opacity == 0 {
				newLight = 15 // straight down through a transparent block: no loss
			} else if loss := stepLoss(opacity); cur > loss {
				newLight = cur - loss
			}

			if newLight > le.getSkyLightWorld(nx, ny, nz) {
				le.setSkyLightWorld(nx, ny, nz, newLight)
				queue = append(queue, lightPos{nx, ny, nz})
			}
		}
	}
}

// computeBlockLight recomputes block light for a chunk from scratch using BFS
// from emitters, losing max(1, opacity) per step. Every cell is reinitialised to
// its own emission first, so a recompute after a light source is removed
// correctly clears stale light.
func (le *LightEngine) computeBlockLight(chunk *Chunk, cx, cz int) {
	var queue []lightPos
	// Seed: reinitialise every cell to its own emission and collect emitters.
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			worldX, worldZ := cx*16+x, cz*16+z
			for y := MinWorldHeight; y <= lightTopY; y++ {
				emission := GetLightProps(chunk.GetBlock(x, y, z).ID()).Emission
				chunk.SetBlockLight(x, y, z, emission)
				if emission > 0 {
					queue = append(queue, lightPos{worldX, y, worldZ})
				}
			}
		}
	}

	// BFS: monotonically raise neighbour light until nothing can be brightened.
	for len(queue) > 0 {
		pos := queue[0]
		queue = queue[1:]
		cur := le.getBlockLightWorld(pos.x, pos.y, pos.z)
		if cur == 0 {
			continue
		}
		for _, off := range lightOffsets {
			nx, ny, nz := pos.x+off[0], pos.y+off[1], pos.z+off[2]
			if ny < MinWorldHeight || ny > lightTopY {
				continue
			}
			opacity := GetLightProps(le.getBlockIDWorld(nx, ny, nz)).Opacity

			var newLight uint8
			if loss := stepLoss(opacity); cur > loss {
				newLight = cur - loss
			}

			if newLight > le.getBlockLightWorld(nx, ny, nz) {
				le.setBlockLightWorld(nx, ny, nz, newLight)
				queue = append(queue, lightPos{nx, ny, nz})
			}
		}
	}
}
