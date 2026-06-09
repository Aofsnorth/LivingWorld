package mobs

import "math"

// Lightweight Brain — the Sensor/Memory half of the vanilla AI architecture
// (net.minecraft.world.entity.ai.Brain). Where the goal selector decides
// *what to do*, the brain stores *what the mob knows*: short-lived facts
// populated by sensors at the top of each tick and read by goals.
//
// This is intentionally minimal for the rombak. The goal selector covers all
// the simple mobs without touching the brain; the brain exists for the
// stateful behaviours the spec calls out — enderman gaze, neutral-mob anger
// timers (Milestone C) — and as the documented extension point for future
// Warden/Villager-grade mobs (vibration listener, work-site memory).
//
// Memories carry an absolute expiry tick (0 = never expires until overwritten).
// The owning Mob threads a monotonically increasing tick counter (m.aiTick) so
// the brain needs no wall clock and stays deterministic for tests.

// MemoryKey identifies one slot in the brain. Keep the set small; add keys as
// stateful behaviours are ported.
type MemoryKey int

const (
	// MemNearestPlayer — UUID of the closest valid player this tick (sensor
	// output, refreshed every tick).
	MemNearestPlayer MemoryKey = iota
	// MemAttackTarget — UUID the mob has committed to attacking. Mirrors
	// m.target; kept in the brain so target goals can reason about it
	// without poking unexported fields across files.
	MemAttackTarget
	// MemHurtBy — UUID of the last entity that damaged the mob.
	MemHurtBy
	// MemHurtByTick — aiTick at which MemHurtBy was recorded (used to expire
	// retaliation interest).
	MemHurtByTick
	// MemGazeTick — aiTick at which a player was last seen looking at this
	// mob (enderman). Aggro is gated on this being recent.
	MemGazeTick
	// MemAngerTarget — UUID a neutral mob is currently angry at.
	MemAngerTarget
	// MemAngerTick — aiTick the anger expires at; past it the mob calms.
	MemAngerTick
	// MemHomePos — encoded home/anchor position (iron golem village,
	// future). Stored as [3]int.
	MemHomePos
)

// memEntry is one stored fact. expireAt is an absolute aiTick; 0 means "valid
// until overwritten".
type memEntry struct {
	value    any
	expireAt int64
}

// aiBrain is the per-mob memory store. Allocated lazily on first write so
// brain-less mobs cost nothing.
type aiBrain struct {
	mem map[MemoryKey]memEntry
}

func newBrain() *aiBrain { return &aiBrain{mem: make(map[MemoryKey]memEntry, 4)} }

// set stores value with no expiry.
func (b *aiBrain) set(k MemoryKey, v any) { b.mem[k] = memEntry{value: v} }

// setFor stores value that expires `ttl` ticks after `now`.
func (b *aiBrain) setFor(k MemoryKey, v any, now int64, ttl int64) {
	b.mem[k] = memEntry{value: v, expireAt: now + ttl}
}

// clear removes a memory.
func (b *aiBrain) clear(k MemoryKey) { delete(b.mem, k) }

// get returns the stored value and whether it is present-and-unexpired at
// `now`. Expired entries are pruned on read.
func (b *aiBrain) get(k MemoryKey, now int64) (any, bool) {
	e, ok := b.mem[k]
	if !ok {
		return nil, false
	}
	if e.expireAt != 0 && now >= e.expireAt {
		delete(b.mem, k)
		return nil, false
	}
	return e.value, true
}

// has is get without the value.
func (b *aiBrain) has(k MemoryKey, now int64) bool {
	_, ok := b.get(k, now)
	return ok
}

// getUUID is a typed convenience for the common [16]byte memories.
func (b *aiBrain) getUUID(k MemoryKey, now int64) ([16]byte, bool) {
	v, ok := b.get(k, now)
	if !ok {
		return [16]byte{}, false
	}
	u, ok := v.([16]byte)
	return u, ok
}

// runSensors refreshes the per-tick sensor memories. Called once at the top of
// aiStep before the selectors run. Cheap probes only; heavy work (LOS) stays
// inside the goals that need it.
func runSensors(m *Mob, ctx *AIContext, players []PlayerTarget) {
	if m.brain == nil {
		return
	}
	// Nearest-player sensor: closest non-spectator/creative player by squared
	// horizontal distance. Targeting goals refine this with range + LOS.
	var best [16]byte
	bestSq := math.MaxFloat64
	found := false
	for i := range players {
		p := &players[i]
		if p.Gamemode == 1 || p.Gamemode == 3 {
			continue
		}
		dx, dz := p.X-m.X, p.Z-m.Z
		sq := dx*dx + dz*dz
		if sq < bestSq {
			bestSq, best, found = sq, p.UUID, true
		}
	}
	if found {
		m.brain.set(MemNearestPlayer, best)
	} else {
		m.brain.clear(MemNearestPlayer)
	}
}
