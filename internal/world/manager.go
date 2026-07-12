package world

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"livingworld/internal/drops"
	"livingworld/internal/mobs"
)

const (
	legacyDefaultWorldStorageName = "world"
	defaultDimensionStorageName   = "dimensions"
)

type Manager struct {
	mu           sync.RWMutex
	worlds       map[string]*World
	defaultWorld *World
	blockEvents  *BlockEventBus
	lightEvents  *LightEventBus
	drops        *drops.Store
	mobs         *mobs.Store
	dropRNG      *rand.Rand
	dropMu       sync.Mutex // guards dropRNG (math/rand is not concurrency-safe)
	autosaveStop chan struct{}
	timeStop     chan struct{}
	weatherStop  chan struct{}

	// tickStop / tickAutosaver / tickAdvanceTime / tickRNG are the
	// Phase-4a unified-tick fields. The legacy StartTimeLoop /
	// StartAutosave fields above are kept for the deprecated wrapper
	// paths; new callers should use startTickLoop.
	tickStop        chan struct{}
	tickAutosaver   time.Duration
	tickAdvanceTime bool
	tickRNG         *rand.Rand
	tickCounter     atomic.Uint64 // monotonic 20 Hz tick count, used to throttle the mob-spawn director

	// pickupCallback is called when a player picks up an item, for Bedrock inventory sync.
	pickupCallback func(playerUUID [16]byte, dropEntityID int64, playerEntityID uint64)
	pickupMu       sync.RWMutex

	// crackManager tracks active block-breaking states for cross-edition crack animation.
	crackManager *CrackManager

	// effectEvents carries cross-edition action effects (crack overlay, break
	// particles+sound) — see effects.go.
	effectEvents *WorldEffectBus

	weatherMu        sync.RWMutex
	weatherCallbacks []func(raining, thundering bool)

	// difficulty gates the mob-spawn director (peaceful suppresses hostiles). Set
	// by StartMobAI; read under m.mu by the director tick.
	difficulty string

	// spawnMode selects the JE ("java") vs BE ("bedrock") mob spawn/despawn
	// model used by the director. Set via SetSpawnMode at bootstrap; read
	// under m.mu by spawnTick. Empty defaults to "java".
	spawnMode string

	// spawnMobsEnabled gates the natural mob-spawn director. When false, spawnTick
	// is skipped entirely — no new mobs appear, but existing ones keep their AI.
	// Defaults to true; set via SetSpawnMobsEnabled from the server bootstrap.
	spawnMobsEnabled bool

	// playerLocator returns the live world positions of connected players. The
	// world package can't import player (cycle), so the server bootstrap wires the
	// player.Manager in via SetPlayerLocator. Used by the mob-spawn director as the
	// anchor set for candidate columns.
	locatorMu     sync.RWMutex
	playerLocator func() []Position

	// aiPlayerList is a parallel hook used by the mob AI tick: it returns the
	// per-player detection surface (uuid, position, sneaking, head, gamemode)
	// the AI needs to pick targets. May be nil — in which case the AI skips
	// detection and just wanders.
	aiPlayerList func() []mobs.PlayerTarget

	// aiHeldItem returns the namespaced item id of the given player's
	// main hand. Used by passive mob food-attraction AI. May be nil —
	// the AI degrades to "no food following". Wired from the server
	// bootstrap with a closure that reads the player's inventory.
	aiHeldItem func(playerUUID [16]byte) string

	// aiMeleeAttack / aiShootArrow / aiExplode are the side-effect callbacks
	// the AI fires when a hostile mob lands a hit, a skeleton fires an arrow,
	// or a creeper explodes. Wired from the server bootstrap with closures
	// that resolve the per-edition damage / projectile / explosion pipeline.
	aiMeleeAttack   func(targetUUID [16]byte, attackerID int64, damage float32)
	aiShootArrow    func(shooterID int64, x, y, z, yaw, pitch float64)
	aiExplode       func(attackerID int64, x, y, z, power float64)
	aiProjectileHit func(arrowID int64, targetUUID [16]byte)
	aiFireDamage    func(mobID int64, damage float32)
	aiSound         func(emits []mobs.SoundEmit)
	// M6: aiTickEffects is called once per 20 Hz tick from Phase 4e.
	// The player manager iterates its effect bag inside the closure:
	// it applies per-tick damage (poison/wither), decrements
	// TicksLeft, and publishes EffectStatusRemove for any effect
	// that just hit 0. The world package doesn't import player
	// (cycle), so the server bootstrap wires the closure in
	// server.go's worlds.SetMobAICallbacks.
	aiTickEffects func()
	// M7: aiTickIFrames decrements every connected player's
	// IFrames counter once per 20 Hz tick. Same world-package
	// indirection as aiTickEffects: the world tick is edition-
	// agnostic, the player manager owns the IFrames field.
	aiTickIFrames func()
	// M1: aiHitEffect is fired by the AI when a melee swing applies
	// a status effect (e.g. husk → hunger, cave spider → poison,
	// wither skeleton → wither). Bridges translate the effect into
	// per-edition damage / status packets.
	aiHitEffect func(targetUUID [16]byte, attackerID int64, effect mobs.HitEffect)
	// M1: aiThrow is fired when an iron golem picks up a player and
	// launches them upward. The bridge applies an upward velocity
	// to the player and queues the throw-damage to land on impact.
	aiThrow func(targetUUID [16]byte, attackerID int64, damage float32)
	// M1: aiShootProjectile is the unified ranged-fire hook. The
	// projectileType string tells the bridge which kind of
	// projectile to spawn ("arrow", "small_fireball", etc).
	aiShootProjectile func(shooterID int64, x, y, z, yaw, pitch float64, projectileType string)
	// M1: aiWaterAt is the AI's water-cell probe. It returns true
	// if the cell at (x, y, z) is water or a waterlogged block.
	// Used by enderman for water-damage. May be nil (the AI
	// degrades gracefully).
	aiWaterAt func(x, y, z int) bool

	// M1: mobSpawnSplits is fired from the world tick Phase 4
	// cleanup when a despawning mob has def.SplitsOnDeath. The
	// closure spawns 2 children at the parent's last position
	// with Size-1. May be nil (no splits).
	mobSpawnSplits func(w *World, mobID int64)
	// M1: mobSpawnDrops fires when a despawning mob drops loot
	// (e.g. zombie rotten flesh, skeleton bone, wither skeleton
	// skull, slime slimeball). The closure rolls the drops and
	// calls drops.Store.Spawn for each. May be nil.
	mobSpawnDrops func(w *World, mobID int64)

	// projectiles is the shared skeleton-arrow store. Bridges subscribe to
	// OnSpawn/OnDespawn; the world tick drives the integrator (Phase 4c).
	projectiles *mobs.ProjectileStore

	// explosion listeners: bridges register here so the world tick can
	// broadcast an ExplosionResult to all clients on both editions.
	explosionMu    sync.RWMutex
	explosionHooks []mobs.ExplosionListener

	// mobSound listeners: bridges register here so the world tick can
	// fan out SoundEmit (entityID + sound id + volume/pitch) to all
	// connected clients. Java and Bedrock each translate into their
	// per-edition packet.
	mobSoundMu    sync.RWMutex
	mobSoundHooks []func(emits []mobs.SoundEmit)

	// hunger tracks per-player food/saturation/exhaustion mechanics.
	hunger *HungerTracker
	// scheduledTicks is the block-update priority queue (redstone,
	// fluids, gravity neighbor notifies, etc.).
	scheduledTicks *ScheduledTickSystem
}

func NewManager() *Manager {
	m := &Manager{
		worlds:           make(map[string]*World),
		blockEvents:      NewBlockEventBus(),
		lightEvents:      NewLightEventBus(),
		drops:            drops.New(),
		mobs:             mobs.New(),
		projectiles:      mobs.NewProjectileStore(),
		dropRNG:          rand.New(rand.NewSource(1)), // deterministic; drops aren't security-sensitive
		crackManager:     NewCrackManager(),
		effectEvents:     NewWorldEffectBus(),
		spawnMobsEnabled: true,
		hunger:           NewHungerTracker(),
		scheduledTicks:   NewScheduledTickSystem(),
	}
	m.defaultWorld = NewWorld("world")
	m.worlds["world"] = m.defaultWorld
	// M7: glue mob-death to XP-orb spawning. The mobs package
	// doesn't import drops (avoids a cycle), so the world
	// layer registers a callback that maps the death
	// snapshot's mob type to the vanilla XP yield and asks
	// the drops store to spawn one orb per point.
	m.mobs.OnDeath(func(snap mobs.Mob) {
		amount := mobs.XPRewardFor(snap.Type)
		if amount <= 0 {
			return
		}
		m.drops.SpawnXP(amount, snap.X, snap.Y+0.5, snap.Z)
	})
	return m
}

// Drops returns the shared item-drop store. Each protocol bridge subscribes to
// it (OnSpawn/OnDespawn) to render and pick up dropped items.
func (m *Manager) Drops() *drops.Store { return m.drops }

// SetDifficulty updates the difficulty used by the mob-spawn
// director (peaceful suppresses hostiles). M2: replaces the
// "set on StartMobAI" approach so the value can be flipped at
// runtime via /difficulty without restarting the tick loop.
func (m *Manager) SetDifficulty(difficulty string) {
	m.mu.Lock()
	m.difficulty = difficulty
	m.mu.Unlock()
}

// Difficulty returns the current difficulty. M2: used by
// plugins and the /difficulty command.
func (m *Manager) Difficulty() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.difficulty
}

// SetSpawnMode selects the JE ("java") vs BE ("bedrock") mob spawn/despawn
// model used by the natural-spawn director. Anything other than "bedrock" is
// treated as "java" (the default).
func (m *Manager) SetSpawnMode(mode string) {
	m.mu.Lock()
	m.spawnMode = mode
	m.mu.Unlock()
}

// SpawnMode returns the current spawn mode ("java" or "bedrock").
func (m *Manager) SpawnMode() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.spawnMode == spawnModeBedrock {
		return spawnModeBedrock
	}
	return spawnModeJava
}

// SetSpawnMobsEnabled toggles the natural mob-spawn director.
func (m *Manager) SetSpawnMobsEnabled(enabled bool) {
	m.mu.Lock()
	m.spawnMobsEnabled = enabled
	m.mu.Unlock()
}

// SpawnMobsEnabled returns true if the natural spawn director is enabled.
func (m *Manager) SpawnMobsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.spawnMobsEnabled
}

// Mobs returns the shared mob store. Each protocol bridge subscribes
// (OnSpawn/OnDespawn) to render and remove mobs cross-edition.
func (m *Manager) Mobs() *mobs.Store { return m.mobs }

// Projectiles returns the shared skeleton-arrow store. Bridges subscribe
// to OnSpawn/OnDespawn to render the projectile (Java SpawnEntity +
// EntityMetadata, Bedrock AddActor).
func (m *Manager) Projectiles() *mobs.ProjectileStore { return m.projectiles }

// Hunger returns the per-player hunger/exhaustion tracker. Bridges
// register players on join and add exhaustion from sprint/jump/attack.
func (m *Manager) Hunger() *HungerTracker { return m.hunger }

// ScheduledTicks returns the block-update priority queue. Bridges
// schedule neighbor notifies from block changes.
func (m *Manager) ScheduledTicks() *ScheduledTickSystem { return m.scheduledTicks }

func (m *Manager) GetWorld(name string) *World {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.worlds[name]
}

func (m *Manager) GetDefaultWorld() *World {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultWorld
}

func (m *Manager) AddWorld(name string, w *World) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.worlds[name] = w
}

func (m *Manager) RemoveWorld(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.worlds, name)
}

// CrackManager returns the shared crack state tracker for cross-edition animation.
func (m *Manager) CrackManager() *CrackManager {
	return m.crackManager
}

// SetPlayerLocator registers a function returning the live world positions of
// connected players (wired from the player.Manager by the server bootstrap).
func (m *Manager) SetPlayerLocator(fn func() []Position) {
	m.locatorMu.Lock()
	m.playerLocator = fn
	m.locatorMu.Unlock()
}

// playerAnchors returns the current player positions, or nil if no locator is set.
func (m *Manager) playerAnchors() []Position {
	m.locatorMu.RLock()
	fn := m.playerLocator
	m.locatorMu.RUnlock()
	if fn == nil {
		return nil
	}
	return fn()
}

// SetMobAIPlayerList registers the function the mob AI uses to read the
// per-tick player list. Wired from server bootstrap; nil disables detection.
func (m *Manager) SetMobAIPlayerList(fn func() []mobs.PlayerTarget) {
	m.locatorMu.Lock()
	m.aiPlayerList = fn
	m.locatorMu.Unlock()
}

// SetMobAIHeldItem registers the function the AI uses to query a player's
// held item. Wired from server bootstrap; nil disables food-attraction AI.
func (m *Manager) SetMobAIHeldItem(fn func(playerUUID [16]byte) string) {
	m.locatorMu.Lock()
	m.aiHeldItem = fn
	m.locatorMu.Unlock()
}

func (m *Manager) GetAllWorlds() []*World {
	m.mu.RLock()
	defer m.mu.RUnlock()
	worlds := make([]*World, 0, len(m.worlds))
	for _, w := range m.worlds {
		worlds = append(worlds, w)
	}
	return worlds
}
