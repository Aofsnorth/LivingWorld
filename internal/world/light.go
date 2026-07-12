package world

import "sync"

// LightEngine manages sky and block light propagation across the world.
// Light is computed per-chunk using BFS from light sources (sky access for
// sky light, light-emitting blocks for block light). When blocks change,
// affected chunks are queued for recomputation and processed during the tick.
type LightEngine struct {
	mu      sync.Mutex
	world   *World
	pending map[ChunkPos]struct{} // chunks needing light recomputation
}

// NewLightEngine creates a light engine for the given world.
func NewLightEngine(w *World) *LightEngine {
	return &LightEngine{
		world:   w,
		pending: make(map[ChunkPos]struct{}),
	}
}

// QueueUpdate marks a chunk for light recomputation on the next tick.
func (le *LightEngine) QueueUpdate(cx, cz int) {
	le.mu.Lock()
	le.pending[ChunkPos{cx, cz}] = struct{}{}
	le.mu.Unlock()
}

// ProcessUpdates recomputes light for all queued chunks and clears the queue.
// Called from the tick loop. It returns the chunks whose light actually changed,
// so the caller can re-send light to players (a no-op relight — common when a
// neighbour load doesn't affect already-correct open terrain — reports nothing
// and triggers no network traffic).
func (le *LightEngine) ProcessUpdates() []ChunkPos {
	le.mu.Lock()
	pending := le.pending
	le.pending = make(map[ChunkPos]struct{})
	le.mu.Unlock()

	if len(pending) == 0 {
		return nil
	}

	// Keep the world read lock for the whole batch. ComputeChunkLight's
	// world-coordinate helpers intentionally use getChunkUnlocked because chunk
	// generation calls this while already holding world.mu; without a lock here,
	// tick-time light recomputation can race a concurrent chunk load/unload and
	// panic on a concurrent map read/write. Fingerprints are snapshotted for the
	// whole batch BEFORE any recompute so a cross-chunk write from one relight is
	// still detected as a change on the chunk it touched.
	type relit struct {
		pos    ChunkPos
		chunk  *Chunk
		before uint64
	}
	le.world.mu.RLock()
	defer le.world.mu.RUnlock()

	items := make([]relit, 0, len(pending))
	for pos := range pending {
		if c := le.world.chunks[pos]; c != nil {
			items = append(items, relit{pos: pos, chunk: c, before: lightFingerprint(c)})
		}
	}
	for _, it := range items {
		le.ComputeChunkLight(it.chunk, it.pos.X, it.pos.Z)
	}
	var changed []ChunkPos
	for _, it := range items {
		if lightFingerprint(it.chunk) != it.before {
			changed = append(changed, it.pos)
		}
	}
	return changed
}

// ComputeChunkLight computes sky and block light for a single chunk.
// This is called when a chunk is first loaded or when blocks change.
func (le *LightEngine) ComputeChunkLight(chunk *Chunk, cx, cz int) {
	// Build heightmap
	le.computeHeightmap(chunk, cx, cz)

	// Compute sky light
	le.computeSkyLight(chunk, cx, cz)

	// Compute block light
	le.computeBlockLight(chunk, cx, cz)
}
