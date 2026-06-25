package world

import (
	"sync"

	"github.com/google/uuid"
)

// Hunger system: tracks per-player food, saturation, and exhaustion.
// Vanilla mechanics:
//   - Sprinting: 0.1 exhaustion per meter
//   - Jumping: 0.05 exhaustion (0.2 while sprinting)
//   - Attacking: 0.1 exhaustion per hit
//   - Taking damage: 0.1 exhaustion per hit
//   - Hunger effect: 0.005 exhaustion per tick
//   - Swimming: 0.01 exhaustion per meter
//   - Breaking block: 0.005 exhaustion
//
// When exhaustion >= 4.0:
//   - Subtract 4.0 from exhaustion
//   - If saturation > 0: subtract 1 from saturation
//   - Else: subtract 1 from food level
//
// When food >= 18: heal 1 HP every 4 seconds (80 ticks) if health < 20
// When food == 0: starve (1 damage per 4 seconds on normal, 1 per 2.5s on hard)
// When food <= 6: cannot sprint

// HungerTracker manages per-player hunger state, independent of the player
// package to avoid import cycles. The server bridges wire this into the
// player manager via callbacks.
type HungerTracker struct {
	mu      sync.RWMutex
	players map[uuid.UUID]*HungerState
	// healCallback is called when a player should heal 1 HP from saturation.
	// The bridge applies the actual health change.
	healCallback func(playerUUID uuid.UUID, amount float32)
	// starveCallback is called when a player should take starvation damage.
	starveCallback func(playerUUID uuid.UUID, amount float32)
	// tickCounter tracks 20 Hz ticks for heal/starve timing.
	tickCounter uint64
}

// HungerState is the per-player hunger tracking state.
type HungerState struct {
	Food       int     // 0-20, visible hunger bar
	Saturation float32 // 0.0-food, invisible buffer (consumed before food)
	Exhaustion float32 // accumulates from actions, drains saturation/food at 4.0

	// Difficulty for starvation damage gating.
	Difficulty string
}

// NewHungerTracker creates a new hunger tracking system.
func NewHungerTracker() *HungerTracker {
	return &HungerTracker{
		players: make(map[uuid.UUID]*HungerState),
	}
}

// SetHealCallback registers the function called when saturation healing fires.
func (ht *HungerTracker) SetHealCallback(fn func(uuid.UUID, float32)) {
	ht.mu.Lock()
	ht.healCallback = fn
	ht.mu.Unlock()
}

// SetStarveCallback registers the function called when starvation damage fires.
func (ht *HungerTracker) SetStarveCallback(fn func(uuid.UUID, float32)) {
	ht.mu.Lock()
	ht.starveCallback = fn
	ht.mu.Unlock()
}

// RegisterPlayer initializes hunger state for a player joining the server.
// Default: 20 food, 5.0 saturation, 0 exhaustion (vanilla spawn values).
func (ht *HungerTracker) RegisterPlayer(id uuid.UUID, food int, saturation float32, difficulty string) {
	ht.mu.Lock()
	if food <= 0 {
		food = 20
	}
	if saturation < 0 {
		saturation = 5.0
	}
	ht.players[id] = &HungerState{
		Food:       food,
		Saturation: saturation,
		Exhaustion: 0,
		Difficulty: difficulty,
	}
	ht.mu.Unlock()
}

// UnregisterPlayer removes a player from the hunger tracker.
func (ht *HungerTracker) UnregisterPlayer(id uuid.UUID) {
	ht.mu.Lock()
	delete(ht.players, id)
	ht.mu.Unlock()
}

// GetState returns the current hunger state for a player.
func (ht *HungerTracker) GetState(id uuid.UUID) (food int, saturation float32, exhaustion float32, ok bool) {
	ht.mu.RLock()
	s, exists := ht.players[id]
	ht.mu.RUnlock()
	if !exists {
		return 20, 5.0, 0, false
	}
	return s.Food, s.Saturation, s.Exhaustion, true
}

// AddExhaustion adds exhaustion to a player's hunger tracker. Called from
// the bridges when a player performs an exhausting action (sprint, jump,
// attack, take damage, break block, swim).
func (ht *HungerTracker) AddExhaustion(id uuid.UUID, amount float32) {
	ht.mu.Lock()
	s, exists := ht.players[id]
	ht.mu.Unlock()
	if !exists {
		return
	}
	s.Exhaustion += amount
}

// CanSprint reports whether a player has enough food to sprint (food > 6).
func (ht *HungerTracker) CanSprint(id uuid.UUID) bool {
	ht.mu.RLock()
	s, exists := ht.players[id]
	ht.mu.RUnlock()
	if !exists {
		return true
	}
	return s.Food > 6
}

// Tick processes hunger mechanics for all tracked players. Called once per
// 20 Hz tick from the unified tick loop.
func (ht *HungerTracker) Tick() {
	ht.mu.Lock()
	ht.tickCounter++
	tick := ht.tickCounter
	healCB := ht.healCallback
	starveCB := ht.starveCallback
	ht.mu.Unlock()

	for id, s := range ht.getPlayersCopy() {
		// Drain exhaustion → saturation → food.
		for s.Exhaustion >= 4.0 {
			s.Exhaustion -= 4.0
			if s.Saturation > 0 {
				s.Saturation--
				if s.Saturation < 0 {
					s.Saturation = 0
				}
			} else if s.Food > 0 {
				s.Food--
			}
		}

		// Saturation healing: food >= 18, heal 1 HP every 80 ticks (4 seconds).
		if s.Food >= 18 && tick%80 == 0 && healCB != nil {
			healCB(id, 1.0)
		}

		// Starvation: food == 0, damage every 80 ticks (normal), 50 ticks (hard).
		// Easy: cap at 10 HP (5 hearts). Normal: cap at 1 HP (0.5 hearts).
		// Hard: no cap (death possible).
		if s.Food <= 0 {
			interval := uint64(80)
			if s.Difficulty == "hard" {
				interval = 50
			}
			if tick%interval == 0 && starveCB != nil {
				starveCB(id, 1.0)
			}
		}
	}
}

// getPlayersCopy returns a shallow copy of the players map for safe iteration.
func (ht *HungerTracker) getPlayersCopy() map[uuid.UUID]*HungerState {
	ht.mu.RLock()
	defer ht.mu.RUnlock()
	out := make(map[uuid.UUID]*HungerState, len(ht.players))
	for k, v := range ht.players {
		out[k] = v
	}
	return out
}

// SetDifficulty updates the difficulty for a player's hunger state.
func (ht *HungerTracker) SetDifficulty(id uuid.UUID, difficulty string) {
	ht.mu.Lock()
	if s, ok := ht.players[id]; ok {
		s.Difficulty = difficulty
	}
	ht.mu.Unlock()
}
