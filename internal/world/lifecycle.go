// Package world — lifecycle.go
//
// This file owns the Manager's background goroutines. The audit (§3) and
// Phase 4a both call out that the legacy per-subsystem loops
// (StartTimeLoop, StartMobAI, StartDropPhysics, StartAutosave) are
// fragmented and need to be replaced by one unified per-world tick
// scheduler. As of Phase 4a those four methods are deprecated thin
// wrappers around the new startTickLoop in tick.go; the bridges
// (internal/bedrock/server, internal/java/server) should call
// startTickLoop directly and stop running their own copy of
// startTimeLoop / startMobSync / startDropLoop.

package world

import (
	"log"
	"math/rand"
	"time"

	"livingworld/internal/mobs"
)

// Save persists all dirty chunks across every world.
func (m *Manager) Save() error {
	var firstErr error
	for _, w := range m.GetAllWorlds() {
		if err := w.Save(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// StartAutosave configures the autosave cadence on the unified tick loop
// and starts the loop if it isn't running yet. Deprecated: prefer
// startTickLoop with an explicit autosaveEvery.
func (m *Manager) StartAutosave(interval time.Duration) {
	if interval <= 0 {
		return
	}
	m.mu.Lock()
	m.tickAutosaver = interval
	alreadyRunning := m.tickStop != nil
	m.mu.Unlock()
	if !alreadyRunning {
		m.startTickLoop(true, interval, 2)
	}
}

// StartTimeLoop ensures the unified tick loop is running. The dayTime-advance
// flag is forwarded into the scheduler. Deprecated: prefer startTickLoop.
func (m *Manager) StartTimeLoop(advance bool) {
	m.mu.Lock()
	alreadyRunning := m.tickStop != nil
	if alreadyRunning {
		m.tickAdvanceTime = advance
	}
	m.mu.Unlock()
	if !alreadyRunning {
		m.startTickLoop(advance, m.tickAutosaver, 2)
	}
}

// StartMobAI ensures the unified tick loop is running with the right
// difficulty. Deprecated: prefer startTickLoop + SetDifficulty.
func (m *Manager) StartMobAI(difficulty string) {
	m.mu.Lock()
	m.difficulty = difficulty
	alreadyRunning := m.tickStop != nil
	m.mu.Unlock()
	if !alreadyRunning {
		m.startTickLoop(true, m.tickAutosaver, 2)
	}
}

// StartWeatherCycle runs the automatic weather director: a clear→rain→thunder
// state machine with vanilla-ish random durations, pushing each transition
// through Manager.SetWeather so BOTH editions stay in sync (the bridges
// already subscribe via OnWeatherChange). enabled=false leaves weather
// frozen at whatever level.json restored, changeable only via /weather.
// Idempotent, mirroring StartTimeLoop's nil-channel guard.
//
// NOTE: weather is intentionally kept on its own 1 Hz loop. The unified
// tick is at 20 Hz and weather phases are seconds long; folding weather
// into the per-50ms path would burn a divide per tick for no benefit.
// Keeping weather separate does not violate the "one tick owner per world"
// rule because weather is non-gameplay state (cosmetic + thunder sky-light)
// and is gated by the manager's existing event-bus path.
func (m *Manager) StartWeatherCycle(enabled bool) {
	if !enabled {
		return
	}
	m.mu.Lock()
	if m.weatherStop != nil {
		m.mu.Unlock()
		log.Printf("[World] StartWeatherCycle: already running, skipping")
		return
	}
	stop := make(chan struct{})
	m.weatherStop = stop
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Second) // weather is second-granular; cheap loop
		defer ticker.Stop()
		rng := rand.New(rand.NewSource(3)) // distinct seed (1=dropRNG, 2=mobAI)

		// Resume from the persisted/current phase so a restart doesn't reset weather.
		raining, thundering := m.GetDefaultWorld().Weather()
		secsLeft := rollWeatherDuration(rng, raining, thundering)

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				secsLeft--
				if secsLeft > 0 {
					continue
				}
				raining, thundering = nextWeather(rng, raining, thundering)
				m.SetWeather(raining, thundering) // off the m.mu critical path
				secsLeft = rollWeatherDuration(rng, raining, thundering)
			}
		}
	}()
}

// nextWeather advances the weather state machine: clear→rain, rain→(50% thunder
// else clear), thunder→clear.
func nextWeather(rng *rand.Rand, raining, thundering bool) (nextRain, nextThunder bool) {
	switch {
	case thundering:
		return false, false // storm passes
	case raining:
		if rng.Float64() < 0.5 {
			return true, true // rain escalates to thunder
		}
		return false, false // rain clears
	default:
		return true, false // clear → rain
	}
}

// rollWeatherDuration returns how many seconds the given phase should last (the
// director ticks at 1 Hz). Ranges are tunable, vanilla-ish.
func rollWeatherDuration(rng *rand.Rand, raining, thundering bool) int {
	switch {
	case thundering:
		return 180 + rng.Intn(420) // 3–10 min
	case raining:
		return 600 + rng.Intn(600) // 10–20 min
	default:
		return 300 + rng.Intn(600) // 5–15 min clear
	}
}

// StartDropPhysics is a no-op when the unified tick loop is already running
// (drop physics is part of runOneTick now). Kept for backward compatibility
// with bridge code that called it explicitly. Deprecated.
func (m *Manager) StartDropPhysics() {
	m.mu.Lock()
	alreadyRunning := m.tickStop != nil
	m.mu.Unlock()
	if !alreadyRunning {
		m.startTickLoop(true, m.tickAutosaver, 2)
	}
}

// Close stops the unified tick loop, the weather loop, and performs a final
// save of all worlds.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.autosaveStop != nil {
		close(m.autosaveStop)
		m.autosaveStop = nil
	}
	if m.timeStop != nil {
		close(m.timeStop)
		m.timeStop = nil
	}
	if m.weatherStop != nil {
		close(m.weatherStop)
		m.weatherStop = nil
	}
	m.mu.Unlock()
	m.stopTickLoop()
	return m.Save()
}

// Compile-time guard: keep mobs in scope so internal references that rely on
// the package (e.g. the legacy StartMobAI wrapper) compile.
var _ = mobs.TickHz
