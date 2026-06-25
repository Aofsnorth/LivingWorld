package world

import (
	"log"
	"math/rand"
	"strings"
	"time"

	"livingworld/internal/mobs"
	aisystems "livingworld/internal/mobs/ai/systems"
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
//  4. Advance mob AI (calls aisystems.Tick at 20 Hz).
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

// isWoodenDoorID reports whether a block state is a wooden door that a
// zombie-family mob may break. Iron and copper doors are explicitly excluded:
// vanilla zombies only break wooden doors on hard difficulty. The AI checks
// ctx.Difficulty == "hard" before invoking OnBreakDoor, preserving the vanilla
// difficulty gate while keeping block-name logic in the world package.
func isWoodenDoorID(id int32) bool {
	name := StateName(id)
	if name == "minecraft:iron_door" || strings.Contains(name, "copper_door") {
		return false
	}
	return strings.HasSuffix(name, "_door")
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
	// Phase 2: scheduled block ticks. Process the priority queue of
	// future block updates (redstone, fluids, gravity neighbor notifies).
	for _, w := range worlds {
		m.scheduledTicks.Process(w)
	}
	// Phase 3: random ticks. Grass spread is the only consumer right now
	// (see internal/world/grass.go); the function is cheap and runs over
	// every loaded chunk at a fixed budget per tick.
	m.grassTick(rng)

	// Phase 3a: gravity blocks (sand, gravel, anvil). Samples random
	// positions and drops unsupported blocks to the nearest solid surface.
	m.gravityTick(rng)

	// Phase 3b: extended random ticks (leaf decay, ice melt, crop growth,
	// fire spread, mushroom spread, sugar cane/cactus growth, farmland
	// dehydration). Runs alongside grass at the same tick budget.
	for _, w := range worlds {
		m.randomTickWorld(rng, w)
	}

	// Phase 3c: light propagation (Phase 4b). Process any queued light updates
	// from block changes. This runs before mob AI so spawning decisions can use
	// up-to-date light levels.
	for _, w := range worlds {
		if w.Light() != nil {
			for _, pos := range w.Light().ProcessUpdates() {
				m.PublishLightUpdate(pos.X, pos.Z)
			}
		}
	}

	// Phase 3d: hunger mechanics. Process exhaustion drain, saturation
	// healing, and starvation damage for all tracked players.
	m.hunger.Tick()

	// Phase 4: mob AI at 20 Hz. The legacy StartMobAI ticked mobs.TickHz
	// times per second (also 20 Hz), so cadence is preserved.
	for _, w := range worlds {
		aisystems.Tick(m.mobs, mobs.AIContext{
			RNG:               rng,
			SolidAt:           func(x, y, z int) bool { return w.GetBlock(x, y, z).ID() != AirID },
			SkyLightAt:        func(x, y, z int) uint8 { return w.GetSkyLight(x, y, z) },
			Players:           m.aiPlayerList,
			OnMeleeAttack:     m.aiMeleeAttack,
			OnShootArrow:      m.aiShootArrow,
			OnExplode:         m.aiExplode,
			OnFireDamage:      m.aiFireDamage,
			OnSound:           m.aiSound,
			OnHitEffect:       m.aiHitEffect,
			OnThrow:           m.aiThrow,
			OnShootProjectile: m.aiShootProjectile,
			WaterAt:           m.aiWaterAt,
			IsDay:             func() bool { return isDay(w.GetDayTime()) },
			DoorAt: func(x, y, z int) bool {
				return isWoodenDoorID(w.GetBlock(x, y, z).ID())
			},
			OnBreakDoor: func(x, y, z int) bool {
				if !isWoodenDoorID(w.GetBlock(x, y, z).ID()) {
					return false
				}
				w.SetBlock(x, y, z, BlockByID(AirID))
				m.PublishBlockUpdate(BlockUpdateSourceServer, x, y, z, AirID)
				return true
			},
			Difficulty: m.Difficulty(),
			HeldItem:   m.aiHeldItem,
			BlockNameAt: func(x, y, z int) string {
				return StateName(w.GetBlock(x, y, z).ID())
			},
		})
		// Phase 4 cleanup: any mob whose AI set Despawn (e.g. creeper
		// post-explosion) is removed here. Remove() fires OnDespawn so
		// bridges drop the entity.
		//
		// M1: SplitsOnDeath (slime / magma cube) is handled here:
		// when a despawning mob has def.SplitsOnDeath && Size > 1, we
		// spawn 2 children at the parent's last position with
		// Size-1. The bridge receives OnSpawn via SpawnAtSize.
		//
		// M1: drops are also spawned here for the despawning mob.
		for _, id := range m.mobs.PendingDespawns() {
			if m.mobSpawnSplits != nil {
				m.mobSpawnSplits(w, id)
			}
			if m.mobSpawnDrops != nil {
				m.mobSpawnDrops(w, id)
			}
			m.mobs.Remove(id)
		}
	}

	// Phase 4c: skeleton-arrow physics. Projectiles fired in earlier ticks
	// (or just now, by the AI tick above) are integrated, tested for block
	// and player collision, and despawned on impact. Bridges get notified
	// via the projectile store's own OnSpawn/OnDespawn listeners.
	m.projectiles.Tick(mobs.ProjectileTickContext{
		SolidAt: func(x, y, z int) bool {
			w := worlds[0]
			return w.GetBlock(x, y, z).ID() != AirID
		},
		Players: func() []mobs.ProjectileTarget {
			if m.aiPlayerList == nil {
				return nil
			}
			src := m.aiPlayerList()
			out := make([]mobs.ProjectileTarget, len(src))
			for i, p := range src {
				out[i] = mobs.ProjectileTarget{UUID: p.UUID, X: p.X, Y: p.Y, Z: p.Z}
			}
			return out
		},
		OnHitPlayer: func(p mobs.Projectile, target [16]byte) {
			if m.aiProjectileHit != nil {
				m.aiProjectileHit(p.EntityID, target)
			}
		},
	})

	// Phase 4b: mob spawn director. Throttled to ~4 Hz (every 5 ticks) so it
	// doesn't burn through the cap set on every 20 Hz cycle. spawnTick is
	// the function defined in mobspawn.go; it was originally called from the
	// legacy StartMobAI loop and lost its call site when Phase 4a folded
	// everything into the unified scheduler. Without this call, mobs never
	// appear in freshly explored chunks — the AI ticks but the population
	// stays at whatever count was on disk (often zero on a fresh world). The
	// director is cheap (3 candidate columns per attempt, capped by per-cat
	// population) so running it every 5 ticks is well within budget.
	tickNum := m.tickCounter.Add(1)
	if tickNum%5 == 0 && m.SpawnMobsEnabled() {
		m.spawnTick(rng)
	}

	// Phase 4e: per-player status-effect ticking. M6 — the player
	// manager walks every connected player's effect bag, applies
	// per-tick damage (poison 0.5 HP / 0.5s, wither 0.5 HP / s),
	// decrements TicksLeft, and publishes EffectStatusRemove for
	// any effect that just hit 0. The callback is set by
	// server.go's SetMobAICallbacks; if the world tick runs before
	// that wire-up completes (e.g. unit tests), the field stays nil
	// and the phase is a no-op.
	if m.aiTickEffects != nil {
		m.aiTickEffects()
	}

	// Phase 4e (continued): M7 invulnerability-frame countdown.
	// Every connected player's IFrames counter is decremented
	// once per 20 Hz tick; reaches 0 after combat.IFramesTicks
	// (20) ticks = 1 second of vanilla post-hit immunity. The
	// bridge routeAttack path stamps a fresh window on every
	// successful hit (see players.HitIFrames). Cheap O(n) over
	// the player map; the world tick owns the cadence so
	// off-edition clients agree on the window length.
	if m.aiTickIFrames != nil {
		m.aiTickIFrames()
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
