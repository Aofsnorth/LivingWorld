// Package mobs tracks live mob entities, edition-neutral like package drops:
// each protocol bridge subscribes (OnSpawn/OnDespawn) and renders the mob with
// its own spawn packet (Java AddEntity, Bedrock AddActor). Mob identity is the
// namespaced type name ("minecraft:pig"); each edition maps it to its entity id.
//
// v1 mobs are static (spawn/visible/removable cross-edition); AI/pathfinding is
// a separate, larger effort.
package mobs

import (
	"math"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// Mob is a live mob entity in the world.
type Mob struct {
	EntityID int64     // globally unique; clear of player + drop id spaces
	UUID     uuid.UUID // Java AddEntity needs a UUID
	Type     string    // namespaced type, e.g. "minecraft:pig"
	X, Y, Z  float64
}

// Store holds active mobs and notifies listeners on spawn/despawn.
type Store struct {
	mu      sync.RWMutex
	mobs    map[int64]*Mob
	nextID  atomic.Int64
	onSpawn []func(Mob)
	onDesp  []func(id int64)
	onMove  []func(Mob)
}

// New returns an empty mob store. Mob entity ids start at 1<<22 to stay clear of
// player ids (Java small ints / Bedrock 100000+) and drop ids (1<<20).
func New() *Store {
	s := &Store{mobs: make(map[int64]*Mob)}
	s.nextID.Store(1 << 22)
	return s
}

// OnSpawn registers a callback invoked when a mob is added.
func (s *Store) OnSpawn(fn func(Mob)) {
	s.mu.Lock()
	s.onSpawn = append(s.onSpawn, fn)
	s.mu.Unlock()
}

// OnDespawn registers a callback invoked when a mob is removed.
func (s *Store) OnDespawn(fn func(id int64)) {
	s.mu.Lock()
	s.onDesp = append(s.onDesp, fn)
	s.mu.Unlock()
}

// Spawn adds a mob of the given namespaced type and notifies listeners.
func (s *Store) Spawn(mobType string, x, y, z float64) Mob {
	m := Mob{EntityID: s.nextID.Add(1), UUID: uuid.New(), Type: mobType, X: x, Y: y, Z: z}
	s.mu.Lock()
	s.mobs[m.EntityID] = &m
	cbs := append([]func(Mob){}, s.onSpawn...)
	s.mu.Unlock()
	for _, cb := range cbs {
		cb(m)
	}
	return m
}

// Remove deletes a mob by id and notifies despawn listeners.
func (s *Store) Remove(id int64) bool {
	s.mu.Lock()
	_, ok := s.mobs[id]
	if ok {
		delete(s.mobs, id)
	}
	cbs := append([]func(int64){}, s.onDesp...)
	s.mu.Unlock()
	if !ok {
		return false
	}
	for _, cb := range cbs {
		cb(id)
	}
	return true
}

// OnMove registers a callback invoked when a mob moves (AI tick).
func (s *Store) OnMove(fn func(Mob)) {
	s.mu.Lock()
	s.onMove = append(s.onMove, fn)
	s.mu.Unlock()
}

// Tick advances simple mob AI: gravity (fall while the block below is air) and
// random wander on X/Z. solidAt reports whether a block coordinate is non-air.
// Moved mobs are reported to OnMove listeners. Minimal — no pathfinding/targeting.
func (s *Store) Tick(rng *rand.Rand, solidAt func(x, y, z int) bool) {
	s.mu.Lock()
	moved := make([]Mob, 0, len(s.mobs))
	for _, m := range s.mobs {
		changed := false
		if !solidAt(int(math.Floor(m.X)), int(math.Floor(m.Y))-1, int(math.Floor(m.Z))) {
			m.Y -= 0.5
			changed = true
		}
		if rng.Float64() < 0.3 {
			m.X += (rng.Float64()*2 - 1) * 0.3
			m.Z += (rng.Float64()*2 - 1) * 0.3
			changed = true
		}
		if changed {
			moved = append(moved, *m)
		}
	}
	cbs := append([]func(Mob){}, s.onMove...)
	s.mu.Unlock()
	for _, m := range moved {
		for _, cb := range cbs {
			cb(m)
		}
	}
}

// All returns a snapshot of every active mob (for catch-up spawning on join).
func (s *Store) All() []Mob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Mob, 0, len(s.mobs))
	for _, m := range s.mobs {
		out = append(out, *m)
	}
	return out
}
