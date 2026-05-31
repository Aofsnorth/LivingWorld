package world

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// levelData is the small per-world metadata persisted to <worldDir>/level.json
// next to the region files (weather + time of day).
type levelData struct {
	Raining    bool  `json:"raining"`
	Thundering bool  `json:"thundering"`
	DayTime    int64 `json:"dayTime"`
}

// Weather returns the world's current weather state.
func (w *World) Weather() (raining, thundering bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.raining, w.thundering
}

// SetWeather sets the world's weather state (thundering implies raining).
func (w *World) SetWeather(raining, thundering bool) {
	if thundering {
		raining = true
	}
	w.mu.Lock()
	w.raining, w.thundering = raining, thundering
	w.mu.Unlock()
}

func (w *World) loadLevel() {
	w.mu.RLock()
	dir := w.worldDir
	w.mu.RUnlock()
	if dir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(dir, "level.json"))
	if err != nil {
		return
	}
	var ld levelData
	if json.Unmarshal(data, &ld) != nil {
		return
	}
	w.mu.Lock()
	w.raining, w.thundering = ld.Raining, ld.Thundering
	if ld.DayTime > 0 {
		w.dayTime = ld.DayTime
	}
	w.mu.Unlock()
}

func (w *World) saveLevel() {
	w.mu.RLock()
	dir := w.worldDir
	ld := levelData{Raining: w.raining, Thundering: w.thundering, DayTime: w.dayTime}
	w.mu.RUnlock()
	if dir == "" {
		return
	}
	if b, err := json.Marshal(ld); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "level.json"), b, 0o644)
	}
}

// SetWeather sets the default world's weather and notifies any registered
// change listener (the protocol bridges use it to push weather to clients).
func (m *Manager) SetWeather(raining, thundering bool) {
	m.GetDefaultWorld().SetWeather(raining, thundering)
	m.weatherMu.RLock()
	cbs := append([]func(bool, bool){}, m.weatherCallbacks...)
	m.weatherMu.RUnlock()
	for _, cb := range cbs {
		cb(raining, thundering)
	}
}

// OnWeatherChange registers a callback invoked whenever SetWeather is called.
// Multiple listeners are supported (each protocol bridge registers its own).
func (m *Manager) OnWeatherChange(fn func(raining, thundering bool)) {
	m.weatherMu.Lock()
	m.weatherCallbacks = append(m.weatherCallbacks, fn)
	m.weatherMu.Unlock()
}
