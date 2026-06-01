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

// AI movement tuning. The world loop (StartMobAI) MUST drive Tick at TickHz; the
// effective ground speed a player sees is TickHz * WalkSpeed blocks/sec, so these
// two are coupled — change one and re-check the product against vanilla.
const (
	TickHz    = 20.0  // AI ticks per second the world loop must honor (vanilla is 20 TPS)
	WalkSpeed = 0.075 // blocks/tick while wandering → TickHz*WalkSpeed = 1.5 blocks/sec
)

// Mob is a live mob entity in the world.
type Mob struct {
	EntityID int64     // globally unique; clear of player + drop id spaces
	UUID     uuid.UUID // Java AddEntity needs a UUID
	Type     string    // namespaced type, e.g. "minecraft:pig"
	X, Y, Z  float64
	// Yaw is the body/heading angle in Minecraft degrees (0=+Z south, 90=-X west).
	// Wandering walks forward along Yaw and the bridges send it so the mob faces its
	// movement direction instead of always staring south ("moonwalking").
	Yaw float64
	// walkTicks counts down a wander burst: >0 means "walking forward this many ticks".
	// Unexported on purpose — it is internal AI state, not part of the rendered snapshot.
	walkTicks int
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

// Tick advances simple mob AI. solidAt reports whether a block coordinate is
// non-air. Moved mobs are reported to OnMove listeners. Minimal — no
// pathfinding/targeting — but it produces natural-looking motion:
//
//   - Gravity: fall while the block under the feet is air, then snap onto the
//     surface on landing (no more sinking to a fractional Y).
//   - Heading-based wander: a mob picks a heading (Yaw) and walks forward in short
//     bursts, turning when it hits a wall and idling between bursts. Walking forward
//     along Yaw (instead of jumping to random X/Z each tick) means the mob actually
//     faces where it goes once the bridges send Yaw — fixing the "moves but sideways
//     and jittery" look.
func (s *Store) Tick(rng *rand.Rand, solidAt func(x, y, z int) bool) {
	const (
		gravity = 0.1 // blocks/tick while falling → 2.0 blocks/sec at 20 Hz
		deg2rad = math.Pi / 180
	)
	s.mu.Lock()
	moved := make([]Mob, 0, len(s.mobs))
	for _, m := range s.mobs {
		changed := false

		fx, fz := int(math.Floor(m.X)), int(math.Floor(m.Z))
		if !solidAt(fx, int(math.Floor(m.Y))-1, fz) {
			// Airborne: fall.
			m.Y -= gravity
			changed = true
		} else {
			// On the ground. Snap to the top of the block if we landed mid-fall.
			if m.Y != math.Floor(m.Y) {
				m.Y = math.Floor(m.Y)
				changed = true
			}
			switch {
			case m.walkTicks > 0:
				// Walk forward along the heading. Minecraft yaw: forward = (-sin, +cos).
				rad := m.Yaw * deg2rad
				nx := m.X - math.Sin(rad)*WalkSpeed
				nz := m.Z + math.Cos(rad)*WalkSpeed
				if solidAt(int(math.Floor(nx)), int(math.Floor(m.Y)), int(math.Floor(nz))) {
					// Blocked by a wall/step — turn to a new heading instead of clipping in.
					m.Yaw = rng.Float64()*360 - 180
				} else {
					m.X, m.Z = nx, nz
				}
				m.walkTicks--
				changed = true
			case rng.Float64() < 0.0125:
				// Idle → occasionally start a new walk burst in a fresh direction.
				// 0.0125/tick at 20 Hz ≈ 0.25 walk-starts/sec (same cadence as the old 5 Hz loop).
				m.walkTicks = 80 + rng.Intn(160) // 4–12 s at 20 Hz
				m.Yaw = rng.Float64()*360 - 180
				changed = true // facing changed; broadcast so the client turns the mob
			}
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
