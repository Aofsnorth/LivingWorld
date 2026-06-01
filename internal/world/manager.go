package world

import (
	"math/rand"
	"path/filepath"
	"sync"
	"time"

	"livingworld/internal/drops"
	"livingworld/internal/loot"
	"livingworld/internal/mobs"

	"github.com/google/uuid"
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
	weatherStop  chan struct{}

	// tickStop / tickAutosaver / tickAdvanceTime / tickRNG are the
	// Phase-4a unified-tick fields. The legacy StartTimeLoop /
	// StartAutosave fields above are kept for the deprecated wrapper
	// paths; new callers should use startTickLoop.
	tickStop        chan struct{}
	tickAutosaver   time.Duration
	tickAdvanceTime bool
	tickRNG         *rand.Rand

	// pickupCallback is called when a player picks up an item, for Bedrock inventory sync.
	pickupCallback func(playerUUID [16]byte, dropEntityID int64, playerEntityID uint64)
	pickupMu       sync.RWMutex

	// crackManager tracks active block-breaking states for cross-edition crack animation.
	crackManager *CrackManager

	// effectEvents carries cross-edition action effects (crack overlay, break
	// particles+sound) — see effects.go.
	effectEvents *WorldEffectBus

	weatherMu        sync.RWMutex
	weatherCallbacks []func(raining, thundering bool)

	// difficulty gates the mob-spawn director (peaceful suppresses hostiles). Set
	// by StartMobAI; read under m.mu by the director tick.
	difficulty string

	// playerLocator returns the live world positions of connected players. The
	// world package can't import player (cycle), so the server bootstrap wires the
	// player.Manager in via SetPlayerLocator. Used by the mob-spawn director as the
	// anchor set for candidate columns.
	locatorMu     sync.RWMutex
	playerLocator func() []Position
}

func NewManager() *Manager {
	m := &Manager{
		worlds:       make(map[string]*World),
		blockEvents:  NewBlockEventBus(),
		drops:        drops.New(),
		mobs:         mobs.New(),
		dropRNG:      rand.New(rand.NewSource(1)), // deterministic; drops aren't security-sensitive
		crackManager: NewCrackManager(),
		effectEvents: NewWorldEffectBus(),
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

// SetPlayerLocator registers a function returning the live world positions of
// connected players (wired from the player.Manager by the server bootstrap).
func (m *Manager) SetPlayerLocator(fn func() []Position) {
	m.locatorMu.Lock()
	m.playerLocator = fn
	m.locatorMu.Unlock()
}

// playerAnchors returns the current player positions, or nil if no locator is set.
func (m *Manager) playerAnchors() []Position {
	m.locatorMu.RLock()
	fn := m.playerLocator
	m.locatorMu.RUnlock()
	if fn == nil {
		return nil
	}
	return fn()
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

func (m *Manager) SubscribeWorldEffects(id string, buffer int) <-chan WorldEffectEvent {
	return m.effectEvents.Subscribe(id, buffer)
}

func (m *Manager) UnsubscribeWorldEffects(id string) {
	m.effectEvents.Unsubscribe(id)
}

func (m *Manager) PublishWorldEffect(ev WorldEffectEvent) {
	m.effectEvents.Publish(ev)
}

// PublishCrack publishes a crack-overlay update (stage>=0) or clear (stage<0) for
// a block being broken by breaker, originating from source.
func (m *Manager) PublishCrack(source BlockUpdateSource, breaker uuid.UUID, x, y, z int, stage int32) {
	m.effectEvents.Publish(WorldEffectEvent{
		Kind: EffectCrackProgress, Source: source, X: x, Y: y, Z: z, Stage: stage, Breaker: breaker,
	})
}

// PublishBlockDestroy publishes a finished-break effect (particles+sound) for the
// block of canonical id blockID at (x,y,z), originating from source.
func (m *Manager) PublishBlockDestroy(source BlockUpdateSource, breaker uuid.UUID, x, y, z int, blockID int32) {
	m.effectEvents.Publish(WorldEffectEvent{
		Kind: EffectBlockDestroy, Source: source, X: x, Y: y, Z: z, BlockID: blockID, Breaker: breaker,
	})
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
