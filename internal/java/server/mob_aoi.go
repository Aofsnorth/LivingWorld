// M0.7 — Java-side per-session mob AOI, mirroring the Bedrock version
// in internal/bedrock/server/mob_aoi.go. The pattern is identical:
// each PlayerSession has a mobViewer that records which mobs are
// currently inside its 80-block AOI; the OnMove callback decides
// add / move / remove per session.

package server

import (
	"sync"
)

// mobAOIRadiusSq is the same 80-block squared radius as the Bedrock
// bridge. Kept as a package-level constant in this file too (not
// shared) so the Java bridge compiles standalone if the Bedrock
// file is moved.
const javaMobAOIRadiusSq = 80.0 * 80.0

// mobTracker is the Java-side equivalent of the Bedrock mobTracker.
// Same shape; kept as a separate type so the two packages don't
// accidentally share state.
type javaMobTracker struct {
	mu      sync.Mutex
	spawned map[int64]struct{}
	burning map[int64]struct{} // mobs currently shown on-fire to this session
}

func newJavaMobTracker() *javaMobTracker {
	return &javaMobTracker{spawned: make(map[int64]struct{}), burning: make(map[int64]struct{})}
}

// fireChanged records the on-fire state for a mob and reports whether it
// changed since the last call. The bridge uses it to send the on-fire
// entity-metadata flag only on transitions (not every move tick).
func (t *javaMobTracker) fireChanged(id int64, onFire bool) bool {
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

func (t *javaMobTracker) markSpawned(id int64) {
	t.mu.Lock()
	t.spawned[id] = struct{}{}
	t.mu.Unlock()
}

func (t *javaMobTracker) markDespawned(id int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.spawned[id]
	delete(t.spawned, id)
	return ok
}

func (t *javaMobTracker) isSpawned(id int64) bool {
	t.mu.Lock()
	_, ok := t.spawned[id]
	t.mu.Unlock()
	return ok
}

func (t *javaMobTracker) spawnedIDs() []int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]int64, 0, len(t.spawned))
	for id := range t.spawned {
		out = append(out, id)
	}
	return out
}

// mobInAOI is the squared-distance test. Same 80 b radius as Bedrock.
func mobInAOI(x0, z0, x1, z1 float64) bool {
	dx, dz := x0-x1, z0-z1
	return dx*dx+dz*dz <= javaMobAOIRadiusSq
}
