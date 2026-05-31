package player

import (
	"math"
	"time"

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
			if a.EntityRuntimeID == 0 || b.EntityRuntimeID == 0 {
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

// MeleeAttack applies vanilla-style melee knockback to the player whose entity
// runtime ID is targetEntityID, knocking them away from the attacker. Health
// damage and the hurt flash are not synced cross-edition yet; knockback is the
// visible hit feedback (the same server-authoritative path player-push uses).
func (m *Manager) MeleeAttack(attackerUUID uuid.UUID, targetEntityID int32) {
	m.mu.RLock()
	attacker := m.players[attackerUUID]
	var target *Player
	for _, p := range m.players {
		if int32(p.EntityRuntimeID) == targetEntityID {
			target = p
			break
		}
	}
	m.mu.RUnlock()
	if attacker == nil || target == nil || target.UUID == attackerUUID {
		return
	}
	dx := target.Position.X - attacker.Position.X
	dz := target.Position.Z - attacker.Position.Z
	d := math.Hypot(dx, dz)
	if d < 1e-4 {
		yaw := float64(attacker.Rotation.Yaw) * math.Pi / 180
		dx, dz, d = -math.Sin(yaw), math.Cos(yaw), 1
	}
	const kb = 0.4
	m.push(target.UUID, dx/d*kb, 0.4, dz/d*kb)
	m.hurt(target.UUID, meleeDamage)                             // target's own health + red flash
	m.publish(Event{Type: EventHurt, Player: target.Snapshot()}) // hurt flash on all viewers
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
