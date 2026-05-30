package world

import (
	"log"
	"math/rand"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"livingworld/internal/drops"
	"livingworld/internal/loot"
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
	defer w.mu.Unlock()
	if s == nil {
		s = NopStorage{}
	}
	w.storage = s
}

// Save persists every chunk with unsaved changes. Safe to call concurrently.
func (w *World) Save() error {
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

type Manager struct {
	mu           sync.RWMutex
	worlds       map[string]*World
	defaultWorld *World
	blockEvents  *BlockEventBus
	drops        *drops.Store
	dropRNG      *rand.Rand
	dropMu       sync.Mutex // guards dropRNG (math/rand is not concurrency-safe)
	autosaveStop chan struct{}
	timeStop     chan struct{}

	// pickupCallback is called when a player picks up an item, for Bedrock inventory sync.
	pickupCallback func(playerUUID [16]byte, dropEntityID int64, playerEntityID uint64)
	pickupMu       sync.RWMutex

	// crackManager tracks active block-breaking states for cross-edition crack animation.
	crackManager *CrackManager
}

func NewManager() *Manager {
	m := &Manager{
		worlds:       make(map[string]*World),
		blockEvents:  NewBlockEventBus(),
		drops:        drops.New(),
		dropRNG:      rand.New(rand.NewSource(1)), // deterministic; drops aren't security-sensitive
		crackManager: NewCrackManager(),
	}
	m.defaultWorld = NewWorld("world")
	m.worlds["world"] = m.defaultWorld
	return m
}

// Drops returns the shared item-drop store. Each protocol bridge subscribes to
// it (OnSpawn/OnDespawn) to render and pick up dropped items.
func (m *Manager) Drops() *drops.Store { return m.drops }

// OnItemPickup registers a callback invoked when a player picks up an item.
// Used by the Bedrock server to send pickup animation + inventory sync.
func (m *Manager) OnItemPickup(fn func(playerUUID [16]byte, dropEntityID int64, playerEntityID uint64)) {
	m.pickupMu.Lock()
	m.pickupCallback = fn
	m.pickupMu.Unlock()
}

// NotifyItemPickup calls the registered pickup callback if one exists.
func (m *Manager) NotifyItemPickup(playerUUID [16]byte, dropEntityID int64, playerEntityID uint64) {
	m.pickupMu.RLock()
	cb := m.pickupCallback
	m.pickupMu.RUnlock()
	if cb != nil {
		cb(playerUUID, dropEntityID, playerEntityID)
	}
}

// DropBlockLoot rolls the vanilla bare-hand loot for the block that was at
// (x,y,z) and spawns the resulting item entities into the drop store, centred on
// the block. blockID is the canonical world block id that was broken. Call this
// BEFORE replacing the block with air.
func (m *Manager) DropBlockLoot(blockID int32, x, y, z int) {
	name := StateName(blockID)
	m.dropMu.Lock()
	stacks := loot.Rolls(name, m.dropRNG)
	m.dropMu.Unlock()
	for _, st := range stacks {
		m.drops.Spawn(st.Item, st.Count, float64(x)+0.5, float64(y)+0.25, float64(z)+0.5)
	}
}

func (m *Manager) GetWorld(name string) *World {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.worlds[name]
}

func (m *Manager) GetDefaultWorld() *World {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultWorld
}

func (m *Manager) AddWorld(name string, w *World) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.worlds[name] = w
}

func (m *Manager) RemoveWorld(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.worlds, name)
}

// CrackManager returns the shared crack state tracker for cross-edition animation.
func (m *Manager) CrackManager() *CrackManager {
	return m.crackManager
}

func (m *Manager) GetAllWorlds() []*World {
	m.mu.RLock()
	defer m.mu.RUnlock()
	worlds := make([]*World, 0, len(m.worlds))
	for _, w := range m.worlds {
		worlds = append(worlds, w)
	}
	return worlds
}

func (m *Manager) SubscribeBlockUpdates(id string, buffer int) <-chan BlockUpdateEvent {
	return m.blockEvents.Subscribe(id, buffer)
}

func (m *Manager) UnsubscribeBlockUpdates(id string) {
	m.blockEvents.Unsubscribe(id)
}

func (m *Manager) PublishBlockUpdate(source BlockUpdateSource, x, y, z int, blockID int32) {
	m.blockEvents.Publish(BlockUpdateEvent{Source: source, X: x, Y: y, Z: z, BlockID: blockID})
}

func (m *Manager) SetBlockAndPublish(source BlockUpdateSource, x, y, z int, block Block) {
	m.GetDefaultWorld().SetBlock(x, y, z, block)
	m.PublishBlockUpdate(source, x, y, z, block.ID())
}

// EnablePersistence attaches a DiskStorage to every world, rooted at
// <baseDir>/<worldName>. Returns the first error encountered creating a backend.
func (m *Manager) EnablePersistence(baseDir string) error {
	for _, w := range m.GetAllWorlds() {
		store, err := NewRegionStorage(filepath.Join(baseDir, w.Name()))
		if err != nil {
			return err
		}
		w.SetStorage(store)
	}
	return nil
}

// Save persists all dirty chunks across every world.
func (m *Manager) Save() error {
	var firstErr error
	for _, w := range m.GetAllWorlds() {
		if err := w.Save(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// StartAutosave periodically saves all worlds until Close is called. A non-positive
// interval disables autosave.
func (m *Manager) StartAutosave(interval time.Duration) {
	if interval <= 0 {
		return
	}
	m.mu.Lock()
	if m.autosaveStop != nil {
		m.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	m.autosaveStop = stop
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_ = m.Save()
			}
		}
	}()
}

// StartTimeLoop advances every world's clock at 20 ticks/sec. worldAge is
// monotonic; dayTime wraps at 24000. When advance is false the clock is frozen
// (e.g. a fixed-time world) but worldAge still increments. Idempotent.
func (m *Manager) StartTimeLoop(advance bool) {
	m.mu.Lock()
	if m.timeStop != nil {
		m.mu.Unlock()
		log.Printf("[World] StartTimeLoop: already running, skipping")
		return
	}
	stop := make(chan struct{})
	m.timeStop = stop
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(50 * time.Millisecond) // 20 TPS
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				for _, w := range m.GetAllWorlds() {
					w.SetTime(w.GetTime() + 1)
					if advance {
						w.SetDayTime((w.GetDayTime() + 1) % 24000)
					}
				}
			}
		}
	}()
}

// Close stops autosave and the time loop, then performs a final save of all worlds.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.autosaveStop != nil {
		close(m.autosaveStop)
		m.autosaveStop = nil
	}
	if m.timeStop != nil {
		close(m.timeStop)
		m.timeStop = nil
	}
	m.mu.Unlock()
	return m.Save()
}
