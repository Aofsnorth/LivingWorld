package server

import (
	"sync"

	"livingworld/internal/player"
	"livingworld/internal/world"

	"github.com/google/uuid"
)

// PROBLEM #9 (Bedrock side) — per-session Area-Of-Interest, mirroring the Java
// implementation. Foreign players (Java or Bedrock) are spawned when they come
// within the viewer's view distance and despawned when they leave, both
// directions, idempotently. Replaces the old spawn-once behavior that left
// players invisible after they walked through an unloaded chunk and back.

type viewerTracker struct {
	mu sync.Mutex
	// spawned holds the foreign players currently spawned on this client, plus
	// their last snapshot so a distance-despawn always has a valid runtime id.
	spawned map[uuid.UUID]player.PlayerSnapshot
}

func newViewerTracker() *viewerTracker {
	return &viewerTracker{spawned: make(map[uuid.UUID]player.PlayerSnapshot)}
}

// markSpawnedIfAbsent atomically records id as spawned and returns true iff it was
// NOT already spawned (so only one of two racing reconcile goroutines actually
// spawns). Bedrock has no per-session serialization queue, so this check-and-set
// is what prevents a duplicate AddPlayer for the same runtime id.
func (v *viewerTracker) markSpawnedIfAbsent(id uuid.UUID, snap player.PlayerSnapshot) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.spawned[id]; ok {
		v.spawned[id] = snap // refresh cached snapshot
		return false
	}
	v.spawned[id] = snap
	return true
}

// markDespawnedIfPresent atomically removes id and returns its last snapshot and
// true iff it was spawned (so only one racing despawn sends RemoveActor).
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

// inViewRange reports whether p is within viewer's chunk view distance (Chebyshev
// chunk distance ≤ view distance). viewer.lastChunkX/Z are maintained by
// publishBedrockMove and seeded by bootstrapWorld.
func (s *Server) inViewRange(viewer *bedrockSession, p player.PlayerSnapshot) bool {
	// viewDistance is always negotiated to cfg.Bedrock.ViewDistance (bootstrapWorld),
	// so use the config value directly — reading the racy per-session field is both
	// unnecessary and a data race. lastChunkX/Z ARE written concurrently (the move
	// goroutine) so snapshot them under bs.mu.
	r := int32(s.cfg.Bedrock.ViewDistance)
	vcx, vcz := viewer.chunkCenter()
	pcx := world.ChunkCoord(p.Position.X)
	pcz := world.ChunkCoord(p.Position.Z)
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
// if it just entered view (then keep moving it), or despawn if it just left.
func (s *Server) reconcileViewerFor(viewer *bedrockSession, p player.PlayerSnapshot, teleport bool) {
	if p.UUID == viewer.id || p.EntityRuntimeID == 0 {
		return
	}
	if s.inViewRange(viewer, p) {
		if viewer.viewers.markSpawnedIfAbsent(p.UUID, p) {
			s.spawnPlayerFor(viewer, p)
		} else {
			s.movePlayerFor(viewer, p, teleport)
		}
	} else if snap, ok := viewer.viewers.markDespawnedIfPresent(p.UUID); ok {
		s.removePlayerFor(viewer, snap)
	}
}

// despawnViewer forces a despawn on EventLeave.
func (s *Server) despawnViewer(viewer *bedrockSession, p player.PlayerSnapshot) {
	viewer.viewers.markDespawnedIfPresent(p.UUID)
	s.removePlayerFor(viewer, p)
}

// reconcileViewers is the full AOI sweep run when THIS viewer crosses a chunk
// boundary: spawn every foreign player now in range, despawn any spawned one now
// out of range.
func (s *Server) reconcileViewers(viewer *bedrockSession) {
	inRange := make(map[uuid.UUID]struct{})
	for _, p := range s.pm.GetAllPlayers() {
		if p.UUID == viewer.id {
			continue
		}
		snap := p.Snapshot()
		if snap.EntityRuntimeID == 0 || !s.inViewRange(viewer, snap) {
			continue
		}
		inRange[p.UUID] = struct{}{}
		if viewer.viewers.markSpawnedIfAbsent(p.UUID, snap) {
			s.spawnPlayerFor(viewer, snap)
		}
	}
	for _, id := range viewer.viewers.spawnedIDs() {
		if _, ok := inRange[id]; ok {
			continue
		}
		if snap, ok := viewer.viewers.markDespawnedIfPresent(id); ok {
			s.removePlayerFor(viewer, snap)
		}
	}
}
