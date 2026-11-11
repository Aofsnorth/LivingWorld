package world

import (
	"log"
	"path/filepath"
	"sync"
)

type ChunkPos struct {
	X, Z int
}

type Dimension string

const (
	DimensionOverworld Dimension = "overworld"
	DimensionNether    Dimension = "nether"
	DimensionEnd       Dimension = "end"
)

type ChunkGenerator interface {
	Generate(cx, cz int) *Chunk
}

type chunkLoadCall struct {
	done  chan struct{}
	chunk *Chunk
}

type World struct {
	name      string
	chunks    map[ChunkPos]*Chunk
	inflight  map[ChunkPos]*chunkLoadCall
	players   map[uint64]*Player
	generator ChunkGenerator
	storage   Storage
	mu        sync.RWMutex
	dimension Dimension
	time      int64
	dayTime   int64

	raining    bool
	thundering bool
	seed       int64
	worldDir   string // set when a disk RegionStorage is attached; holds level.json

	light *LightEngine // Phase 4b: sky + block light propagation
}

func NewWorld(name string) *World {
	w := &World{
		name:      name,
		chunks:    make(map[ChunkPos]*Chunk),
		inflight:  make(map[ChunkPos]*chunkLoadCall),
		players:   make(map[uint64]*Player),
		dimension: DimensionOverworld,
		storage:   NopStorage{},
	}
	w.light = NewLightEngine(w)
	return w
}

// HasRegionStorageDir reports whether w is already backed by a RegionStorage
// rooted at regionDir, avoiding redundant directory stat and level.json reloads.
func (w *World) HasRegionStorageDir(regionDir string) bool {
	w.mu.RLock()
	rs, ok := w.storage.(*RegionStorage)
	w.mu.RUnlock()
	return ok && rs != nil && rs.dir == regionDir
}

// SetStorage attaches a persistence backend to the world. Pass NopStorage{} to
// disable persistence.
func (w *World) SetStorage(s Storage) {
	w.mu.Lock()
	if s == nil {
		s = NopStorage{}
	}
	prevDir := w.worldDir
	_, wasNop := w.storage.(NopStorage)
	w.storage = s
	if rs, ok := s.(*RegionStorage); ok {
		newDir := filepath.Dir(rs.dir)
		w.worldDir = newDir
		// Clear any chunks generated in memory before persistence was attached
		// (or when switching to a different world directory) so subsequent
		// LoadChunk calls consult the attached RegionStorage.
		if wasNop || prevDir != newDir {
			w.chunks = make(map[ChunkPos]*Chunk)
		}
	}
	w.mu.Unlock()
	w.loadLevel()
}

// EvictChunk removes a chunk from in-memory world map (used to discard
// rejected ocean chunks probed during initial spawn search).
func (w *World) EvictChunk(cx, cz int) {
	w.mu.Lock()
	delete(w.chunks, ChunkPos{cx, cz})
	w.mu.Unlock()
}

// Save persists every chunk with unsaved changes. Safe to call concurrently.
func (w *World) Save() error {
	w.saveLevel() // persist weather/time alongside chunks
	w.mu.RLock()
	type entry struct {
		pos ChunkPos
		c   *Chunk
	}
	var dirty []entry
	storage := w.storage
	for pos, c := range w.chunks {
		if c.Dirty() {
			dirty = append(dirty, entry{pos, c})
		}
	}
	w.mu.RUnlock()

	if storage == nil || len(dirty) == 0 {
		return nil
	}
	var firstErr error
	for _, e := range dirty {
		if err := storage.SaveChunk(e.pos.X, e.pos.Z, e.c); err != nil {
			log.Printf("[World %s] save chunk (%d,%d) failed: %v", w.name, e.pos.X, e.pos.Z, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	// Commit buffered writes (region backends write whole files here).
	if err := storage.Flush(); err != nil {
		log.Printf("[World %s] flush failed: %v", w.name, err)
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		for _, e := range dirty {
			e.c.ClearDirty()
		}
		log.Printf("[World %s] saved %d chunk(s)", w.name, len(dirty))
	}
	return firstErr
}

func (w *World) Name() string                  { return w.name }
func (w *World) SetGenerator(g ChunkGenerator) { w.generator = g }

// Dimension returns this world's dimension (overworld | nether | end).
// M2: used by the natural-spawning director to filter mob rules by
// dimension (e.g. piglin only spawns in the nether, ghast only in
// the nether, wither skeleton only in the nether).
func (w *World) Dimension() Dimension {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.dimension
}

func (w *World) GetChunk(cx, cz int) *Chunk {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.chunks[ChunkPos{cx, cz}]
}

func (w *World) SetChunk(cx, cz int, c *Chunk) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.chunks[ChunkPos{cx, cz}] = c
}

func (w *World) LoadChunk(cx, cz int) *Chunk {
	pos := ChunkPos{cx, cz}
	w.mu.RLock()
	chunk := w.chunks[pos]
	w.mu.RUnlock()
	if chunk != nil {
		return chunk
	}

	w.mu.Lock()
	if chunk = w.chunks[pos]; chunk != nil {
		w.mu.Unlock()
		return chunk
	}
	if call, ok := w.inflight[pos]; ok {
		w.mu.Unlock()
		<-call.done
		return call.chunk
	}
	call := &chunkLoadCall{done: make(chan struct{})}
	if w.inflight == nil {
		w.inflight = make(map[ChunkPos]*chunkLoadCall)
	}
	w.inflight[pos] = call
	storage := w.storage
	gen := w.generator
	w.mu.Unlock()

	// Run disk I/O and CPU-intensive terrain generation OUTSIDE w.mu.Lock()
	// (Moonrise/C2ME lock-free concurrent chunk pipeline principle) so worker
	// goroutines scale across all CPU cores without blocking world readers/tick loop.
	var loadedFromDisk bool
	if storage != nil {
		if c, ok, err := storage.LoadChunk(cx, cz); err != nil {
			log.Printf("[World %s] load chunk (%d,%d) failed, regenerating: %v", w.name, cx, cz, err)
		} else if ok {
			chunk = c
			loadedFromDisk = true
		}
	}

	if !loadedFromDisk {
		if gen != nil {
			chunk = gen.Generate(cx, cz)
			if storage != nil {
				chunk.MarkDirty()
			}
		} else {
			chunk = NewChunk()
		}
	}

	// Pre-compute the chunk's own heightmap outside the world lock (touches only chunk).
	if w.light != nil {
		w.light.computeHeightmap(chunk, cx, cz)
	}

	w.mu.Lock()
	// If SetChunk or SetStorage intervened while unlocked, respect any already-stored chunk.
	if existing := w.chunks[pos]; existing != nil {
		chunk = existing
	} else {
		w.chunks[pos] = chunk
		if w.light != nil {
			w.light.computeSkyLight(chunk, cx, cz)
			w.light.computeBlockLight(chunk, cx, cz)
			w.queueNeighborRelight(cx, cz)
		}
	}
	call.chunk = chunk
	delete(w.inflight, pos)
	close(call.done)
	w.mu.Unlock()

	return chunk
}

// LoadChunksBatch loads or generates multiple chunks concurrently across up to
// workers goroutines without holding w.mu during disk I/O or terrain generation,
// then computes their lighting once the entire batch is registered in w.chunks.
// Because all chunks in the batch are already present in w.chunks when lighting
// runs, cross-chunk heightmaps and border light settle on the first pass.
func (w *World) LoadChunksBatch(coords []ChunkPos, workers int) []*Chunk {
	n := len(coords)
	if n == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1
	}

	results := make([]*Chunk, n)
	newlyInserted := make([]bool, n)

	// Fast path: check which chunks are already loaded under RLock.
	missingCount := 0
	w.mu.RLock()
	for i, pos := range coords {
		if c := w.chunks[pos]; c != nil {
			results[i] = c
		} else {
			missingCount++
		}
	}
	w.mu.RUnlock()

	if missingCount == 0 {
		return results
	}

	type pendingItem struct {
		idx  int
		pos  ChunkPos
		call *chunkLoadCall
		own  bool
	}
	pending := make([]pendingItem, 0, missingCount)

	w.mu.Lock()
	if w.inflight == nil {
		w.inflight = make(map[ChunkPos]*chunkLoadCall)
	}
	storage := w.storage
	gen := w.generator
	for i, pos := range coords {
		if results[i] != nil {
			continue
		}
		if c := w.chunks[pos]; c != nil {
			results[i] = c
			continue
		}
		if call, ok := w.inflight[pos]; ok {
			pending = append(pending, pendingItem{idx: i, pos: pos, call: call, own: false})
			continue
		}
		call := &chunkLoadCall{done: make(chan struct{})}
		w.inflight[pos] = call
		pending = append(pending, pendingItem{idx: i, pos: pos, call: call, own: true})
	}
	w.mu.Unlock()

	// Phase 1: Generate / disk-load all owned missing chunks in parallel outside w.mu.
	// Cap generator concurrency at 4 so SteelMC's internal Rayon pool is not over-saturated.
	genWorkers := min(workers, 4)
	generated := make([]*Chunk, len(pending))
	var wg sync.WaitGroup
	sem := make(chan struct{}, genWorkers)
	for pIdx, item := range pending {
		if !item.own {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(pIdx int, pos ChunkPos) {
			defer wg.Done()
			defer func() { <-sem }()

			var chunk *Chunk
			var loadedFromDisk bool
			if storage != nil {
				if c, ok, err := storage.LoadChunk(pos.X, pos.Z); err != nil {
					log.Printf("[World %s] load chunk (%d,%d) failed, regenerating: %v", w.name, pos.X, pos.Z, err)
				} else if ok {
					chunk = c
					loadedFromDisk = true
				}
			}
			if !loadedFromDisk {
				if gen != nil {
					chunk = gen.Generate(pos.X, pos.Z)
					if storage != nil {
						chunk.MarkDirty()
					}
				} else {
					chunk = NewChunk()
				}
			}
			if w.light != nil {
				w.light.computeHeightmap(chunk, pos.X, pos.Z)
			}
			generated[pIdx] = chunk
		}(pIdx, item.pos)
	}
	wg.Wait()

	// Phase 2: Register all newly generated chunks into w.chunks, then run light
	// propagation with all batch neighbors already present in w.chunks.
	batchSet := make(map[ChunkPos]struct{}, len(pending))
	w.mu.Lock()
	for pIdx, item := range pending {
		if !item.own {
			continue
		}
		c := generated[pIdx]
		if existing := w.chunks[item.pos]; existing != nil {
			c = existing
		} else {
			w.chunks[item.pos] = c
			newlyInserted[item.idx] = true
			batchSet[item.pos] = struct{}{}
		}
		results[item.idx] = c
	}

	if w.light != nil {
		for _, item := range pending {
			if !item.own || !newlyInserted[item.idx] {
				continue
			}
			c := results[item.idx]
			w.light.computeSkyLight(c, item.pos.X, item.pos.Z)
			w.light.computeBlockLight(c, item.pos.X, item.pos.Z)
		}
		// Queue relights only for already-loaded neighbors outside this batch.
		for _, item := range pending {
			if !item.own || !newlyInserted[item.idx] {
				continue
			}
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				npos := ChunkPos{item.pos.X + d[0], item.pos.Z + d[1]}
				if _, inBatch := batchSet[npos]; !inBatch && w.chunks[npos] != nil {
					w.light.QueueUpdate(npos.X, npos.Z)
				}
			}
		}
	}

	for _, item := range pending {
		if !item.own {
			continue
		}
		item.call.chunk = results[item.idx]
		delete(w.inflight, item.pos)
		close(item.call.done)
	}
	w.mu.Unlock()

	// Wait for any chunks that were already in-flight from another goroutine.
	for _, item := range pending {
		if item.own {
			continue
		}
		<-item.call.done
		results[item.idx] = item.call.chunk
	}

	return results
}

// queueNeighborRelight queues the four already-loaded cardinal neighbours of a
// just-loaded chunk for light recomputation on the next tick. A chunk that was
// lit before this neighbour existed treated the missing neighbour as opaque/dark
// at the shared edge, leaving an order-dependent dark seam; recomputing the
// neighbours (which now read this chunk's real light via the world-coord helpers)
// settles the seam symmetrically.
//
// Precondition: the caller holds w.mu (so reading w.chunks is safe). QueueUpdate
// takes its own lock and never re-enters w.mu, and ComputeChunkLight is NOT run
// synchronously here — the work is deferred to the tick's ProcessUpdates.
func (w *World) queueNeighborRelight(cx, cz int) {
	for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		if w.chunks[ChunkPos{cx + d[0], cz + d[1]}] != nil {
			w.light.QueueUpdate(cx+d[0], cz+d[1])
		}
	}
}

func (w *World) UnloadChunk(cx, cz int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.chunks, ChunkPos{cx, cz})
}

func (w *World) SpawnPlayer(p *Player) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.players[p.UUID] = p
	p.World = w
}

func (w *World) RemovePlayer(id uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.players, id)
}

func (w *World) GetPlayers() []*Player {
	w.mu.RLock()
	defer w.mu.RUnlock()
	players := make([]*Player, 0, len(w.players))
	for _, p := range w.players {
		players = append(players, p)
	}
	return players
}

// Light returns the world's light engine (Phase 4b).
func (w *World) Light() *LightEngine { return w.light }
