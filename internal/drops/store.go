// Package drops tracks dropped item entities that exist in the world after a
// block is broken. The store is edition-neutral: each protocol bridge (Java,
// Bedrock) subscribes, spawns its own item-entity packets for the drops, and
// reports pickups back here. Item identity is the namespaced item name
// ("minecraft:cobblestone"); each edition resolves that to its own network id.
package drops

import (
	"sync"
	"sync/atomic"
)

// Drop is a single item entity lying in the world.
type Drop struct {
	// EntityID is a globally-unique id for this drop, distinct from player entity
	// ids. Each edition maps it to its own runtime/entity id space.
	EntityID int64
	Item     string // namespaced item name, e.g. "minecraft:cobblestone"
	Count    int
	X, Y, Z  float64
	// SpawnTick is the store tick at which the drop appeared; used for a pickup
	// delay (so the breaker doesn't instantly re-collect) and despawn timeout.
	SpawnTick int64
}

// Store holds all active drops and notifies listeners on spawn/despawn.
type Store struct {
	mu      sync.RWMutex
	drops   map[int64]*Drop
	nextID  atomic.Int64
	tick    atomic.Int64
	onSpawn []func(Drop)
	oneDesp []func(id int64)
}

// New returns an empty drop store. Item entity ids start at 1<<20 to stay clear
// of player entity ids (Java: small ints from 2; Bedrock: 100000+).
func New() *Store {
	s := &Store{drops: make(map[int64]*Drop)}
	s.nextID.Store(1 << 20)
	return s
}

// OnSpawn registers a callback invoked when a drop is added.
func (s *Store) OnSpawn(fn func(Drop)) {
	s.mu.Lock()
	s.onSpawn = append(s.onSpawn, fn)
	s.mu.Unlock()
}

// OnDespawn registers a callback invoked when a drop is removed (pickup/timeout).
func (s *Store) OnDespawn(fn func(id int64)) {
	s.mu.Lock()
	s.oneDesp = append(s.oneDesp, fn)
	s.mu.Unlock()
}

// Spawn adds a drop at a position and notifies listeners. Returns the new drop.
func (s *Store) Spawn(item string, count int, x, y, z float64) Drop {
	d := Drop{
		EntityID:  s.nextID.Add(1),
		Item:      item,
		Count:     count,
		X:         x,
		Y:         y,
		Z:         z,
		SpawnTick: s.tick.Load(),
	}
	s.mu.Lock()
	s.drops[d.EntityID] = &d
	cbs := append([]func(Drop){}, s.onSpawn...)
	s.mu.Unlock()
	for _, cb := range cbs {
		cb(d)
	}
	return d
}

// Remove deletes a drop by id and notifies despawn listeners. Returns false if
// it was already gone (e.g. two players reached it on the same tick).
func (s *Store) Remove(id int64) bool {
	s.mu.Lock()
	_, ok := s.drops[id]
	if ok {
		delete(s.drops, id)
	}
	cbs := append([]func(int64){}, s.oneDesp...)
	s.mu.Unlock()
	if !ok {
		return false
	}
	for _, cb := range cbs {
		cb(id)
	}
	return true
}

// Get returns a copy of the drop with the given id.
func (s *Store) Get(id int64) (Drop, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.drops[id]
	if !ok {
		return Drop{}, false
	}
	return *d, true
}

// All returns a snapshot of every active drop.
func (s *Store) All() []Drop {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Drop, 0, len(s.drops))
	for _, d := range s.drops {
		out = append(out, *d)
	}
	return out
}

// Tick advances the store clock; returns the new tick value. Drop pickup delay
// and despawn timeouts are measured against this.
func (s *Store) Tick() int64 {
	return s.tick.Add(1)
}

// CurrentTick returns the store clock without advancing it.
func (s *Store) CurrentTick() int64 {
	return s.tick.Load()
}
