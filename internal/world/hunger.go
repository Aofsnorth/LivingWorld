package world

import (
	"strings"
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

// FoodDef describes the vanilla nutritional properties of an edible item.
type FoodDef struct {
	FoodPoints   int
	Saturation   float32
	AlwaysEdible bool
}

var vanillaFoods = map[string]FoodDef{
	"minecraft:apple":                  {FoodPoints: 4, Saturation: 2.4},
	"minecraft:baked_potato":           {FoodPoints: 5, Saturation: 6.0},
	"minecraft:beef":                   {FoodPoints: 3, Saturation: 1.8},
	"minecraft:beetroot":               {FoodPoints: 1, Saturation: 1.2},
	"minecraft:beetroot_soup":          {FoodPoints: 6, Saturation: 7.2},
	"minecraft:bread":                  {FoodPoints: 5, Saturation: 6.0},
	"minecraft:carrot":                 {FoodPoints: 3, Saturation: 3.6},
	"minecraft:chicken":                {FoodPoints: 2, Saturation: 1.2},
	"minecraft:chorus_fruit":           {FoodPoints: 4, Saturation: 2.4, AlwaysEdible: true},
	"minecraft:cod":                    {FoodPoints: 2, Saturation: 0.4},
	"minecraft:cooked_beef":            {FoodPoints: 8, Saturation: 12.8},
	"minecraft:cooked_chicken":         {FoodPoints: 6, Saturation: 7.2},
	"minecraft:cooked_cod":             {FoodPoints: 5, Saturation: 6.0},
	"minecraft:cooked_mutton":          {FoodPoints: 6, Saturation: 9.6},
	"minecraft:cooked_porkchop":        {FoodPoints: 8, Saturation: 12.8},
	"minecraft:cooked_rabbit":          {FoodPoints: 5, Saturation: 6.0},
	"minecraft:cooked_salmon":          {FoodPoints: 6, Saturation: 9.6},
	"minecraft:cookie":                 {FoodPoints: 2, Saturation: 0.4},
	"minecraft:dried_kelp":             {FoodPoints: 1, Saturation: 0.6},
	"minecraft:enchanted_golden_apple": {FoodPoints: 4, Saturation: 9.6, AlwaysEdible: true},
	"minecraft:glow_berries":           {FoodPoints: 2, Saturation: 0.4},
	"minecraft:golden_apple":           {FoodPoints: 4, Saturation: 9.6, AlwaysEdible: true},
	"minecraft:golden_carrot":          {FoodPoints: 6, Saturation: 14.4},
	"minecraft:honey_bottle":           {FoodPoints: 6, Saturation: 1.2, AlwaysEdible: true},
	"minecraft:melon_slice":            {FoodPoints: 2, Saturation: 1.2},
	"minecraft:mushroom_stew":          {FoodPoints: 6, Saturation: 7.2},
	"minecraft:mutton":                 {FoodPoints: 2, Saturation: 1.2},
	"minecraft:poisonous_potato":       {FoodPoints: 2, Saturation: 1.2},
	"minecraft:porkchop":               {FoodPoints: 3, Saturation: 1.8},
	"minecraft:potato":                 {FoodPoints: 1, Saturation: 0.6},
	"minecraft:pufferfish":             {FoodPoints: 1, Saturation: 0.2},
	"minecraft:pumpkin_pie":            {FoodPoints: 8, Saturation: 4.8},
	"minecraft:rabbit":                 {FoodPoints: 3, Saturation: 1.8},
	"minecraft:rabbit_stew":            {FoodPoints: 10, Saturation: 12.0},
	"minecraft:rotten_flesh":           {FoodPoints: 4, Saturation: 0.8},
	"minecraft:salmon":                 {FoodPoints: 2, Saturation: 0.4},
	"minecraft:spider_eye":             {FoodPoints: 2, Saturation: 3.2},
	"minecraft:suspicious_stew":        {FoodPoints: 6, Saturation: 7.2},
	"minecraft:sweet_berries":          {FoodPoints: 2, Saturation: 0.4},
	"minecraft:tropical_fish":          {FoodPoints: 1, Saturation: 0.2},
}

// LookupFood returns the vanilla nutrition stats for an item name if it is edible.
func LookupFood(itemName string) (FoodDef, bool) {
	def, ok := vanillaFoods[itemName]
	return def, ok
}

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
	// syncCallback is called whenever a player's food or saturation changes.
	syncCallback func(playerUUID uuid.UUID, food int, saturation, exhaustion float32)
	// canHealCallback reports whether a player is alive and below max health.
	canHealCallback func(playerUUID uuid.UUID) bool
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

// RemovePlayer removes a player's hunger state when they disconnect.
func (ht *HungerTracker) RemovePlayer(id uuid.UUID) {
	ht.mu.Lock()
	delete(ht.players, id)
	ht.mu.Unlock()
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

// SetSyncCallback registers the function called whenever a player's food or
// saturation changes so protocol bridges can push HUD updates.
func (ht *HungerTracker) SetSyncCallback(fn func(playerUUID uuid.UUID, food int, saturation, exhaustion float32)) {
	ht.mu.Lock()
	ht.syncCallback = fn
	ht.mu.Unlock()
}

// SetCanHealCallback registers an optional predicate reporting whether a player
// is alive and currently below max health (needs natural regeneration).
func (ht *HungerTracker) SetCanHealCallback(fn func(playerUUID uuid.UUID) bool) {
	ht.mu.Lock()
	ht.canHealCallback = fn
	ht.mu.Unlock()
}

// RegisterPlayer initializes hunger state for a player joining the server.
// Default: 20 food, 5.0 saturation, 0 exhaustion (vanilla spawn values).
func (ht *HungerTracker) RegisterPlayer(id uuid.UUID, food int, saturation float32, difficulty string) {
	ht.mu.Lock()
	if food <= 0 {
		food = 20
	} else if food > 20 {
		food = 20
	}
	if saturation < 0 {
		saturation = 5.0
	} else if saturation > float32(food) {
		saturation = float32(food)
	}
	ht.players[id] = &HungerState{
		Food:       food,
		Saturation: saturation,
		Exhaustion: 0,
		Difficulty: difficulty,
	}
	syncCB := ht.syncCallback
	ht.mu.Unlock()
	if syncCB != nil {
		syncCB(id, food, saturation, 0)
	}
}

// ResetPlayer restores a player's hunger state to full (20 food, 5.0 saturation, 0 exhaustion) on respawn.
func (ht *HungerTracker) ResetPlayer(id uuid.UUID, difficulty string) {
	ht.RegisterPlayer(id, 20, 5.0, difficulty)
}

// CanEat reports whether a player can eat a food item right now.
func (ht *HungerTracker) CanEat(id uuid.UUID, alwaysEdible bool) bool {
	if alwaysEdible {
		return true
	}
	ht.mu.RLock()
	s, exists := ht.players[id]
	ht.mu.RUnlock()
	if !exists {
		return false
	}
	return s.Food < 20
}

// Feed adds food points and saturation from eating a food item. Returns true if fed.
func (ht *HungerTracker) Feed(id uuid.UUID, foodPoints int, saturationMod float32) bool {
	ht.mu.Lock()
	s, exists := ht.players[id]
	if !exists {
		ht.mu.Unlock()
		return false
	}
	s.Food += foodPoints
	if s.Food > 20 {
		s.Food = 20
	}
	s.Saturation += saturationMod
	if s.Saturation > float32(s.Food) {
		s.Saturation = float32(s.Food)
	}
	food, sat, exh := s.Food, s.Saturation, s.Exhaustion
	syncCB := ht.syncCallback
	ht.mu.Unlock()
	if syncCB != nil {
		syncCB(id, food, sat, exh)
	}
	return true
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
	if amount <= 0 {
		return
	}
	ht.mu.Lock()
	if s, exists := ht.players[id]; exists {
		if strings.EqualFold(s.Difficulty, "peaceful") {
			s.Exhaustion = 0
		} else {
			s.Exhaustion += amount
		}
	}
	ht.mu.Unlock()
}

// CanSprint reports whether a player has enough food to sprint (food > 6).
func (ht *HungerTracker) CanSprint(id uuid.UUID) bool {
	ht.mu.RLock()
	s, exists := ht.players[id]
	if !exists {
		ht.mu.RUnlock()
		return true
	}
	ok := s.Food > 6
	ht.mu.RUnlock()
	return ok
}

type hungerSyncEvent struct {
	id         uuid.UUID
	food       int
	saturation float32
	exhaustion float32
}

// Tick processes hunger mechanics for all tracked players. Called once per
// 20 Hz tick from the unified tick loop.
func (ht *HungerTracker) Tick() {
	ht.mu.Lock()
	ht.tickCounter++
	tick := ht.tickCounter
	if len(ht.players) == 0 {
		ht.mu.Unlock()
		return
	}
	healCB := ht.healCallback
	starveCB := ht.starveCallback
	syncCB := ht.syncCallback
	canHealCB := ht.canHealCallback

	var heals []uuid.UUID
	var starves []uuid.UUID
	var syncs []hungerSyncEvent

	for id, s := range ht.players {
		prevFood := s.Food
		prevSat := s.Saturation

		if strings.EqualFold(s.Difficulty, "peaceful") {
			s.Exhaustion = 0
			if tick%20 == 0 {
				if s.Food < 20 {
					s.Food++
				}
				if healCB != nil && (canHealCB == nil || canHealCB(id)) {
					heals = append(heals, id)
				}
			}
			if s.Food != prevFood || s.Saturation != prevSat {
				syncs = append(syncs, hungerSyncEvent{
					id:         id,
					food:       s.Food,
					saturation: s.Saturation,
					exhaustion: s.Exhaustion,
				})
			}
			continue
		}

		// Drain exhaustion → saturation → food.
		for s.Exhaustion >= 4.0 {
			s.Exhaustion -= 4.0
			if s.Saturation > 0 {
				s.Saturation -= 1.0
				if s.Saturation < 0 {
					s.Saturation = 0
				}
			} else if s.Food > 0 {
				s.Food--
			}
		}

		// Natural regeneration:
		// 1) Vanilla rapid saturation boost when food == 20 && saturation > 0 (every 10 ticks / 0.5s).
		// 2) Standard saturation/food healing when food >= 18 (every 80 ticks / 4s).
		// Only triggers when the player is alive and actually needs healing (health < maxHealth).
		if s.Food == 20 && s.Saturation > 0 && tick%10 == 0 && healCB != nil {
			if canHealCB == nil || canHealCB(id) {
				heals = append(heals, id)
				s.Exhaustion += 6.0
			}
		} else if s.Food >= 18 && tick%80 == 0 && healCB != nil {
			if canHealCB == nil || canHealCB(id) {
				heals = append(heals, id)
				s.Exhaustion += 6.0
			}
		}

		// Re-drain if natural regeneration added >= 4.0 exhaustion this tick.
		for s.Exhaustion >= 4.0 {
			s.Exhaustion -= 4.0
			if s.Saturation > 0 {
				s.Saturation -= 1.0
				if s.Saturation < 0 {
					s.Saturation = 0
				}
			} else if s.Food > 0 {
				s.Food--
			}
		}

		// Starvation: food == 0, damage every 80 ticks (normal/easy), 50 ticks (hard).
		if s.Food <= 0 && s.Difficulty != "peaceful" {
			interval := uint64(80)
			if s.Difficulty == "hard" {
				interval = 50
			}
			if tick%interval == 0 && starveCB != nil {
				starves = append(starves, id)
			}
		}

		if s.Food != prevFood || s.Saturation != prevSat {
			syncs = append(syncs, hungerSyncEvent{
				id:         id,
				food:       s.Food,
				saturation: s.Saturation,
				exhaustion: s.Exhaustion,
			})
		}
	}
	ht.mu.Unlock()

	if syncCB != nil {
		for _, ev := range syncs {
			syncCB(ev.id, ev.food, ev.saturation, ev.exhaustion)
		}
	}
	if healCB != nil {
		for _, id := range heals {
			healCB(id, 1.0)
		}
	}
	if starveCB != nil {
		for _, id := range starves {
			starveCB(id, 1.0)
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
