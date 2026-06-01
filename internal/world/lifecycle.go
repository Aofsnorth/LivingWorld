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

// StartAutosave periodically saves all worlds until Close is called. A non-positive
// interval disables autosave.
func (m *Manager) StartAutosave(interval time.Duration) {
	if interval <= 0 {
		return
	}
	m.mu.Lock()
	if m.autosaveStop != nil {
		m.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	m.autosaveStop = stop
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_ = m.Save()
			}
		}
	}()
}

// StartTimeLoop advances every world's clock at 20 ticks/sec. worldAge is
// monotonic; dayTime wraps at 24000. When advance is false the clock is frozen
// (e.g. a fixed-time world) but worldAge still increments. Idempotent.
func (m *Manager) StartTimeLoop(advance bool) {
	m.mu.Lock()
	if m.timeStop != nil {
		m.mu.Unlock()
		log.Printf("[World] StartTimeLoop: already running, skipping")
		return
	}
	stop := make(chan struct{})
	m.timeStop = stop
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(50 * time.Millisecond) // 20 TPS
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				for _, w := range m.GetAllWorlds() {
					w.SetTime(w.GetTime() + 1)
					if advance {
						w.SetDayTime((w.GetDayTime() + 1) % 24000)
					}
				}
			}
		}
	}()
}

// StartMobAI runs the simple mob AI loop (gravity + random wander) at mobs.TickHz
// (20 TPS, matching vanilla so movement is smooth and full-speed) AND the mob-spawn
// director (see mobspawn.go). The director is throttled to ~5 Hz so raising the AI
// rate doesn't also speed up spawning. difficulty gates hostile spawning (peaceful
// suppresses it). Idempotent-ish: intended to be called once at startup.
func (m *Manager) StartMobAI(difficulty string) {
	m.mu.Lock()
	m.difficulty = difficulty
	m.mu.Unlock()
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(mobs.TickHz)) // mobs.TickHz Hz
		defer ticker.Stop()
		rng := rand.New(rand.NewSource(2))
		// Run the spawn director every spawnEvery AI ticks (~5 Hz) so its vanilla-ish
		// pacing and caps are unaffected by the faster movement loop.
		spawnEvery := int(mobs.TickHz / 5)
		if spawnEvery < 1 {
			spawnEvery = 1
		}
		sinceSpawn := 0
		for range ticker.C {
			w := m.GetDefaultWorld()
			m.mobs.Tick(rng, func(x, y, z int) bool {
				return w.GetBlock(x, y, z).ID() != AirID
			})
			if sinceSpawn++; sinceSpawn >= spawnEvery {
				sinceSpawn = 0
				m.spawnTick(rng)
			}
		}
	}()
}

// StartWeatherCycle runs the automatic weather director: a clear→rain→thunder
// state machine with vanilla-ish random durations, pushing each transition
// through Manager.SetWeather so BOTH editions stay in sync (the bridges already
// subscribe via OnWeatherChange). enabled=false leaves weather frozen at whatever
// level.json restored, changeable only via /weather. Idempotent, mirroring
// StartTimeLoop's nil-channel guard.
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

// nextWeather advances the weather state machine: clear→rain, rain→(≈50% thunder
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

// StartDropPhysics runs the item-drop physics integrator at 20 Hz (gravity,
// settle, friction) so dropped items fall, bounce, roll and come to rest, and
// their movement is broadcast to both editions via the drop store's OnMove.
// Runs in server bootstrap (not a bridge) so it ticks regardless of which
// editions are connected. Fire-and-forget, like StartMobAI.
func (m *Manager) StartDropPhysics() {
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond) // 20 TPS
		defer ticker.Stop()
		for range ticker.C {
			w := m.GetDefaultWorld()
			m.drops.TickPhysics(func(x, y, z int) bool {
				return w.GetBlock(x, y, z).ID() != AirID
			})
			// Despawn any drop that fell below the world floor so a void-faller
			// doesn't broadcast a move packet forever (Remove fires OnDespawn →
			// both editions clean it up).
			for _, d := range m.drops.All() {
				if d.Y < float64(MinWorldHeight-4) {
					m.drops.Remove(d.EntityID)
				}
			}
		}
	}()
}

// Close stops autosave and the time loop, then performs a final save of all worlds.
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
	return m.Save()
}
