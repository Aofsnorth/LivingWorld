// Package mobs tracks live mob entities, edition-neutral like package drops:
// each protocol bridge subscribes (OnSpawn/OnDespawn) and renders the mob with
// its own spawn packet (Java AddEntity, Bedrock AddActor). Mob identity is the
// namespaced type name ("minecraft:pig"); each edition maps it to its own entity id.
//
// AI is per-tick, driven by Manager.runOneTick via Store.Tick. The AI itself
// lives in ai.go; this file owns the state, the store, and the OnSpawn/
// OnDespawn/OnMove event bus. Bridges add their own render code on top of the
// snapshot data exposed here.
package mobs

import (
	"math"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// AI movement tuning. The world loop (Manager.runOneTick) MUST drive Tick at
// TickHz; the effective ground speed a player sees is TickHz * WalkSpeed
// blocks/sec. Vanilla mobs use attribute * 0.1 blocks/tick at 20 Hz, so the
// per-tick delta inside ai.go is def.WanderSpeed * 0.1 (see aiStep).
const TickHz = 20.0

// AIState is the per-mob high-level behaviour. The tree in ai.go switches on
// it to decide what to do this tick.
type AIState int

const (
	StateIdle     AIState = iota // wandering aimlessly
	StatePursue                  // hostile: chasing a target
	StateFlee                    // passive: panic-running from a recent attacker
	StateMelee                   // hostile: in attack range, swinging on cooldown
	StateShoot                   // skeleton: bow drawn, will fire on next AI tick
	StateFuse                    // creeper: hissing, will explode at fuse == 0
)

// Mob is a live mob entity in the world. The exported fields are the
// cross-edition snapshot; AI-internal state (walkTicks, target, etc.) lives in
// the unexported fields and is never serialised to bridges.
type Mob struct {
	EntityID int64     // globally unique; clear of player + drop id spaces
	UUID     uuid.UUID // Java AddEntity needs a UUID
	Type     string    // namespaced type, e.g. "minecraft:pig"
	X, Y, Z  float64
	// Yaw is the body/heading angle in Minecraft degrees (0=+Z south, 90=-X west).
	// Wandering walks forward along Yaw and the bridges send it so the mob faces its
	// movement direction instead of always staring south ("moonwalking").
	Yaw float64
	// HeadYaw is the head-only rotation, decoupled from body when the mob is
	// looking at something without turning. Defaults to Yaw; look-at-entity
	// overrides it when a player is within 6 blocks.
	HeadYaw float64
	// OnGround is the server's view of the mob's grounding; used for the
	// OnMove broadcast so the client can stop running its own gravity.
	OnGround bool

	// Despawn is set by the AI (e.g. creeper post-explosion) to flag the
	// mob for removal at the end of the current tick. The world tick
	// collects these via PendingDespawns() and calls Remove on each.
	Despawn bool

	// --- per-mob AI state (unexported; reset on Tick, never serialised) ---
	state          AIState
	walkTicks      int      // >0: walking forward for this many ticks
	cooldownTicks  int      // generic per-mob cooldown (used for attack reuse)
	target         [16]byte // current target player UUID; zero if none
	hurtBy         [16]byte // last attacker; cleared after panic ends
	panicTicks     int      // remaining ticks in StateFlee
	fuseTicks      int      // creeper-only: counts down StateFuse → boom
	wasInRange     bool     // creeper-only: was target in range last tick (resets fuse if so)
	jumpCooldown   int      // 2 ticks after a jump so we don't double-bounce
	vy             float64  // vertical velocity (jumping / falling) — M0.2
	ambientCD      int      // UX (M0.7 sound): ticks until next ambient sound candidate

	// M0.3: cached A* path toward the current target. Replanned every
	// 20 ticks or when the target moves >2 cells. `pathIdx` is the
	// index of the next waypoint to walk toward.
	path     Path
	pathIdx  int
	pathTick int // world tick counter at last replan; ticks == 0 means no path

	// M0.4: skeleton bow-draw. Set to 20 when entering StateShoot and
	// decremented each tick. The mob fires its arrow on the tick the
	// counter hits 0 and goes back to StatePursue.
	drawTicks int

	// M0.6: fire / HP. HP is the per-mob health pool; 0 = dead.
	// The world layer calls Hurt() to set hurtBy/state, HurtFire() to
	// apply 1 HP/tick of sun burn, and HurtDirect() to apply direct
	// melee damage. FireTicks is decremented each tick; while > 0
	// the bridge broadcasts the fire overlay.
	HP        float32
	MaxHP     float32
	FireTicks int

	// M1: Size. Slime / MagmaCube only — the cube's hitbox
	// (radius = Size × 0.5 + 0.1, height = Size × 0.5 + 0.1). At
	// 0 or 1 the mob is "small", at 4 it is the "large" boss-tier
	// cube. Used by:
	//   - SplitsOnDeath: spawn 2 children of Size-1
	//   - Movement="hop": larger sizes jump on a longer cadence
	//   - Bridge: send the right AABB + scale to the client
	// Other mobs leave this at 0; the bridge uses def.Scale if
	// present, otherwise 1.0.
	Size int

	// M7: knockback velocity in blocks/tick, set by
	// HurtDirect when the swing lands. The AI integrator
	// (moveMob) consumes KnockbackVX/VY/VZ once, zeros the
	// components, and adds them to the per-tick delta — the
	// mob visibly staggers away from the attacker for ~5-10
	// ticks. Vanilla knockback is 0.4 horizontal + 0.4
	// vertical, scaled by Knockback level (axe = +0.3,
	// sprint = +0.5). M7 wires the v1 vanilla math; the
	// strength/sprint scaling lands in M7.x.
	KnockbackVX, KnockbackVY, KnockbackVZ float64

	// M0.8: despawn grace. noPlayerTicks counts consecutive ticks
	// that no player was within 128 blocks. When it hits 600 (30 s at
	// 20 Hz) the mob sets Despawn=true and the world tick
	// removes it. Resets to 0 whenever a player is in range.
	// PersistenceRequired mobs (named-tagged) skip this check.
	noPlayerTicks int

	// M1: PersistenceRequired is set when a mob is named with a name
	// tag (or otherwise marked persistent). Persistent mobs never
	// despawn regardless of player distance or time.
	PersistenceRequired bool

	// M7.10: baby flag. Vanilla zombies have a 5% chance of
	// spawning as a baby (smaller hitbox, 1.5× chase speed).
	// Set on spawn by the world layer (Spawn() helper) using
	// the per-mob BabyChance. AI uses Baby for the speed
	// multiplier and the bridge uses it for the scale
	// override.
	Baby bool

	// M7.10: stuckTicks counts consecutive ticks the mob
	// tried to step into a solid block. When it hits 3 the
	// mob turns 90° and resets — this is the vanilla
	// "stuck" feel where a mob bumping into a wall doesn't
	// keep grinding on it.
	stuckTicks int
}

// Store holds active mobs and notifies listeners on spawn/despawn/move.
type Store struct {
	mu      sync.RWMutex
	mobs    map[int64]*Mob
	nextID  atomic.Int64
	onSpawn []func(Mob)
	onDesp  []func(id int64)
	onMove  []func(Mob)
	// onDeath fires once per mob when HP drops to <= 0. Receives the
	// mob snapshot (type, x, y, z) so the world layer can spawn XP
	// orbs and other death-time drops. Listeners run outside the
	// store lock to avoid re-entrant deadlock.
	onDeath []func(Mob)
}

// New returns an empty mob store. Mob entity ids start at 1<<22 to stay clear of
// player ids (Java small ints / Bedrock 100000+) and drop ids (1<<20).
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

// OnMove registers a callback invoked when a mob's position or head yaw
// changes during an AI tick. Bridges may emit their position/rotation packets
// from here.
func (s *Store) OnMove(fn func(Mob)) {
	s.mu.Lock()
	s.onMove = append(s.onMove, fn)
	s.mu.Unlock()
}

// OnDeath (M7) registers a callback fired when a mob's HP drops to 0
// from a damage event (HurtDirect / HurtDirectWithKnockback). Listeners
// get the mob snapshot at the moment of death (Type, X/Y/Z) so they
// can spawn XP orbs, item drops, and other death-time effects without
// pulling in cross-package imports.
func (s *Store) OnDeath(fn func(Mob)) {
	s.mu.Lock()
	s.onDeath = append(s.onDeath, fn)
	s.mu.Unlock()
}

// Spawn adds a mob of the given namespaced type and notifies listeners.
func (s *Store) Spawn(mobType string, x, y, z float64) Mob {
	return s.SpawnAtSize(mobType, x, y, z, 0)
}

// SpawnAtSize is Spawn with an explicit Size (M1 — slime / magma cube
// splits). Size 0 means "use def.Size". The split-on-death path uses
// this directly: it spawns children at the parent's Size-1.
func (s *Store) SpawnAtSize(mobType string, x, y, z float64, size int) Mob {
	def := defFor(mobType)
	if size == 0 {
		size = def.Size
	}
	m := Mob{
		EntityID: s.nextID.Add(1),
		UUID:     uuid.New(),
		Type:     mobType,
		X:        x, Y: y, Z: z,
		Yaw:     rand.Float64()*360 - 180,
		HeadYaw: 0,
		OnGround: true,
		// M0.6: HP. The vanilla values are per-mob-attribute; for v1
		// we use 20 (zombie) / 20 (skeleton) / 20 (creeper) / 10
		// (passive) — the actual vanilla number is the same. The
		// MaxHP is stored so the bridge can compute the health
		// bar without a lookup.
		HP:    def.MaxHP,
		MaxHP: def.MaxHP,
		Size:  size,
		state: StateIdle,
	}
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

// All returns a snapshot of every active mob (for catch-up spawning on join).
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
// Mob if not found. Used by M1's split/drop callbacks to read the
// despawning mob's last position, size, and type.
func (s *Store) Get(id int64) Mob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.mobs[id]
	if !ok {
		return Mob{}
	}
	return *m
}

// PendingDespawns returns the EntityIDs of every mob whose Despawn flag was
// set during the last tick. The caller (world tick) is expected to call
// Remove(id) for each one to actually drop them from the store and broadcast
// the OnDespawn event.
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

// Hurt records an attack on this mob from the given player. The world
// tick (or a future combat path) calls this whenever a player deals
// damage; the AI uses it to set hurtBy and trigger StateFlee on passives or
// StatePursue-on-the-attacker on hostiles. The mob is looked up by EntityID
// (the server's authoritative id, not the player entity id).
func (s *Store) Hurt(mobID int64, attacker [16]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.mobs[mobID]
	if !ok {
		return
	}
	m.hurtBy = attacker
	// Mob took damage from a player. If hostile and has no current target,
	// lock on to the attacker. If passive, enter panic.
	def := defFor(m.Type)
	if def.IsHostile {
		var zero [16]byte
		if m.target == zero {
			m.target = attacker
			m.state = StatePursue
		}
	} else {
		// M7.10: vanilla passive-mob panic duration is
		// 2-4 s (40-80 ticks). The wider 60-100 range we
		// used pre-M7.10 looked like a stun, not a
		// frightened dash.
		m.panicTicks = 40 + rand.Intn(40) // 2-4s at 20 Hz
		m.state = StateFlee
	}
}

// HurtFire applies fire damage from the M0.6 sun-burn path. Sets
// FireTicks to 60 (3 s of visible fire) so the bridge broadcasts the
// flame overlay; subtracts HP and flags Despawn on zero HP. Sun-burn
// damage is 0.05 HP/tick = 1 HP/s (vanilla).
//
// M1: FireImmune mobs (wither skeleton, blaze, magma cube) are
// immune to all fire damage. The sun-burn path passes damage=0
// effectively, so we short-circuit and return. The fire overlay
// is also skipped (no flame particles on the mob).
func (s *Store) HurtFire(mobID int64, damage float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.mobs[mobID]
	if !ok {
		return
	}
	def := defFor(m.Type)
	if def.FireImmune {
		// Vanilla: wither skeleton, blaze, magma cube, ender
		// dragon, etc. all take 0 damage from fire. We
		// additionally skip the FireTicks overlay since
		// the visual would be confusing (mob is on fire
		// but not taking damage).
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

// HurtDirect applies direct damage from a player melee swing. (Not
// wired yet — the player → mob damage path is a future M0.9 step —
// but the function is here so the world tick can use it without
// re-locking.)
func (s *Store) HurtDirect(mobID int64, attacker [16]byte, damage float32) {
	s.HurtDirectWithKnockback(mobID, attacker, damage, 0, 0, 0)
}

// HurtDirectWithKnockback (M7) applies direct damage AND sets a
// knockback velocity on the mob. dirX/dirZ is the horizontal
// direction from the mob toward the attacker (NOT the push
// direction; the integrator flips the sign when applying). kb
// is the vanilla knockback strength (≈0.4 for a bare hit, 0.4
// + 0.5 per Knockback level). When kb == 0 or dirX/dirZ == 0
// the knockback is a no-op (e.g. the attacker is at the same
// position as the mob).
//
// The mob's HP, hurtBy, target-lock, and Despawn flag are all
// updated under the same store lock; the bridge listeners
// observe the change on the next OnMove broadcast. M7: when
// the damage kills the mob (HP drops to <= 0) the death
// listeners fire AFTER the lock is released, with a snapshot
// of the mob at the moment of death so listeners can spawn
// XP orbs and other death-time effects.
func (s *Store) HurtDirectWithKnockback(mobID int64, attacker [16]byte, damage float32, dirX, dirZ, kb float64) {
	var deathSnap *Mob
	var deathCbs []func(Mob)
	s.mu.Lock()
	m, ok := s.mobs[mobID]
	if !ok {
		s.mu.Unlock()
		return
	}
	m.hurtBy = attacker
	def := defFor(m.Type)
	if def.IsHostile {
		var zero [16]byte
		if m.target == zero {
			m.target = attacker
			m.state = StatePursue
		}
	}
	m.HP -= damage
	if m.HP <= 0 {
		m.Despawn = true
		// Snapshot the death state so we can fire listeners
		// outside the lock. The snapshot is read-only — the
		// listener can't mutate the mob (the store will
		// Remove it via PendingDespawns on the next tick).
		snap := *m
		deathSnap = &snap
		deathCbs = append(deathCbs, s.onDeath...)
	}
	if kb > 0 && !def.NoKnockback {
		d := math.Hypot(dirX, dirZ)
		if d > 0 {
			// Push is OPPOSITE the attacker direction (mob
			// away from attacker). Vanilla adds the on-
			// ground Y bump (0.4) so the mob visibly
			// hops when hit. The integrator consumes
			// these once and zeros them.
			//
			// M7.10: NoKnockback (slime / magma cube) skip
			// the impulse entirely. Vanilla 1.20 slime has
			// knockback_resistance=1.0 — a hit produces no
			// visible stagger.
			m.KnockbackVX = -dirX / d * kb
			m.KnockbackVZ = -dirZ / d * kb
			if m.OnGround {
				m.KnockbackVY = 0.4
			} else {
				m.KnockbackVY = 0
			}
		}
	}
	s.mu.Unlock()
	if deathSnap != nil {
		for _, cb := range deathCbs {
			cb(*deathSnap)
		}
	}
}

// TickFire decrements every mob's FireTicks. The bridge mob sync
// broadcasts an OnMove update whenever a mob's state changes; the
// fire-overlay is read by the bridge on each OnMove. Called once
// per world tick from world/tick.go (Phase 4d) so it runs after
// the AI tick.
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

// --- M0.3 path helpers ----------------------------------------------------

// hasPath reports whether m currently has a non-empty A* path it is
// walking along. A nil/empty path means "go directly toward the target"
// in moveMob.
func (m *Mob) hasPath() bool {
	return len(m.path) > 0 && m.pathIdx < len(m.path)
}

// pathGoalStale is true when the path's last node is more than 2 cells
// from the requested goal cell. moveMob calls this every tick during
// StatePursue; when stale, the path is replanned.
func (m *Mob) pathGoalStale(goal PathNode) bool {
	if len(m.path) == 0 {
		return true
	}
	last := m.path[len(m.path)-1]
	return absI(last.X-goal.X) > 2 || absI(last.Z-goal.Z) > 2
}

// pathDirection returns the (dx, dz) unit vector from the mob's cell to
// the next waypoint, plus the next waypoint's Y. y is the next cell's
// floor (moveMob jumps if y > currentY+0).
//
// ok=false means the path is exhausted (or empty); the caller should
// fall back to direct-line chase or wander.
func (m *Mob) pathDirection(mx, mz int) (dx, dz float64, y int, ok bool) {
	if !m.hasPath() {
		return 0, 0, 0, false
	}
	np := m.path[m.pathIdx]
	dx, dz = float64(np.X-mx), float64(np.Z-mz)
	if d := math.Hypot(dx, dz); d > 0 {
		dx, dz = dx/d, dz/d
	}
	// Pop this waypoint if the mob is already on the cell.
	if mx == np.X && mz == np.Z {
		m.pathIdx++
	}
	return dx, dz, np.Y, true
}

// replanPath runs A* from from to to and stores the result on m.
// moveMob drives the 20-tick refresh via m.pathTick.
func (m *Mob) replanPath(from, to PathNode, ctx AIContext) {
	wq := func(x, y, z int) bool {
		// Walkable: the feet cell is air AND the head cell is air.
		// The pathfinder's neighbors() function will also try
		// step-up / step-down from here, so we don't gate on
		// "below must be solid" — A* will find the right surface.
		if ctx.SolidAt(x, y, z) {
			return false
		}
		if ctx.SolidAt(x, y+1, z) {
			return false
		}
		return true
	}
	res := PathFind(from, to, wq)
	if res.Found {
		m.path = res.Nodes
		m.pathIdx = 0
		m.pathTick = 20
	} else {
		m.path = nil
		m.pathIdx = 0
		m.pathTick = 0
	}
}

func absI(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
