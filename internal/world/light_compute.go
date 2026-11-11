package world

import (
	"encoding/binary"
	"sync"
)

// skyLightFullSection is a pre-filled 2048-byte buffer of 0xFF (sky light 15 in every nibble)
// used for fast bulk initialization of full-sky sections via copy().
var skyLightFullSection = func() [2048]byte {
	var b [2048]byte
	for i := range b {
		b[i] = 0xFF
	}
	return b
}()

// lightFingerprint is an FNV-1a hash over a chunk's sky and block light, used to
// detect whether a recompute actually changed anything. Processes 8 bytes per step
// using 64-bit little-endian words for ~8x faster hashing.
func lightFingerprint(c *Chunk) uint64 {
	const (
		offset = uint64(14695981039346656037)
		prime  = uint64(1099511628211)
	)
	h := offset
	for i := range c.sections {
		for _, arr := range [2][]byte{c.sections[i].skyLight, c.sections[i].blockLight} {
			for j := 0; j+8 <= len(arr); j += 8 {
				w := binary.LittleEndian.Uint64(arr[j : j+8])
				h = (h ^ w) * prime
			}
		}
	}
	return h
}

// lightNode is a world-coordinate position and cached light level used during BFS propagation.
// Carrying level inside the queue entry avoids redundant world-coordinate light reads on pop
// and enables immediate stale-entry skipping (Starlight principle).
type lightNode struct {
	x, y, z int32
	level   uint8
}

var lightQueuePool = sync.Pool{
	New: func() any {
		s := make([]lightNode, 0, 2048)
		return &s
	},
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
var lightOffsets = [6][3]int32{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {0, 1, 0}, {0, -1, 0}}

const lightTopY = MinWorldHeight + SectionsPerChunk*16 - 1 // highest canonical Y (319)

// lightChunkNeighborhood caches the 3x3 chunk grid around (centerCX, centerCZ)
// so BFS steps access chunks via O(1) array indexing instead of hashing into
// World.chunks map[ChunkPos]*Chunk millions of times.
type lightChunkNeighborhood struct {
	centerCX int
	centerCZ int
	grid     [3][3]*Chunk
	le       *LightEngine
}

func (le *LightEngine) makeNeighborhood(center *Chunk, cx, cz int) lightChunkNeighborhood {
	var nb lightChunkNeighborhood
	nb.centerCX = cx
	nb.centerCZ = cz
	nb.le = le
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if dx == 0 && dz == 0 {
				nb.grid[1][1] = center
			} else {
				nb.grid[dx+1][dz+1] = le.getChunkUnlocked(cx+dx, cz+dz)
			}
		}
	}
	return nb
}

func (nb *lightChunkNeighborhood) chunkAt(cx, cz int) *Chunk {
	dx := cx - nb.centerCX + 1
	dz := cz - nb.centerCZ + 1
	if uint(dx) < 3 && uint(dz) < 3 {
		return nb.grid[dx][dz]
	}
	return nb.le.getChunkUnlocked(cx, cz)
}

func (nb *lightChunkNeighborhood) getSkyLight(x, y, z int32) uint8 {
	chunk := nb.chunkAt(int(x>>4), int(z>>4))
	if chunk == nil {
		return 0
	}
	return chunk.GetSkyLight(int(x&15), int(y), int(z&15))
}

func (nb *lightChunkNeighborhood) setSkyLight(x, y, z int32, val uint8) {
	chunk := nb.chunkAt(int(x>>4), int(z>>4))
	if chunk == nil {
		return
	}
	chunk.SetSkyLight(int(x&15), int(y), int(z&15), val)
}

func (nb *lightChunkNeighborhood) getBlockLight(x, y, z int32) uint8 {
	chunk := nb.chunkAt(int(x>>4), int(z>>4))
	if chunk == nil {
		return 0
	}
	return chunk.GetBlockLight(int(x&15), int(y), int(z&15))
}

func (nb *lightChunkNeighborhood) setBlockLight(x, y, z int32, val uint8) {
	chunk := nb.chunkAt(int(x>>4), int(z>>4))
	if chunk == nil {
		return
	}
	chunk.SetBlockLight(int(x&15), int(y), int(z&15), val)
}

func (nb *lightChunkNeighborhood) getBlockID(x, y, z int32) int32 {
	chunk := nb.chunkAt(int(x>>4), int(z>>4))
	if chunk == nil {
		return AirID
	}
	secIdx := (int(y) - MinWorldHeight) >> 4
	if uint(secIdx) >= SectionsPerChunk {
		return AirID
	}
	sec := &chunk.sections[secIdx]
	if sec.nonAirBlocks == 0 || sec.blocks == nil {
		return AirID
	}
	idx := (((int(y) - MinWorldHeight) & 15) << 8) | (int(z&15) << 4) | int(x&15)
	return sec.blocks[idx]
}

// getChunkUnlocked retrieves a chunk without acquiring locks.
// Used internally when the caller already holds the appropriate lock.
func (le *LightEngine) getChunkUnlocked(cx, cz int) *Chunk {
	return le.world.chunks[ChunkPos{cx, cz}]
}

// computeHeightmap scans the chunk and records the highest light-attenuating block per column.
// Non-solid transparent blocks (Opacity == 0, such as short_grass, tall_grass, flowers,
// cave_air, torches) do not obstruct direct vertical sky light (15), so the heightmap
// stops at the first block with Opacity > 0.
//
// Optimization: skips empty sections from the top down in O(1) and accesses section
// block arrays directly instead of calling GetBlock 98,304 times.
func (le *LightEngine) computeHeightmap(chunk *Chunk, cx, cz int) {
	topSec := SectionsPerChunk - 1
	for topSec >= 0 && chunk.sections[topSec].IsEmpty() {
		topSec--
	}
	if topSec < 0 {
		for i := range chunk.heightMap {
			chunk.heightMap[i] = MinWorldHeight - 1
		}
		return
	}

	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			highest := int32(MinWorldHeight - 1)
			colBits := (z << 4) | x
		sectionLoop:
			for secIdx := topSec; secIdx >= 0; secIdx-- {
				sec := &chunk.sections[secIdx]
				if sec.IsEmpty() {
					continue
				}
				baseY := MinWorldHeight + secIdx*16
				blocks := sec.blocks
				for ly := 15; ly >= 0; ly-- {
					blockID := blocks[(ly<<8)|colBits]
					if blockID != AirID && GetLightProps(blockID).Opacity > 0 {
						highest = int32(baseY + ly)
						break sectionLoop
					}
				}
			}
			chunk.heightMap[x*16+z] = highest
		}
	}
}

// computeSkyLight recomputes sky light for a chunk from scratch using a Starlight-inspired
// surface/cliff-edge seeded BFS (https://github.com/PaperMC/Starlight/blob/fabric/TECHNICAL_DETAILS.md):
//   - Every block above its column heightmap has direct sky access (15).
//   - Instead of pushing all ~65,000 sky-exposed air blocks into the BFS queue (99% of which
//     are surrounded on all 6 sides by other level-15 sky blocks and cannot propagate anywhere),
//     a sky-exposed cell at (x, y, z) is ONLY seeded into the BFS queue when:
//     (a) y == heightY + 1 (directly above the surface, so it can propagate downward into
//     a translucent block at heightY or horizontally along the surface), OR
//     (b) y <= maxNeighborHeight (at least one of the 4 cardinal neighbor columns has a
//     heightmap >= y, meaning there is an adjacent wall/cliff/overhang at height y
//     that blocks direct sky access in that neighbor column).
//   - Sections strictly above maxHeightY are bulk-filled with 0xFF (level 15), and sections
//     strictly below minHeightY are bulk-cleared to 0x00 before the BFS fills caves/overhangs.
func (le *LightEngine) computeSkyLight(chunk *Chunk, cx, cz int) {
	nb := le.makeNeighborhood(chunk, cx, cz)

	qPtr := lightQueuePool.Get().(*[]lightNode)
	queue := (*qPtr)[:0]
	defer func() {
		*qPtr = queue[:0]
		lightQueuePool.Put(qPtr)
	}()

	// Find min and max heightmap values across the chunk's 256 columns.
	minHeightY := int32(lightTopY)
	maxHeightY := int32(MinWorldHeight - 1)
	for _, h := range chunk.heightMap {
		if h < minHeightY {
			minHeightY = h
		}
		if h > maxHeightY {
			maxHeightY = h
		}
	}

	// Bulk-initialize sections that are 100% above maxHeightY or 100% at/below minHeightY.
	for secIdx := 0; secIdx < SectionsPerChunk; secIdx++ {
		secMinY := int32(MinWorldHeight + secIdx*16)
		secMaxY := secMinY + 15
		sec := &chunk.sections[secIdx]
		if secMaxY <= minHeightY {
			// Completely below the lowest surface column: sky light starts at 0.
			// Keep sec.skyLight nil until BFS actually propagates cave/overhang light into it.
			if sec.skyLight != nil {
				clear(sec.skyLight)
			}
			continue
		}
		if sec.skyLight == nil {
			sec.skyLight = make([]byte, 2048)
		}
		if secMinY > maxHeightY {
			copy(sec.skyLight, skyLightFullSection[:])
		} else {
			// Straddling section: clear first, then fill 15 above heightY per column below.
			clear(sec.skyLight)
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					h := chunk.heightMap[x*16+z]
					startLY := 0
					if h >= secMinY {
						startLY = int(h - secMinY + 1)
					}
					for ly := startLY; ly < 16; ly++ {
						idx := (ly << 8) | (z << 4) | x
						byteIdx := idx >> 1
						if idx&1 == 0 {
							sec.skyLight[byteIdx] |= 0x0F
						} else {
							sec.skyLight[byteIdx] |= 0xF0
						}
					}
				}
			}
		}
	}

	// Helper to read column heightmap in local or adjacent chunk.
	colHeight := func(lx, lz int, fallback int32) int32 {
		if uint(lx) < 16 && uint(lz) < 16 {
			return chunk.heightMap[lx*16+lz]
		}
		ncx, ncz := cx, cz
		if lx < 0 {
			ncx--
			lx += 16
		} else if lx >= 16 {
			ncx++
			lx -= 16
		}
		if lz < 0 {
			ncz--
			lz += 16
		} else if lz >= 16 {
			ncz++
			lz -= 16
		}
		nc := nb.chunkAt(ncx, ncz)
		if nc == nil {
			return fallback
		}
		return nc.heightMap[lx*16+lz]
	}

	// Seed only sky-exposed cells that can actually propagate light (Starlight seeding).
	baseWorldX := int32(cx * 16)
	baseWorldZ := int32(cz * 16)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			heightY := chunk.heightMap[x*16+z]
			maxN := heightY + 1
			if h := colHeight(x-1, z, heightY); h > maxN {
				maxN = h
			}
			if h := colHeight(x+1, z, heightY); h > maxN {
				maxN = h
			}
			if h := colHeight(x, z-1, heightY); h > maxN {
				maxN = h
			}
			if h := colHeight(x, z+1, heightY); h > maxN {
				maxN = h
			}
			if maxN > lightTopY {
				maxN = lightTopY
			}
			wx := baseWorldX + int32(x)
			wz := baseWorldZ + int32(z)
			for y := heightY + 1; y <= maxN; y++ {
				queue = append(queue, lightNode{x: wx, y: y, z: wz, level: 15})
			}
		}
	}

	// Also seed from already-loaded cardinal neighbor border columns so cross-chunk
	// light (e.g. into overhangs/caves at the chunk border) is pulled in immediately.
	borderDirs := [4]struct {
		dx, dz         int
		localX, localZ int
		neighX, neighZ int
		stepX, stepZ   int
	}{
		{-1, 0, 0, 0, 15, 0, 0, 1},
		{1, 0, 15, 0, 0, 0, 0, 1},
		{0, -1, 0, 0, 0, 15, 1, 0},
		{0, 1, 0, 15, 0, 0, 1, 0},
	}
	for _, bd := range borderDirs {
		nc := nb.chunkAt(cx+bd.dx, cz+bd.dz)
		if nc == nil {
			continue
		}
		for i := 0; i < 16; i++ {
			lx := bd.localX + i*bd.stepX
			lz := bd.localZ + i*bd.stepZ
			nx := bd.neighX + i*bd.stepX
			nz := bd.neighZ + i*bd.stepZ
			localH := chunk.heightMap[lx*16+lz]
			if localH < MinWorldHeight {
				continue
			}
			nwx := int32((cx+bd.dx)*16 + nx)
			nwz := int32((cz+bd.dz)*16 + nz)
			topY := localH
			if topY > lightTopY {
				topY = lightTopY
			}
			for y := int32(MinWorldHeight); y <= topY; y++ {
				nl := nc.GetSkyLight(nx, int(y), nz)
				if nl > 1 {
					queue = append(queue, lightNode{x: nwx, y: y, z: nwz, level: nl})
				}
			}
		}
	}

	// BFS: monotonically raise neighbour light until nothing can be brightened.
	for head := 0; head < len(queue); head++ {
		node := queue[head]
		cur := node.level
		if cur <= 1 {
			continue
		}
		for i, off := range lightOffsets {
			nx, ny, nz := node.x+off[0], node.y+off[1], node.z+off[2]
			if ny < MinWorldHeight || ny > lightTopY {
				continue
			}
			if int(nx>>4) != cx || int(nz>>4) != cz {
				continue
			}
			existing := nb.getSkyLight(nx, ny, nz)
			// Starlight neighbor short-circuit: skip block/opacity lookup if neighbor
			// already has at least the maximum possible light this step could produce.
			if i == 5 && cur == 15 {
				if existing == 15 {
					continue
				}
			} else if existing >= cur-1 {
				continue
			}

			opacity := GetLightProps(nb.getBlockID(nx, ny, nz)).Opacity
			var newLight uint8
			if i == 5 && cur == 15 && opacity == 0 {
				newLight = 15 // straight down through a transparent block: no loss
			} else if loss := stepLoss(opacity); cur > loss {
				newLight = cur - loss
			}

			if newLight > existing {
				nb.setSkyLight(nx, ny, nz, newLight)
				if newLight > 1 {
					queue = append(queue, lightNode{x: nx, y: ny, z: nz, level: newLight})
				}
			}
		}
	}
}

// computeBlockLight recomputes block light for a chunk from scratch using BFS
// from emitters, losing max(1, opacity) per step. Empty sections (IsEmpty())
// cannot contain light-emitting blocks and are bulk-cleared in O(1).
func (le *LightEngine) computeBlockLight(chunk *Chunk, cx, cz int) {
	nb := le.makeNeighborhood(chunk, cx, cz)

	qPtr := lightQueuePool.Get().(*[]lightNode)
	queue := (*qPtr)[:0]
	defer func() {
		*qPtr = queue[:0]
		lightQueuePool.Put(qPtr)
	}()

	baseWorldX := int32(cx * 16)
	baseWorldZ := int32(cz * 16)

	for secIdx := 0; secIdx < SectionsPerChunk; secIdx++ {
		sec := &chunk.sections[secIdx]
		if sec.blockLight != nil {
			clear(sec.blockLight)
		}
		if sec.IsEmpty() {
			continue
		}
		baseY := int32(MinWorldHeight + secIdx*16)
		blocks := sec.blocks
		for ly := 0; ly < 16; ly++ {
			y := baseY + int32(ly)
			yBits := ly << 8
			for z := 0; z < 16; z++ {
				yzBits := yBits | (z << 4)
				wz := baseWorldZ + int32(z)
				for x := 0; x < 16; x++ {
					id := blocks[yzBits|x]
					if id == AirID {
						continue
					}
					emission := GetLightProps(id).Emission
					if emission > 0 {
						sec.SetBlockLight(x, ly, z, emission)
						queue = append(queue, lightNode{
							x:     baseWorldX + int32(x),
							y:     y,
							z:     wz,
							level: emission,
						})
					}
				}
			}
		}
	}

	// Pull block light from already-loaded cardinal neighbor border columns first.
	borderDirs := [4]struct {
		dx, dz         int
		localX, localZ int
		neighX, neighZ int
		stepX, stepZ   int
	}{
		{-1, 0, 0, 0, 15, 0, 0, 1},
		{1, 0, 15, 0, 0, 0, 0, 1},
		{0, -1, 0, 0, 0, 15, 1, 0},
		{0, 1, 0, 15, 0, 0, 1, 0},
	}
	for _, bd := range borderDirs {
		nc := nb.chunkAt(cx+bd.dx, cz+bd.dz)
		if nc == nil {
			continue
		}
		nwxBase := int32((cx + bd.dx) * 16)
		nwzBase := int32((cz + bd.dz) * 16)
		for secIdx := 0; secIdx < SectionsPerChunk; secIdx++ {
			nsec := &nc.sections[secIdx]
			if nsec.blockLight == nil {
				continue
			}
			// Quick check if neighbor section has any block light at all.
			hasAny := false
			for j := 0; j+8 <= len(nsec.blockLight); j += 8 {
				if binary.LittleEndian.Uint64(nsec.blockLight[j:j+8]) != 0 {
					hasAny = true
					break
				}
			}
			if !hasAny {
				continue
			}
			secBaseY := int32(MinWorldHeight + secIdx*16)
			for i := 0; i < 16; i++ {
				nx := bd.neighX + i*bd.stepX
				nz := bd.neighZ + i*bd.stepZ
				nwx := nwxBase + int32(nx)
				nwz := nwzBase + int32(nz)
				for ly := 0; ly < 16; ly++ {
					nl := nsec.GetBlockLight(nx, ly, nz)
					if nl > 1 {
						queue = append(queue, lightNode{
							x:     nwx,
							y:     secBaseY + int32(ly),
							z:     nwz,
							level: nl,
						})
					}
				}
			}
		}
	}

	// BFS: monotonically raise neighbour light until nothing can be brightened.
	for head := 0; head < len(queue); head++ {
		node := queue[head]
		cur := node.level
		if cur <= 1 {
			continue
		}
		maxPossible := cur - 1
		for _, off := range lightOffsets {
			nx, ny, nz := node.x+off[0], node.y+off[1], node.z+off[2]
			if ny < MinWorldHeight || ny > lightTopY {
				continue
			}
			if int(nx>>4) != cx || int(nz>>4) != cz {
				continue
			}
			existing := nb.getBlockLight(nx, ny, nz)
			if existing >= maxPossible {
				continue
			}
			opacity := GetLightProps(nb.getBlockID(nx, ny, nz)).Opacity
			if loss := stepLoss(opacity); cur > loss {
				newLight := cur - loss
				if newLight > existing {
					nb.setBlockLight(nx, ny, nz, newLight)
					if newLight > 1 {
						queue = append(queue, lightNode{x: nx, y: ny, z: nz, level: newLight})
					}
				}
			}
		}
	}
}
