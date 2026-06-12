// Package aibrain is the Sensor / Memory half of the vanilla AI
// architecture (net.minecraft.world.entity.ai.Brain). The brain
// stores *what the mob knows*: short-lived facts populated by
// sensors at the top of each tick and read by goals.
//
// Memories carry an absolute expiry tick (0 = never expires until
// overwritten). The owning Mob threads a monotonically increasing
// tick counter (m.AITick) so the brain needs no wall clock and stays
// deterministic for tests.
//
// This package is intentionally Mob-free: it owns the data type
// Brain and the Get/Set/Has methods, but the per-tick sensor
// refresh (RunSensors) lives in the systems package to avoid a
// mobs ↔ brain import cycle (mobs imports aibrain for the Brain
// type, aibrain must not import mobs back).
package aibrain

// MemoryKey identifies one slot in the brain.
type MemoryKey int

const (
	MemNearestPlayer MemoryKey = iota
	MemAttackTarget
	MemHurtBy
	MemHurtByTick
	MemGazeTick
	MemAngerTarget
	MemAngerTick
	MemHomePos
)

// MemEntry is one stored fact.
type MemEntry struct {
	Value    any
	ExpireAt int64
}

// Brain is the per-mob memory store. Allocated lazily on first
// write so brain-less mobs cost nothing.
type Brain struct {
	Mem map[MemoryKey]MemEntry
}

// NewBrain returns a fresh brain.
func NewBrain() *Brain { return &Brain{Mem: make(map[MemoryKey]MemEntry, 4)} }

// Set stores value with no expiry.
func (b *Brain) Set(k MemoryKey, v any) { b.Mem[k] = MemEntry{Value: v} }

// SetFor stores value that expires `ttl` ticks after `now`.
func (b *Brain) SetFor(k MemoryKey, v any, now int64, ttl int64) {
	b.Mem[k] = MemEntry{Value: v, ExpireAt: now + ttl}
}

// Clear removes a memory.
func (b *Brain) Clear(k MemoryKey) { delete(b.Mem, k) }

// Get returns the stored value and whether it is present-and-
// unexpired at `now`. Expired entries are pruned on read.
func (b *Brain) Get(k MemoryKey, now int64) (any, bool) {
	e, ok := b.Mem[k]
	if !ok {
		return nil, false
	}
	if e.ExpireAt != 0 && now >= e.ExpireAt {
		delete(b.Mem, k)
		return nil, false
	}
	return e.Value, true
}

// Has is Get without the value.
func (b *Brain) Has(k MemoryKey, now int64) bool {
	_, ok := b.Get(k, now)
	return ok
}

// GetUUID is a typed convenience for the common [16]byte memories.
func (b *Brain) GetUUID(k MemoryKey, now int64) ([16]byte, bool) {
	v, ok := b.Get(k, now)
	if !ok {
		return [16]byte{}, false
	}
	u, ok := v.([16]byte)
	return u, ok
}

// RunSensors is provided here as a thin wrapper that lives in the
// systems package to keep the import graph acyclic. The real
// implementation is in internal/mobs/ai/systems.RunSensors.

// _ ensures the file is non-empty when the brain package is
// imported standalone. The Brain type is used by *mobs.Mob.
var _ = struct{}{}
