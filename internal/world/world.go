package world

import (
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
	}
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
	w.mu.RLock()
	chunk := w.chunks[ChunkPos{cx, cz}]
	w.mu.RUnlock()
	if chunk != nil {
		return chunk
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.generator != nil {
		chunk = w.generator.Generate(cx, cz)
	} else {
		chunk = NewChunk()
	}
	w.chunks[ChunkPos{cx, cz}] = chunk
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
	chunkX, chunkZ := x>>4, z>>4
	chunk := w.GetChunk(chunkX, chunkZ)
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
}

func NewManager() *Manager {
	m := &Manager{
		worlds:      make(map[string]*World),
		blockEvents: NewBlockEventBus(),
	}
	m.defaultWorld = NewWorld("world")
	m.worlds["world"] = m.defaultWorld
	return m
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
