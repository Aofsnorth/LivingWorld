package world

import (
	"livingworld/internal/mobs"
)

// OnExplosion registers a listener for creeper explosions. The world tick
// publishes the result after applying damage and knockback; bridges
// translate it into the per-edition explosion event (Java
// ClientboundGameExplosion, Bedrock LevelEvent{Explode}).
func (m *Manager) OnExplosion(fn mobs.ExplosionListener) {
	m.explosionMu.Lock()
	m.explosionHooks = append(m.explosionHooks, fn)
	m.explosionMu.Unlock()
}

// PublishExplosion fires every registered listener. Called from the
// aiExplode callback in the world tick.
func (m *Manager) PublishExplosion(result mobs.ExplosionResult) {
	m.explosionMu.RLock()
	hooks := append([]mobs.ExplosionListener{}, m.explosionHooks...)
	m.explosionMu.RUnlock()
	for _, h := range hooks {
		h(result)
	}
}

// OnMobSound registers a listener that receives a SoundEmit list
// each tick. The world tick pre-computes the list in Phase 4 (after
// AI + despawn) and PublishMobSounds fans it out to every bridge.
func (m *Manager) OnMobSound(fn func(emits []mobs.SoundEmit)) {
	m.mobSoundMu.Lock()
	m.mobSoundHooks = append(m.mobSoundHooks, fn)
	m.mobSoundMu.Unlock()
}

// PublishMobSounds fans the SoundEmit list out to every listener.
// Listeners are expected to translate each emit into the per-edition
// sound packet and broadcast it. Empty lists are still passed (so
// listeners can short-circuit cheaply) — the world tick gates the
// call on len(emits) > 0 already.
func (m *Manager) PublishMobSounds(emits []mobs.SoundEmit) {
	m.mobSoundMu.RLock()
	hooks := append([]func(emits []mobs.SoundEmit){}, m.mobSoundHooks...)
	m.mobSoundMu.RUnlock()
	for _, h := range hooks {
		h(emits)
	}
}

// SetMobAICallbacks wires the side-effect hooks the AI fires when a hostile
// mob lands a melee hit, a skeleton fires, or a creeper explodes. Each hook
// may be nil — the AI degrades gracefully (a skeleton with no arrow hook
// still draws, but doesn't spawn the projectile).
//
// M1: extended with hitEffect, throw, shootProjectile, and waterAt
// hooks. Callers that don't need them can pass nil.
func (m *Manager) SetMobAICallbacks(
	melee func([16]byte, int64, float32),
	arrow func(int64, float64, float64, float64, float64, float64),
	explode func(int64, float64, float64, float64, float64),
	projectileHit func(int64, [16]byte),
	fireDamage func(int64, float32),
	sound func([]mobs.SoundEmit),
	hitEffect func([16]byte, int64, mobs.HitEffect),
	throw func([16]byte, int64, float32),
	shootProjectile func(int64, float64, float64, float64, float64, float64, string),
	waterAt func(int, int, int) bool,
) {
	m.locatorMu.Lock()
	m.aiMeleeAttack = melee
	m.aiShootArrow = arrow
	m.aiExplode = explode
	m.aiProjectileHit = projectileHit
	m.aiFireDamage = fireDamage
	m.aiSound = sound
	m.aiHitEffect = hitEffect
	m.aiThrow = throw
	m.aiShootProjectile = shootProjectile
	m.aiWaterAt = waterAt
	m.locatorMu.Unlock()
}

// SetMobSplitCallback registers a hook called from the world tick
// Phase 4 cleanup when a despawning mob has def.SplitsOnDeath. The
// closure spawns 2 children at the parent's last position with
// Size-1. May be nil (no splits).
func (m *Manager) SetMobSplitCallback(fn func(w *World, mobID int64)) {
	m.locatorMu.Lock()
	m.mobSpawnSplits = fn
	m.locatorMu.Unlock()
}

// SetMobDropCallback registers a hook called from the world tick
// Phase 4 cleanup when a despawning mob drops loot. The closure
// rolls the drops and calls drops.Store.Spawn for each. May be
// nil.
func (m *Manager) SetMobDropCallback(fn func(w *World, mobID int64)) {
	m.locatorMu.Lock()
	m.mobSpawnDrops = fn
	m.locatorMu.Unlock()
}
