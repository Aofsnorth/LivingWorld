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

// XPOrb is an experience orb lying in the world, dropped by a mob on death.
// XP orbs follow vanilla pickup rules: any player within 1.5 blocks (0.5 +
// 1.0 buffer) gets the closest one. Awarded amount is the Experience value;
// multiple orbs at the same location stack (vanilla merges).
//
// M7: reuses the same physics integrator as item drops (gravity, ground
// settle, drag) so the bridges can subscribe with one loop. Bridges keep
// a separate renderer for orb packets (Java uses ThrownExperienceBottle /
// ExperienceOrb entity, Bedrock uses packet.ActorEvent + AddActor with
// entity_data_key=37 "xp_value").
type XPOrb struct {
	EntityID   int64
	Experience int
	X, Y, Z    float64
	VX, VY, VZ float64
	SpawnTick  int64
	OnGround   bool
}

// Store holds all active drops and notifies listeners on spawn/despawn.
type Store struct {
	mu       sync.RWMutex
	drops    map[int64]*Drop
	orbs     map[int64]*XPOrb
	nextID   atomic.Int64
	tick     atomic.Int64
	onSpawn  []func(Drop)
	oneDesp  []func(id int64)
	onMove   []func(Drop)
	onOrbSp  []func(XPOrb)
	onOrbDes []func(id int64)
	onOrbMv  []func(XPOrb)
}

// New returns an empty drop store. Item entity ids start at 1<<20 to stay clear
// of player entity ids (Java: small ints from 2; Bedrock: 100000+).
func New() *Store {
	s := &Store{drops: make(map[int64]*Drop), orbs: make(map[int64]*XPOrb)}
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

// OnOrbSpawn registers a callback invoked when an XP orb appears. Bridges
// subscribe to render the orb at the spawn position.
func (s *Store) OnOrbSpawn(fn func(XPOrb)) {
	s.mu.Lock()
	s.onOrbSp = append(s.onOrbSp, fn)
	s.mu.Unlock()
}

// OnOrbDespawn registers a callback invoked when an XP orb is removed
// (player picked it up or despawn timeout). Bridges remove the orb entity
// on the client.
func (s *Store) OnOrbDespawn(fn func(id int64)) {
	s.mu.Lock()
	s.onOrbDes = append(s.onOrbDes, fn)
	s.mu.Unlock()
}

// OnOrbMove registers a callback invoked when an XP orb changes position
// (physics tick).
func (s *Store) OnOrbMove(fn func(XPOrb)) {
	s.mu.Lock()
	s.onOrbMv = append(s.onOrbMv, fn)
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
	orbsMoved := make([]XPOrb, 0, len(s.orbs))
	for _, o := range s.orbs {
		startX, startY, startZ := o.X, o.Y, o.Z
		// XP orbs have stronger gravity (0.03, no — vanilla is 0.05) and a
		// weaker drag so they fly further. Pull toward the nearest player
		// within ~8 blocks is handled by the pickup loop in Tick() (we don't
		// have player positions here). Physics:
		//   gravity  = 0.05
		//   drag     = 0.98
		//   friction = 0.6 on ground
		o.VY -= 0.05
		nextY := o.Y + o.VY
		if o.VY < 0 && solidAt(int(math.Floor(o.X)), int(math.Floor(nextY)), int(math.Floor(o.Z))) {
			o.Y = math.Floor(nextY) + 1
			o.VY = 0
			o.OnGround = true
		} else {
			o.Y = nextY
			o.OnGround = false
		}
		if nextX := o.X + o.VX; !solidAt(int(math.Floor(nextX)), int(math.Floor(o.Y)), int(math.Floor(o.Z))) {
			o.X = nextX
		} else {
			o.VX = 0
		}
		if nextZ := o.Z + o.VZ; !solidAt(int(math.Floor(o.X)), int(math.Floor(o.Y)), int(math.Floor(nextZ))) {
			o.Z = nextZ
		} else {
			o.VZ = 0
		}
		o.VX *= drag
		o.VZ *= drag
		o.VY *= drag
		if o.OnGround {
			o.VX *= groundFriction
			o.VZ *= groundFriction
		}
		if o.OnGround && math.Abs(o.VX) < settle && math.Abs(o.VY) < settle && math.Abs(o.VZ) < settle {
			o.VX, o.VY, o.VZ = 0, 0, 0
		}
		if o.X != startX || o.Y != startY || o.Z != startZ {
			orbsMoved = append(orbsMoved, *o)
		}
	}
	orbCbs := append([]func(XPOrb){}, s.onOrbMv...)
	s.mu.Unlock()

	for _, d := range moved {
		for _, cb := range cbs {
			cb(d)
		}
	}
	for _, o := range orbsMoved {
		for _, cb := range orbCbs {
			cb(o)
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

// SpawnFromPlayer is a Spawn variant used when a player actively drops an item
// (Q / Ctrl+Q). The item gets a stronger forward throw derived from the look
// yaw so it visibly arcs away from the player instead of just popping up in
// place like a broken-block drop. yaw is in degrees, 0 = +Z, 90 = -X.
func (s *Store) SpawnFromPlayer(item string, count int, x, y, z, yaw float64) Drop {
	rad := yaw * math.Pi / 180.0
	// Vanilla Player.drop throws with horizontal speed ~0.3 blocks/tick along
	// the look direction and 0.4 upward.
	throwX := -math.Sin(rad) * 0.3
	throwZ := math.Cos(rad) * 0.3
	id := s.nextID.Add(1)
	d := Drop{
		EntityID:  id,
		Item:      item,
		Count:     count,
		X:         x,
		Y:         y,
		Z:         z,
		VX:        throwX,
		VY:        0.4,
		VZ:        throwZ,
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

// SpawnXP drops an experience orb at the given position and notifies
// orb-spawn listeners. Amount is the orb's `Experience` field; vanilla
// awards stack the awarded total into the orb value rather than spawning
// many small orbs. Bridges render the orb at the spawn position and
// handle pickup server-side. Returns the new orb.
func (s *Store) SpawnXP(amount int, x, y, z float64) XPOrb {
	id := s.nextID.Add(1)
	// Vanilla throws the orb toward the killer with a small forward velocity.
	randX := (float64(id%200) - 100) / 666.0
	randZ := (float64(id%173) - 86) / 666.0
	o := XPOrb{
		EntityID:   id,
		Experience: amount,
		X:          x,
		Y:          y,
		Z:          z,
		VX:         randX,
		VY:         0.2,
		VZ:         randZ,
		SpawnTick:  s.tick.Load(),
		OnGround:   false,
	}
	s.mu.Lock()
	s.orbs[o.EntityID] = &o
	cbs := append([]func(XPOrb){}, s.onOrbSp...)
	s.mu.Unlock()
	for _, cb := range cbs {
		cb(o)
	}
	return o
}

// RemoveOrb deletes an XP orb by id and notifies orb-despawn listeners.
// Used for both pickup (after awarding XP to the player) and timeout
// despawn. Returns false if the orb was already gone.
func (s *Store) RemoveOrb(id int64) bool {
	s.mu.Lock()
	_, ok := s.orbs[id]
	if ok {
		delete(s.orbs, id)
	}
	cbs := append([]func(int64){}, s.onOrbDes...)
	s.mu.Unlock()
	if !ok {
		return false
	}
	for _, cb := range cbs {
		cb(id)
	}
	return true
}

// Orbs returns a snapshot of every active XP orb. Used by the pickup loop
// to find the closest orb for a given player.
func (s *Store) Orbs() []XPOrb {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]XPOrb, 0, len(s.orbs))
	for _, o := range s.orbs {
		out = append(out, *o)
	}
	return out
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
