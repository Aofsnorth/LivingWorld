package player

import (
	"math"
	"time"

	"livingworld/internal/combat"
	"livingworld/internal/shared/constants/gameplay"

	"github.com/google/uuid"
)

// Player-push tuning. Values are in blocks; velocity is blocks/tick.
// Constants moved to gameplay.physics package
const (
	pushTickHz     = gameplay.PushTickHz
	pushRadius     = gameplay.PushRadius
	pushVertical   = gameplay.PushVertical
	pushStrength   = gameplay.PushStrength
	pushMaxPerTick = gameplay.PushMaxPerTick
)

// StartPushLoop runs the cross-edition player-push loop until Close. Players of
// both editions push each other apart (Java's player-vs-player push is
// server-authoritative, so it must be driven here rather than client-side).
func (m *Manager) StartPushLoop() {
	go func() {
		ticker := time.NewTicker(time.Second / pushTickHz)
		defer ticker.Stop()
		for range ticker.C {
			m.pushTick()
		}
	}()
}

func (m *Manager) pushTick() {
	players := m.GetAllPlayers()
	for i := 0; i < len(players); i++ {
		for j := i + 1; j < len(players); j++ {
			a, b := players[i], players[j]
			if a.EntityRuntimeID == 0 || b.EntityRuntimeID == 0 || a.Health <= 0 || b.Health <= 0 || a.Gamemode == 3 || b.Gamemode == 3 {
				continue
			}
			if math.Abs(b.Position.Y-a.Position.Y) >= pushVertical {
				continue
			}
			dx := b.Position.X - a.Position.X
			dz := b.Position.Z - a.Position.Z
			distSq := dx*dx + dz*dz
			if distSq >= pushRadius*pushRadius || distSq < 1e-6 {
				continue
			}
			dist := math.Sqrt(distSq)
			f := pushStrength * (1.0 - dist/pushRadius)
			if f <= 0 {
				continue
			}
			vx := clampF((dx/dist)*f, -pushMaxPerTick, pushMaxPerTick)
			vz := clampF((dz/dist)*f, -pushMaxPerTick, pushMaxPerTick)
			// Only push a grounded player. The per-edition Push SETS the client's
			// whole velocity vector (Java ClientboundSetEntityMotion and Bedrock
			// SetActorMotion both replace, not add â€” unlike vanilla Entity.push
			// which adds a purely horizontal impulse). Pushing an airborne player
			// would therefore zero its vertical velocity every tick and cancel its
			// fall: a player descending onto another stalls ~1 block above them
			// ("turun nyangkut di ranting"). Skipping airborne players keeps the
			// fall intact; once landed (vyâ‰ˆ0) the horizontal push slides them apart.
			if a.OnGround {
				m.push(a.UUID, -vx, 0, -vz)
			}
			if b.OnGround {
				m.push(b.UUID, vx, 0, vz)
			}
		}
	}
}

// MeleeAttack applies 1:1 vanilla-style melee knockback and weapon damage to
// the player whose entity runtime ID is targetEntityID, knocking them away
// from the attacker while respecting 10-tick post-hit invulnerability frames
// and sprint-knockback bonus.
func (m *Manager) MeleeAttack(attackerUUID uuid.UUID, targetEntityID int32) {
	m.mu.Lock()
	attacker := m.players[attackerUUID]
	var target *Player
	for _, p := range m.players {
		if int32(p.EntityRuntimeID) == targetEntityID {
			target = p
			break
		}
	}
	if attacker == nil || target == nil || target.UUID == attackerUUID || attacker.Health <= 0 || target.Health <= 0 || attacker.Gamemode == 3 || target.Creative || target.Gamemode == 1 || target.Gamemode == 3 {
		m.mu.Unlock()
		return
	}
	if target.IFrames > 0 {
		m.mu.Unlock()
		return
	}
	target.IFrames = 10

	dmg := float32(meleeDamage)
	if attacker.Inventory != nil {
		if held := attacker.Inventory.GetHeldItem(); held != nil && held.ID != 0 {
			dmg = float32(combat.SwordDamage(held.ID))
		}
	}
	isCrit := attacker.CanCrit()
	if isCrit {
		dmg *= 1.5
		attacker.FallDistance = 0
		attacker.LastFallDistance = 0
		attacker.LastAirborneAt = time.Time{}
	}

	dx := target.Position.X - attacker.Position.X
	dz := target.Position.Z - attacker.Position.Z
	d := math.Hypot(dx, dz)
	if d < 1e-4 {
		yaw := float64(attacker.Rotation.Yaw) * math.Pi / 180
		dx, dz, d = -math.Sin(yaw), math.Cos(yaw), 1
	}
	kb := 0.4
	wasSprinting := attacker.Sprinting
	if wasSprinting {
		kb += 0.5
	}
	vy := 0.0
	if target.OnGround {
		vy = 0.4
	}
	targetUUID := target.UUID
	prevTargetHP := target.Health
	exhaustionFn := m.exhaustionFn
	m.mu.Unlock()

	if exhaustionFn != nil {
		exhaustionFn(attackerUUID, 0.1) // vanilla melee attack exhaustion
	}
	if wasSprinting {
		m.UpdateSprinting(attackerUUID, false)
	}
	m.push(targetUUID, dx/d*kb, vy, dz/d*kb)
	m.hurt(targetUUID, dmg) // target's own health + red flash

	m.mu.Lock()
	var targetSnap PlayerSnapshot
	if t := m.players[targetUUID]; t != nil {
		if t.Health == prevTargetHP {
			t.Health -= dmg
			if t.Health < 0 {
				t.Health = 0
			}
			if t.Health <= 0 {
				t.OnFire = false
				t.FireTicks = 0
				t.LavaTickCooldown = 0
				t.Sprinting = false
				t.Swimming = false
				t.Sneaking = false
			}
		}
		targetSnap = t.Snapshot()
	}
	m.mu.Unlock()
	if targetSnap.UUID != (uuid.UUID{}) {
		m.publish(Event{Type: EventHurt, Player: targetSnap}) // hurt flash (or death) on all viewers
		if isCrit {
			m.publish(Event{Type: EventCrit, Player: targetSnap})
		}
	}
}

// meleeDamage is the bare-hand attack damage (vanilla: 1 point = half a heart).
const meleeDamage = 1.0

// hurt routes melee damage to a player's live session (no-op if absent).
func (m *Manager) hurt(id uuid.UUID, amount float32) {
	m.ctrlMu.RLock()
	c := m.controllers[id]
	m.ctrlMu.RUnlock()
	if c != nil {
		c.Hurt(amount)
	}
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
