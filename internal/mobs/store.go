package mobs

import (
	"livingworld/internal/combat"
	// Alias the AI subpackages so the Mob struct can use
	// short, readable type names without colliding with the
	// "ai" prefix. The aliases match the convention used in
	// the rest of the codebase: `selector`, `brain`,
	// `navigation`.
	"livingworld/internal/registry"

	brain "livingworld/internal/mobs/ai/brain"
	aictx "livingworld/internal/mobs/ai/context"
	navigation "livingworld/internal/mobs/ai/navigation"
	selector "livingworld/internal/mobs/ai/selector"

	"math"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// Mob re-exports the alias types from internal/mobs/ai/context so
// existing call sites don't need to change imports.
var _ = aictx.ZeroUUID
var _ = aictx.FindPlayer

// MinWorldHeightForMobAI is the floor Y the AI clamps mobs to. Mirrors
// world.MinWorldHeight without an import cycle (the world package
// imports mobs).
const MinWorldHeightForMobAI = -64

// TickHz is the mob AI cadence used by the unified world tick loop.
const TickHz = 20.0

// AIState is the per-mob high-level behaviour. The tree in
// internal/mobs/ai/systems switches on it to decide what to do this
// tick.
type AIState int

const (
	StateIdle AIState = iota
	StatePursue
	StateFlee
	StateMelee
	StateShoot
	StateFuse
	StateGraze
	StateAlert
	StateFollow
)

// Mob is a live mob entity in the world. The exported fields are
// the cross-edition snapshot; AI-internal state lives in the
// unexported fields and is never serialised to bridges.
type Mob struct {
	EntityID  int64
	UUID      uuid.UUID
	Type      string
	X, Y, Z   float64
	Yaw       float64
	HeadYaw   float64
	HeadPitch float64
	OnGround  bool

	Despawn bool

	// --- AI engine: goal-selector + brain, built lazily on first
	// Tick from the mob's MobDef via buildAI (lives in
	// internal/mobs/ai/systems). ---
	GoalSel   *selector.GoalSelector
	TargetSel *selector.GoalSelector
	Brain     *brain.Brain
	Nav       *navigation.NavProfile
	AITick    int64

	// --- per-mob AI state (exported so the AI subpackages can
	// read/write without an import cycle) ---
	State         AIState
	WalkTicks     int
	LookTicks     int
	LookYawTarget float64
	CooldownTicks int
	Target        [16]byte
	HurtBy        [16]byte
	HurtByTick    int64
	FuseTicks     int
	JumpCooldown  int
	Vy            float64
	AmbientCD     int

	Path     Path
	PathIdx  int
	PathTick int

	DrawTicks int

	HP        float32
	MaxHP     float32
	FireTicks int

	Size int

	KnockbackVX, KnockbackVY, KnockbackVZ float64

	NoPlayerTicks int

	HeadStillTicks int
	StuckTicks     int

	PersistenceRequired bool
	Baby                bool
}

// Store holds active mobs and notifies listeners on spawn/despawn/move.
type Store struct {
	mu      sync.RWMutex
	mobs    map[int64]*Mob
	nextID  atomic.Int64
	onSpawn []func(Mob)
	onDesp  []func(id int64)
	onMove  []func(Mob)
	onDeath []func(Mob)
}

// New returns an empty mob store.
func New() *Store {
	s := &Store{mobs: make(map[int64]*Mob)}
	s.nextID.Store(1 << 22)
	return s
}

// OnSpawn registers a callback invoked when a mob is added.
func (s *Store) OnSpawn(fn func(Mob)) {
	s.mu.Lock()
	s.onSpawn = append(s.onSpawn, fn)
	s.mu.Unlock()
}

// OnDespawn registers a callback invoked when a mob is removed.
func (s *Store) OnDespawn(fn func(id int64)) {
	s.mu.Lock()
	s.onDesp = append(s.onDesp, fn)
	s.mu.Unlock()
}

// OnMove registers a callback invoked when a mob's position or
// head yaw changes during an AI tick.
func (s *Store) OnMove(fn func(Mob)) {
	s.mu.Lock()
	s.onMove = append(s.onMove, fn)
	s.mu.Unlock()
}

// OnDeath registers a callback fired when a mob's HP drops to 0
// from a damage event.
func (s *Store) OnDeath(fn func(Mob)) {
	s.mu.Lock()
	s.onDeath = append(s.onDeath, fn)
	s.mu.Unlock()
}

// OnMoveCBs returns a snapshot of the on-move listener slice.
// Used by the AI subpackages (which run outside the store lock)
// to fan out movement events.
func (s *Store) OnMoveCBs() []func(Mob) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]func(Mob){}, s.onMove...)
}

// Mobs returns the live mob map. The AI subpackages that run under
// the store lock use this to iterate.
func (s *Store) Mobs() map[int64]*Mob { return s.mobs }

// MuLock locks the store's RWMutex. The AI subpackages take it
// during the per-tick pass; world-layer callbacks release it.
func (s *Store) MuLock() { s.mu.Lock() }

// MuUnlock releases the store's RWMutex.
func (s *Store) MuUnlock() { s.mu.Unlock() }

// Spawn adds a mob of the given namespaced type and notifies
// listeners.
func (s *Store) Spawn(mobType string, x, y, z float64) Mob {
	return s.SpawnAtSize(mobType, x, y, z, 0)
}

// SpawnAtSize is Spawn with an explicit Size (M1 — slime / magma
// cube splits).
func (s *Store) SpawnAtSize(mobType string, x, y, z float64, size int) Mob {
	def := DefFor(mobType)
	if size == 0 {
		size = def.Size
	}
	m := Mob{
		EntityID: s.nextID.Add(1),
		UUID:     uuid.New(),
		Type:     mobType,
		X:        x, Y: y, Z: z,
		Yaw:      rand.Float64()*360 - 180,
		OnGround: true,
		HP:       def.MaxHP,
		MaxHP:    def.MaxHP,
		Size:     size,
		State:    StateIdle,
	}
	m.HeadYaw = m.Yaw
	s.mu.Lock()
	s.mobs[m.EntityID] = &m
	cbs := append([]func(Mob){}, s.onSpawn...)
	s.mu.Unlock()
	for _, cb := range cbs {
		cb(m)
	}
	return m
}

// Remove deletes a mob by id and notifies despawn listeners.
func (s *Store) Remove(id int64) bool {
	s.mu.Lock()
	_, ok := s.mobs[id]
	if ok {
		delete(s.mobs, id)
	}
	cbs := append([]func(int64){}, s.onDesp...)
	s.mu.Unlock()
	if !ok {
		return false
	}
	for _, cb := range cbs {
		cb(id)
	}
	return true
}

// All returns a snapshot of every active mob.
func (s *Store) All() []Mob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Mob, 0, len(s.mobs))
	for _, m := range s.mobs {
		out = append(out, *m)
	}
	return out
}

// Get returns a copy of the mob with the given EntityID, or zero
// Mob if not found.
func (s *Store) Get(id int64) Mob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.mobs[id]
	if !ok {
		return Mob{}
	}
	return *m
}

// PendingDespawns returns the EntityIDs of every mob whose Despawn
// flag was set during the last tick.
func (s *Store) PendingDespawns() []int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]int64, 0)
	for _, m := range s.mobs {
		if m.Despawn {
			out = append(out, m.EntityID)
		}
	}
	return out
}

// Hurt records an attack on this mob from the given player.
func (s *Store) Hurt(mobID int64, attacker [16]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.mobs[mobID]
	if !ok {
		return
	}
	m.HurtBy = attacker
	m.HurtByTick = m.AITick
	def := DefFor(m.Type)
	if def.IsHostile {
		var zero [16]byte
		if m.Target == zero {
			m.Target = attacker
			m.State = StatePursue
		}
	} else {
		// M7.10: vanilla passive-mob panic duration is
		// 2-4 s (40-80 ticks).
		// Stored in unexported `panicTicks` for legacy callers.
		_ = 40 + rand.Intn(40) // 2-4s at 20 Hz (kept for compat)
		m.State = StateFlee
	}
}

// HurtFire applies fire damage from the M0.6 sun-burn path.
func (s *Store) HurtFire(mobID int64, damage float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.mobs[mobID]
	if !ok {
		return
	}
	def := DefFor(m.Type)
	if def.FireImmune {
		return
	}
	if m.FireTicks < 60 {
		m.FireTicks = 60
	}
	m.HP -= damage
	if m.HP <= 0 {
		m.Despawn = true
	}
}

// HurtDirect applies direct damage from a player melee swing.
func (s *Store) HurtDirect(mobID int64, attacker [16]byte, damage float32) {
	s.HurtDirectWithKnockback(mobID, attacker, damage, 0, 0, 0)
}

// HurtDirectWithKnockback (M7) applies direct damage AND sets a
// knockback velocity on the mob.
func (s *Store) HurtDirectWithKnockback(mobID int64, attacker [16]byte, damage float32, dirX, dirZ, kb float64) {
	var deathSnap *Mob
	var deathCbs []func(Mob)
	s.mu.Lock()
	m, ok := s.mobs[mobID]
	if !ok {
		s.mu.Unlock()
		return
	}
	m.HurtBy = attacker
	m.HurtByTick = m.AITick
	def := DefFor(m.Type)
	if def.IsHostile {
		var zero [16]byte
		if m.Target == zero {
			m.Target = attacker
			m.State = StatePursue
		}
	}
	m.HP -= damage
	if m.HP <= 0 {
		m.Despawn = true
		snap := *m
		deathSnap = &snap
		deathCbs = append(deathCbs, s.onDeath...)
	}
	if kb > 0 {
		resistance := 0.0
		if def.NoKnockback {
			resistance = 1.0
		}
		nv := combat.Knockback(registry.Vec3{Y: m.Vy}, kb, dirX, dirZ, resistance, m.OnGround)
		m.KnockbackVX = nv.X
		m.KnockbackVZ = nv.Z
		m.KnockbackVY = nv.Y - m.Vy
	}
	s.mu.Unlock()
	if deathSnap != nil {
		for _, cb := range deathCbs {
			cb(*deathSnap)
		}
	}
}

// TickFire decrements every mob's FireTicks. Called once per world
// tick from world/tick.go (Phase 4d) so it runs after the AI tick.
func (s *Store) TickFire() []Mob {
	s.mu.Lock()
	defer s.mu.Unlock()
	var burning []Mob
	for _, m := range s.mobs {
		if m.FireTicks > 0 {
			m.FireTicks--
			burning = append(burning, *m)
		}
	}
	return burning
}

// --- M0.3 path helpers (exported for the AI subpackages) ---

// HasPath reports whether m currently has a non-empty A* path it
// is walking along.
func (m *Mob) HasPath() bool {
	return len(m.Path) > 0 && m.PathIdx < len(m.Path)
}

// PathGoalStale is true when the path's last node is more than 2
// cells from the requested goal cell.
func (m *Mob) PathGoalStale(goal PathNode) bool {
	if len(m.Path) == 0 {
		return true
	}
	last := m.Path[len(m.Path)-1]
	return absI(last.X-goal.X) > 2 || absI(last.Z-goal.Z) > 2
}

// PathDirection returns the (dx, dz) unit vector from the mob's
// cell to the next waypoint, plus the next waypoint's Y.
func (m *Mob) PathDirection(mx, mz int) (dx, dz float64, y int, ok bool) {
	if !m.HasPath() {
		return 0, 0, 0, false
	}
	np := m.Path[m.PathIdx]
	dx, dz = float64(np.X-mx), float64(np.Z-mz)
	if d := math.Hypot(dx, dz); d > 0 {
		dx, dz = dx/d, dz/d
	}
	if mx == np.X && mz == np.Z {
		m.PathIdx++
	}
	return dx, dz, np.Y, true
}

// ReplanPath runs A* from from to to and stores the result on m.
func (m *Mob) ReplanPath(from, to PathNode, ctx AIContext) {
	wq := func(x, y, z int) bool {
		if ctx.SolidAt(x, y, z) {
			return false
		}
		if ctx.SolidAt(x, y+1, z) {
			return false
		}
		return true
	}
	var cost CostFn
	if m.Nav != nil {
		cost = m.Nav.Cost(&ctx)
	} else {
		def := DefFor(m.Type)
		prof := navigation.NavProfileFor(m.Type, def.Movement)
		cost = prof.Cost(&ctx)
	}
	res := PathFind(from, to, wq, cost)
	if res.Found {
		m.Path = res.Nodes
		m.PathIdx = 0
		m.PathTick = 20
	} else {
		m.Path = nil
		m.PathIdx = 0
		m.PathTick = 0
	}
}

func absI(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
