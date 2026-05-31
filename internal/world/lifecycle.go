package world

import (
	"log"
	"math/rand"
	"time"
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

// StartMobAI runs the simple mob AI loop (gravity + random wander) at 5 Hz.
// Idempotent-ish: intended to be called once at startup.
func (m *Manager) StartMobAI() {
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		rng := rand.New(rand.NewSource(2))
		for range ticker.C {
			w := m.GetDefaultWorld()
			m.mobs.Tick(rng, func(x, y, z int) bool {
				return w.GetBlock(x, y, z).ID() != AirID
			})
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
	m.mu.Unlock()
	return m.Save()
}
