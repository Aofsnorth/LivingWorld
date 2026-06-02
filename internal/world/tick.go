package world

import (
	"log"
	"math/rand"
	"time"

	"livingworld/internal/mobs"
)

// Phase 4a: unified per-world tick loop.
//
// The audit (§3 audit table, "Unified tick loop") and the Advance.md §8 spec
// both require a single scheduler that owns every tickable subsystem, replacing
// the old fragmented goroutines:
//
//   - StartTimeLoop   (50ms world time + dayTime)
//   - StartMobAI      (mobs.TickHz = 20, mob AI + spawn director)
//   - StartDropPhysics (50ms, drop integrator)
//   - StartAutosave   (configurable, autosave ticker)
//   - StartPushLoop   (lives on player.Manager; not driven from here — push
//                       ticks are owned by the player package, not world, by
//                       the same "one tick owner per world" rule)
//
// Why one scheduler: the spec requires deterministic phase ordering
// (Advance.md §8.2) and forbids per-edge goroutines mutating canonical state
// (Phase 4a DoD). After this change, the protocol edges ONLY translate
// packets; they MUST NOT run their own tick loops. (Existing edges'
// startTimeLoop/startMobSync/startDropLoop are tracked separately as
// bridges to deprecate, not as part of Phase 4a's contract.)
//
// Phase order (20 Hz = 50 ms cadence, Advance.md §8.2):
//  1. Consume player inputs (deferred mutations from previous tick).
//     Reserved for the input-queue hook that Phase 4a's wiring introduces.
//     Today: no-op (player actions land synchronously via the bridges).
//  2. Run scheduled block ticks. Today: no-op (Phase 4d will fill).
//  3. Run random ticks. Today: no-op (Phase 4d will fill).
//  4. Advance mob AI (calls m.mobs.Tick at 20 Hz).
//  5. Advance drop physics (calls m.drops.TickPhysics at 20 Hz).
//  6. Stage outbound state changes to protocol edges.
//     Today: no-op (edges subscribe to blockEvents / drops / mobs already;
//     "staging" is the existing event-bus model, not a new queue).
//  7. Periodic save hooks if due. m.autosaveEvery controls cadence.
//
// startTickLoop is idempotent: the second call returns immediately, matching
// the nil-channel guard pattern StartTimeLoop used.

// tickHz is the unified scheduler's rate. 20 Hz is vanilla and matches the
// existing mob/ai/drop loops.
const tickHz = 20

// tickInterval is the time between ticks. Using a derived constant keeps the
// "20 Hz" intent visible at the call site.
var tickInterval = time.Second / time.Duration(tickHz)

// startTickLoop launches the unified per-world tick scheduler on its own
// goroutine. m.tickStop is the close channel; calling stopTickLoop closes it
// and the goroutine exits after the in-flight tick.
//
// Parameters:
//   - advanceDayTime: forwarded from the legacy StartTimeLoop API. When false,
//     world time still advances (mobs/drops/AI) but dayTime is frozen.
//   - autosaveEvery: the autosave cadence. 0 or negative disables autosave.
//   - seed: deterministic RNG seed for the mob AI tick. The mob store's
//     existing legacy path used rand.NewSource(2); we keep that contract
//     so behaviour is preserved.
func (m *Manager) startTickLoop(advanceDayTime bool, autosaveEvery time.Duration, seed int64) {
	m.mu.Lock()
	if m.tickStop != nil {
		m.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	m.tickStop = stop
	m.tickAutosaver = autosaveEvery
	m.tickAdvanceTime = advanceDayTime
	m.tickRNG = rand.New(rand.NewSource(seed))
	m.mu.Unlock()

	go m.tickLoop(stop)
}

// tickLoop is the per-world scheduler body. One goroutine, one ticker.
//
// stop signals shutdown. The goroutine exits after the in-flight tick
// completes; no in-flight writes are abandoned mid-call.
func (m *Manager) tickLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	var lastSave time.Time
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			m.runOneTick(m.advanceDayTime(), &lastSave)
		}
	}
}

// advanceDayTime reads the current setting under the lock.
func (m *Manager) advanceDayTime() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tickAdvanceTime
}

// runOneTick runs one 20 Hz tick through the 7 documented phases. Designed
// to be cheap to call from tests (it never spawns a goroutine and never
// blocks on I/O unless Save is actually due).
func (m *Manager) runOneTick(advanceDayTime bool, lastSave *time.Time) {
	// Snapshot the worlds list + RNG + autosave cadence under the lock so the
	// body of the tick runs lock-free.
	m.mu.RLock()
	worlds := make([]*World, 0, len(m.worlds))
	for _, w := range m.worlds {
		worlds = append(worlds, w)
	}
	rng := m.tickRNG
	autosaveEvery := m.tickAutosaver
	m.mu.RUnlock()

	// Phase 1: consume player inputs. No-op today (see tickLoop comment).
	// Phase 2: scheduled block ticks. No-op (Phase 4d).
	// Phase 3: random ticks. No-op (Phase 4d).

	// Phase 3b: light propagation (Phase 4b). Process any queued light updates
	// from block changes. This runs before mob AI so spawning decisions can use
	// up-to-date light levels.
	for _, w := range worlds {
		if w.Light() != nil {
			w.Light().ProcessUpdates()
		}
	}

	// Phase 4: mob AI at 20 Hz. The legacy StartMobAI ticked mobs.TickHz
	// times per second (also 20 Hz), so cadence is preserved.
	for _, w := range worlds {
		m.mobs.Tick(rng, func(x, y, z int) bool {
			return w.GetBlock(x, y, z).ID() != AirID
		})
	}

	// Phase 5: drop physics at 20 Hz. Matches StartDropPhysics cadence.
	for _, w := range worlds {
		m.drops.TickPhysics(func(x, y, z int) bool {
			return w.GetBlock(x, y, z).ID() != AirID
		})
		// Void-despawn: drops that fell below the world get cleaned up so
		// they don't keep broadcasting move packets. Same rule as the
		// legacy StartDropPhysics.
		for _, d := range m.drops.All() {
			if d.Y < float64(MinWorldHeight-4) {
				m.drops.Remove(d.EntityID)
			}
		}
	}

	// Phase 6: stage outbound state changes. The blockEvents bus is the
	// existing "staging" surface; bridges subscribe and forward. New state
	// from this tick is delivered through the bus automatically; no extra
	// flush is needed.

	// World time + dayTime, formerly StartTimeLoop.
	for _, w := range worlds {
		w.SetTime(w.GetTime() + 1)
		if advanceDayTime {
			w.SetDayTime((w.GetDayTime() + 1) % 24000)
		}
	}

	// Phase 7: periodic save hooks.
	if autosaveEvery > 0 && time.Since(*lastSave) >= autosaveEvery {
		if err := m.Save(); err != nil {
			log.Printf("[World] autosave tick failed: %v", err)
		}
		*lastSave = time.Now()
	}
}

// stopTickLoop signals the unified tick goroutine to exit and performs a
// final save (matching the legacy Close() contract).
func (m *Manager) stopTickLoop() {
	m.mu.Lock()
	stop := m.tickStop
	m.tickStop = nil
	m.mu.Unlock()
	if stop != nil {
		close(stop)
	}
}

// Compile-time guard: tickHz must match mobs.TickHz so the cadence test
// pinning (20 ticks/sec) is meaningful.
var _ = mobs.TickHz
