package server

import (
	"sync"

	"livingworld/internal/player"
	"livingworld/internal/world"

	"github.com/google/uuid"
)

// PROBLEM #9 — cross-edition players vanish after leaving an unloaded chunk.
//
// Avatars used to be spawned exactly once (on join / catch-up) and never
// re-spawned, so when a player walked far enough that the viewer's client
// unloaded the chunk holding them, the avatar was gone for good. viewerTracker
// plus the reconcile methods below implement a per-session Area-Of-Interest:
// foreign players are spawned when they come within view distance and despawned
// when they leave — both directions, idempotently.
//
// NOTE: the tab-list entry (PlayerInfoAdd) is coupled to the avatar spawn, so a
// distant player isn't listed until it's in range. Acceptable for v1 — the
// reported bug is the disappearing AVATAR, not the tab list.

type viewerTracker struct {
	mu sync.Mutex
	// spawned holds the foreign players currently spawned on this client, with
	// their last snapshot so a distance-despawn always has a valid EntityRuntimeID
	// even if the player object is mid-leave.
	spawned map[uuid.UUID]player.PlayerSnapshot
}

func newViewerTracker() *viewerTracker {
	return &viewerTracker{spawned: make(map[uuid.UUID]player.PlayerSnapshot)}
}

// markSpawnedIfAbsent atomically records id as spawned and returns true iff it was
// NOT already spawned (i.e. the caller now owns the single spawn). If already
// spawned it refreshes the cached snapshot and returns false. This is the
// check-and-set that makes spawning race-free when two goroutines reconcile the
// same target concurrently (the boundary-cross sweep vs. the event-loop diff).
func (v *viewerTracker) markSpawnedIfAbsent(id uuid.UUID, snap player.PlayerSnapshot) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.spawned[id]; ok {
		v.spawned[id] = snap // refresh cached snapshot for a future despawn
		return false
	}
	v.spawned[id] = snap
	return true
}

// markDespawnedIfPresent atomically removes id and returns its last snapshot and
// true iff it was spawned (so only one racing despawn actually sends the packet).
func (v *viewerTracker) markDespawnedIfPresent(id uuid.UUID) (player.PlayerSnapshot, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	snap, ok := v.spawned[id]
	if ok {
		delete(v.spawned, id)
	}
	return snap, ok
}

func (v *viewerTracker) spawnedIDs() []uuid.UUID {
	v.mu.Lock()
	defer v.mu.Unlock()
	ids := make([]uuid.UUID, 0, len(v.spawned))
	for id := range v.spawned {
		ids = append(ids, id)
	}
	return ids
}

// inViewRange reports whether p is within this viewer's chunk view distance
// (Chebyshev chunk distance ≤ ViewDistance — the same square window used by chunk
// streaming, so an avatar is spawned exactly when its chunk is loaded).
func (s *PlayerSession) inViewRange(p player.PlayerSnapshot) bool {
	vcx, vcz := s.ChunkX(), s.ChunkZ()
	pcx := world.ChunkCoord(p.Position.X)
	pcz := world.ChunkCoord(p.Position.Z)
	r := int32(s.Bridge.cfg.Java.ViewDistance)
	dx := pcx - vcx
	if dx < 0 {
		dx = -dx
	}
	dz := pcz - vcz
	if dz < 0 {
		dz = -dz
	}
	return dx <= r && dz <= r
}

// reconcileViewerFor handles a single foreign player on EventJoin/EventMove: spawn
// it if it just entered view (then keep relaying its movement), or despawn it if
// it just left. Idempotent via the spawned set. O(1) — the hot path.
func (s *PlayerSession) reconcileViewerFor(p player.PlayerSnapshot) {
	if !s.Ready || p.UUID == s.UUID() {
		return
	}
	if s.inViewRange(p) {
		if s.viewers.markSpawnedIfAbsent(p.UUID, p) {
			s.spawnForeignAvatar(p)
		} else {
			s.moveForeignAvatar(p)
		}
	} else if snap, ok := s.viewers.markDespawnedIfPresent(p.UUID); ok {
		s.removeForeignAvatar(snap)
		s.mu.Lock()
		delete(s.lastSentPos, p.UUID) // re-spawn must re-send an absolute position
		s.mu.Unlock()
	}
}

// despawnViewer forces a despawn on EventLeave (the player object is gone, so
// range can't be recomputed). removeForeignAvatar also sends PlayerInfoRemove to
// clean up the tab list.
func (s *PlayerSession) despawnViewer(p player.PlayerSnapshot) {
	if !s.Ready {
		return
	}
	s.viewers.markDespawnedIfPresent(p.UUID)
	s.removeForeignAvatar(p)
	s.mu.Lock()
	delete(s.lastSentPos, p.UUID)
	s.mu.Unlock()
}

// refreshViewer re-applies a skin change, respecting AOI (re-spawn only if the
// player is in range).
func (s *PlayerSession) refreshViewer(p player.PlayerSnapshot) {
	if !s.Ready {
		return
	}
	if snap, ok := s.viewers.markDespawnedIfPresent(p.UUID); ok {
		s.removeForeignAvatar(snap)
	}
	s.reconcileViewerFor(p)
}

// reconcileViewers is the full AOI sweep run when THIS viewer crosses a chunk
// boundary: spawn every foreign player now in range, despawn any spawned one now
// out of range. O(players) — only on boundary cross, not per movement packet.
func (s *PlayerSession) reconcileViewers() {
	if !s.Ready {
		return
	}
	inRange := make(map[uuid.UUID]struct{})
	for _, p := range s.Bridge.pm.GetAllPlayers() {
		if p.UUID == s.UUID() {
			continue
		}
		snap := p.Snapshot()
		if !s.inViewRange(snap) {
			continue
		}
		inRange[p.UUID] = struct{}{}
		if s.viewers.markSpawnedIfAbsent(p.UUID, snap) {
			s.spawnForeignAvatar(snap)
		}
	}
	for _, id := range s.viewers.spawnedIDs() {
		if _, ok := inRange[id]; ok {
			continue
		}
		if snap, ok := s.viewers.markDespawnedIfPresent(id); ok {
			s.removeForeignAvatar(snap)
			s.mu.Lock()
			delete(s.lastSentPos, id)
			s.mu.Unlock()
		}
	}
}
