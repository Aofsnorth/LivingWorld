package world

import (
	"log"
	"path/filepath"
	"sync"
	"sync/atomic"
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

type World struct {
	name      string
	chunks    map[ChunkPos]*Chunk
	players   map[uint64]*Player
	generator ChunkGenerator
	storage   Storage
	mu        sync.RWMutex
	dimension Dimension
	time      int64
	dayTime   int64

	raining    bool
	thundering bool
	worldDir   string // set when a disk RegionStorage is attached; holds level.json
}

func NewWorld(name string) *World {
	return &World{
		name:      name,
		chunks:    make(map[ChunkPos]*Chunk),
		players:   make(map[uint64]*Player),
		dimension: DimensionOverworld,
		storage:   NopStorage{},
	}
}

// SetStorage attaches a persistence backend to the world. Pass NopStorage{} to
// disable persistence.
func (w *World) SetStorage(s Storage) {
	w.mu.Lock()
	if s == nil {
		s = NopStorage{}
	}
	w.storage = s
	if rs, ok := s.(*RegionStorage); ok {
		w.worldDir = filepath.Dir(rs.dir)
	}
	w.mu.Unlock()
	w.loadLevel()
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
	defer w.mu.Unlock()
	// Re-check under the write lock: another goroutine may have loaded it.
	if chunk = w.chunks[pos]; chunk != nil {
		return chunk
	}

	// Prefer the saved chunk (player edits) over regeneration.
	if w.storage != nil {
		if c, ok, err := w.storage.LoadChunk(cx, cz); err != nil {
			log.Printf("[World %s] load chunk (%d,%d) failed, regenerating: %v", w.name, cx, cz, err)
		} else if ok {
			w.chunks[pos] = c
			return c
		}
	}

	if w.generator != nil {
		chunk = w.generator.Generate(cx, cz)
	} else {
		chunk = NewChunk()
	}
	w.chunks[pos] = chunk
	return chunk
}

func (w *World) UnloadChunk(cx, cz int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.chunks, ChunkPos{cx, cz})
}

func (w *World) SetBlock(x, y, z int, block Block) {
	chunkX, chunkZ := x>>4, z>>4
	chunk := w.LoadChunk(chunkX, chunkZ)
	chunk.SetBlock(x&15, y, z&15, block)
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

func (w *World) SetTime(t int64)    { atomic.StoreInt64(&w.time, t) }
func (w *World) GetTime() int64     { return atomic.LoadInt64(&w.time) }
func (w *World) SetDayTime(t int64) { atomic.StoreInt64(&w.dayTime, t) }
func (w *World) GetDayTime() int64  { return atomic.LoadInt64(&w.dayTime) }
