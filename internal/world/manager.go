package world

import (
	"math/rand"
	"path/filepath"
	"sync"

	"livingworld/internal/drops"
	"livingworld/internal/loot"
	"livingworld/internal/mobs"
)

type Manager struct {
	mu           sync.RWMutex
	worlds       map[string]*World
	defaultWorld *World
	blockEvents  *BlockEventBus
	drops        *drops.Store
	mobs         *mobs.Store
	dropRNG      *rand.Rand
	dropMu       sync.Mutex // guards dropRNG (math/rand is not concurrency-safe)
	autosaveStop chan struct{}
	timeStop     chan struct{}

	// pickupCallback is called when a player picks up an item, for Bedrock inventory sync.
	pickupCallback func(playerUUID [16]byte, dropEntityID int64, playerEntityID uint64)
	pickupMu       sync.RWMutex

	// crackManager tracks active block-breaking states for cross-edition crack animation.
	crackManager *CrackManager

	weatherMu        sync.RWMutex
	weatherCallbacks []func(raining, thundering bool)
}

func NewManager() *Manager {
	m := &Manager{
		worlds:       make(map[string]*World),
		blockEvents:  NewBlockEventBus(),
		drops:        drops.New(),
		mobs:         mobs.New(),
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

// Mobs returns the shared mob store. Each protocol bridge subscribes
// (OnSpawn/OnDespawn) to render and remove mobs cross-edition.
func (m *Manager) Mobs() *mobs.Store { return m.mobs }

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
