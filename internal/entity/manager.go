// Package entity implements LivingWorld's edition-agnostic entity layer on top
// of the canonical model (registry.Entity): a live-entity manager with network
// id allocation, spawning and despawning. AI, pathfinding and metadata sync
// build on this. See REQUIREMENTS R5.1.
package entity

import (
	"sync"

	"livingworld/internal/registry"

	"github.com/google/uuid"
)

// Manager owns the set of live entities in a world and allocates their network
// ids. It is safe for concurrent use.
type Manager struct {
	mu     sync.RWMutex
	nextID int32
	ents   map[int32]*registry.Entity
}

// NewManager returns an empty entity manager.
func NewManager() *Manager {
	return &Manager{ents: make(map[int32]*registry.Entity)}
}

// Spawn creates and registers a new entity of the given type at pos, assigning
// a fresh network id and UUID.
func (m *Manager) Spawn(typ string, pos registry.Vec3) *registry.Entity {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	e := &registry.Entity{
		ID:   m.nextID,
		UUID: uuid.New(),
		Type: typ,
		Pos:  pos,
		Meta: registry.MetaMap{},
	}
	m.ents[e.ID] = e
	return e
}

// Despawn removes an entity by id, reporting whether it existed.
func (m *Manager) Despawn(id int32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.ents[id]; !ok {
		return false
	}
	delete(m.ents, id)
	return true
}

// Get returns the entity with the given id.
func (m *Manager) Get(id int32) (*registry.Entity, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.ents[id]
	return e, ok
}

// All returns a snapshot of live entities (for ticking / viewer broadcast).
func (m *Manager) All() []*registry.Entity {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*registry.Entity, 0, len(m.ents))
	for _, e := range m.ents {
		out = append(out, e)
	}
	return out
}

// Count returns the number of live entities.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.ents)
}
