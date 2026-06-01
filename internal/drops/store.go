// Package drops tracks dropped item entities that exist in the world after a
// block is broken. The store is edition-neutral: each protocol bridge (Java,
// Bedrock) subscribes, spawns its own item-entity packets for the drops, and
// reports pickups back here. Item identity is the namespaced item name
// ("minecraft:cobblestone"); each edition resolves that to its own network id.
package drops

import (
	"math"
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
	// Velocity for vanilla physics (blocks/tick)
	VX, VY, VZ float64
	// SpawnTick is the store tick at which the drop appeared; used for a pickup
	// delay (so the breaker doesn't instantly re-collect) and despawn timeout.
	SpawnTick int64
	OnGround  bool
}

// Store holds all active drops and notifies listeners on spawn/despawn.
type Store struct {
	mu      sync.RWMutex
	drops   map[int64]*Drop
	nextID  atomic.Int64
	tick    atomic.Int64
	onSpawn []func(Drop)
	oneDesp []func(id int64)
	onMove  []func(Drop)
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

// OnMove registers a callback invoked when a drop changes position (physics tick).
func (s *Store) OnMove(fn func(Drop)) {
	s.mu.Lock()
	s.onMove = append(s.onMove, fn)
	s.mu.Unlock()
}

// TickPhysics integrates one tick of vanilla-ish item physics for every drop:
// gravity, ground settle, air drag and ground friction. solidAt reports whether a
// block coordinate is non-air. Drops whose position changed are reported to
// OnMove listeners (settled drops produce no callback, so grounded items don't
// broadcast every tick). Callbacks fire OUTSIDE the lock to avoid re-entrant
// deadlock, mirroring mobs.Store.Tick.
//
// This is separate from Tick(): Tick() advances the store clock used for pickup
// delay/despawn (the Java pickup loop is its only caller) and must stay untouched.
func (s *Store) TickPhysics(solidAt func(x, y, z int) bool) {
	const (
		gravity        = 0.04
		drag           = 0.98
		groundFriction = 0.6
		settle         = 0.005
	)

	s.mu.Lock()
	moved := make([]Drop, 0, len(s.drops))
	for _, d := range s.drops {
		startX, startY, startZ := d.X, d.Y, d.Z

		d.VY -= gravity

		// Integrate Y, resolving a downward collision by clamping onto the block.
		nextY := d.Y + d.VY
		if d.VY < 0 && solidAt(int(math.Floor(d.X)), int(math.Floor(nextY)), int(math.Floor(d.Z))) {
			d.Y = math.Floor(nextY) + 1 // rest on top of the solid block
			d.VY = 0
			d.OnGround = true
		} else {
			d.Y = nextY
			d.OnGround = false
		}

		// Integrate X/Z, blocking into solid columns (zero the velocity on the
		// blocked axis) so a rolling item can't drift inside a wall.
		if nextX := d.X + d.VX; !solidAt(int(math.Floor(nextX)), int(math.Floor(d.Y)), int(math.Floor(d.Z))) {
			d.X = nextX
		} else {
			d.VX = 0
		}
		if nextZ := d.Z + d.VZ; !solidAt(int(math.Floor(d.X)), int(math.Floor(d.Y)), int(math.Floor(nextZ))) {
			d.Z = nextZ
		} else {
			d.VZ = 0
		}

		d.VX *= drag
		d.VZ *= drag
		d.VY *= drag
		if d.OnGround {
			d.VX *= groundFriction
			d.VZ *= groundFriction
		}

		// Settle: zero tiny residual velocities so a resting item stops moving (and
		// stops emitting move packets).
		if d.OnGround && math.Abs(d.VX) < settle && math.Abs(d.VY) < settle && math.Abs(d.VZ) < settle {
			d.VX, d.VY, d.VZ = 0, 0, 0
		}

		if d.X != startX || d.Y != startY || d.Z != startZ {
			moved = append(moved, *d)
		}
	}
	cbs := append([]func(Drop){}, s.onMove...)
	s.mu.Unlock()

	for _, d := range moved {
		for _, cb := range cbs {
			cb(d)
		}
	}
}

// Spawn adds a drop at a position and notifies listeners. Returns the new drop.
func (s *Store) Spawn(item string, count int, x, y, z float64) Drop {
	id := s.nextID.Add(1)
	// Vanilla random scatter for initial velocity
	randX := (float64(id%200) - 100) / 666.0 // ±0.15 blocks/tick
	randZ := (float64(id%173) - 86) / 666.0

	d := Drop{
		EntityID:  id,
		Item:      item,
		Count:     count,
		X:         x,
		Y:         y,
		Z:         z,
		VX:        randX,
		VY:        0.2, // upward pop
		VZ:        randZ,
		SpawnTick: s.tick.Load(),
		OnGround:  false,
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

// Claim atomically removes a drop WITHOUT firing despawn callbacks. Use it for
// pickups: the edition's take animation (Java ClientboundTakeItemEntity / Bedrock
// TakeItemActor) flies the item to the collector and removes it client-side, so
// sending a despawn (RemoveEntities/RemoveActor) here would delete the entity
// first and cancel the animation. Returns false if the drop was already gone.
func (s *Store) Claim(id int64) bool {
	s.mu.Lock()
	_, ok := s.drops[id]
	if ok {
		delete(s.drops, id)
	}
	s.mu.Unlock()
	return ok
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
