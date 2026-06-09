// M0.7 — per-session mob AOI.
//
// Without AOI, the mob store's OnMove fires for every mob every tick and
// the bridges fan out to every connected session. With ~10 mobs and
// ~10 clients that's 100 packets/tick for the mob broadcast alone, and
// most are useless (the mob is 1000 blocks away and the client has it
// culled).
//
// This file adds a per-session tracker that records which mobs the
// session has been told about. The OnMove callback iterates every
// session and either (a) sends AddActor + move if the mob just entered
// the AOI, (b) sends MoveActorAbsolute if it was already inside, or
// (c) sends RemoveActor if it just left.
//
// AOI radius is 80 blocks (5 chunks). This matches a typical client's
// 6-8 chunk render distance and keeps the per-tick fan-out bounded.

package server

import (
	"math"
	"sync"

	"livingworld/internal/mobs"
)

// mobAOIRadius is the squared horizontal distance used to decide if a
// mob is in this session's AOI. 80 blocks (6400 squared) covers a
// 5-chunk window, which is what a typical Minecraft client renders.
const mobAOIRadiusSq = 80.0 * 80.0

// mobTracker mirrors viewerTracker but for mobs (keyed by int64
// entityID, not UUID). The set tracks "is this mob currently
// rendered on the client?".
type mobTracker struct {
	mu      sync.Mutex
	spawned map[int64]struct{}
	burning map[int64]struct{} // mobs currently shown on-fire to this session
}

// fireChanged records the on-fire state for a mob and reports whether it
// changed since the last call, so the bridge sends a SetActorData flag update
// only on transitions (not every move tick).
func (t *mobTracker) fireChanged(id int64, onFire bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, was := t.burning[id]
	if onFire == was {
		return false
	}
	if onFire {
		t.burning[id] = struct{}{}
	} else {
		delete(t.burning, id)
	}
	return true
}

func newMobTracker() *mobTracker {
	return &mobTracker{spawned: make(map[int64]struct{}), burning: make(map[int64]struct{})}
}

func (t *mobTracker) markSpawned(id int64) {
	t.mu.Lock()
	t.spawned[id] = struct{}{}
	t.mu.Unlock()
}

func (t *mobTracker) markDespawned(id int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.spawned[id]
	delete(t.spawned, id)
	return ok
}

func (t *mobTracker) isSpawned(id int64) bool {
	t.mu.Lock()
	_, ok := t.spawned[id]
	t.mu.Unlock()
	return ok
}

func (t *mobTracker) spawnedIDs() []int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]int64, 0, len(t.spawned))
	for id := range t.spawned {
		out = append(out, id)
	}
	return out
}

// mobInAOI returns true if the squared horizontal distance from the
// viewer to the mob is ≤ 80 blocks. Y is not part of the radius — the
// client renders entities ±64 b vertically regardless of horizontal
// distance, so a mob in the same chunk column 200 b below the
// viewer is still "visible".
func mobInAOI(viewerX, viewerZ, mobX, mobZ float64) bool {
	dx, dz := viewerX-mobX, viewerZ-mobZ
	return dx*dx+dz*dz <= mobAOIRadiusSq
}

// reconcileMobs is called by the bridge mob sync loop. It scans the
// full mob list, and for each session decides which mobs should be
// in vs out of AOI. Used both on join (to seed the session) and on
// viewer chunk-boundary cross (rare).
//
// `add`, `move`, `remove` are callbacks the bridge implements for
// its edition's spawn packets. They take (session, mob).
func reconcileMobs(
	tracker *mobTracker,
	allMobs []mobs.Mob,
	viewerX, viewerZ float64,
	add func(m mobs.Mob),
	move func(m mobs.Mob),
	remove func(m mobs.Mob),
) {
	// First pass: spawn/move.
	inRange := make(map[int64]struct{}, len(allMobs))
	for _, m := range allMobs {
		if !mobInAOI(viewerX, viewerZ, m.X, m.Z) {
			continue
		}
		inRange[m.EntityID] = struct{}{}
		if tracker.isSpawned(m.EntityID) {
			move(m)
		} else {
			add(m)
		}
	}
	// Second pass: despawn.
	for _, id := range tracker.spawnedIDs() {
		if _, ok := inRange[id]; ok {
			continue
		}
		tracker.markDespawned(id)
		// Reconstruct a stub Mob for the remove callback so the
		// bridge has the entityID. Type can be empty.
		remove(mobs.Mob{EntityID: id})
	}
}

// mobInAOIFloat is a math.Hypot version kept for completeness; the
// squared comparison is preferred in the hot path.
func mobInAOIFloat(viewerX, viewerZ, mobX, mobZ float64) bool {
	return math.Hypot(viewerX-mobX, viewerZ-mobZ) <= 80.0
}
