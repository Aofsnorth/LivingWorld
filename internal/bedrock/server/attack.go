package server

import (
	"livingworld/internal/combat"
	"math"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// routeBedrockAttack resolves a player→entity attack on the
// Bedrock bridge. The target id is the entity runtime id from
// the InventoryTransaction{UseItemOnEntityTransactionData}
// packet with ActionType=Attack.
//
// Routing:
//   - Probe the world manager's mob store. If the id matches a
//     live mob, apply direct damage via HurtDirect and play the
//     hurt flash on every Bedrock viewer. Sword damage scales
//     by the attacker's held item, knockback velocity is set on
//     the mob, and (for swords) a sweep fan-out hits every mob
//     within a 1-block horizontal radius.
//   - Otherwise, fall through to the player→player path
//     (pm.MeleeAttack).
//
// M7: sword damage, I-frames stamp, knockback velocity, sweep.
func (s *Server) routeBedrockAttack(attacker uuid.UUID, targetEntityRuntimeID int64) {
	// Mob routing: probe the world manager's mob store.
	if m := s.wm.Mobs().Get(targetEntityRuntimeID); m.EntityID != 0 {
		var attackerBytes [16]byte
		copy(attackerBytes[:], attacker[:])
		// Held item → sword damage. Bare hand = 1 HP.
		base := float32(1.0)
		var heldID int32
		if p := s.pm.GetPlayer(attacker); p != nil {
			held := p.Inventory.GetHeldItem()
			if held != nil {
				heldID = held.ID
				base = float32(combat.SwordDamage(heldID))
			}
		}
		// Push direction: mob minus attacker.
		dx, dz := float64(m.X), float64(m.Z)
		if p := s.pm.GetPlayer(attacker); p != nil {
			dx = float64(m.X) - p.Position.X
			dz = float64(m.Z) - p.Position.Z
		}
		const kbStrength = 0.4
		s.wm.Mobs().HurtDirectWithKnockback(m.EntityID, attackerBytes, base, dx, dz, kbStrength)
		// Hurt flash on every Bedrock viewer. ActorEvent(2) =
		// Hurt. Sends a red flash on the mob for the local
		// client and all other viewers in AOI.
		s.forEachSession(func(v *bedrockSession) {
			v.write(&packet.ActorEvent{
				EntityRuntimeID: uint64(targetEntityRuntimeID),
				EventType:       packet.ActorEventHurt,
			})
		})
		// Sweep fan-out (swords only).
		if combat.IsSwordItem(heldID) {
			s.sweepBedrockAttack(attackerBytes, m.X, m.Y, m.Z, m.EntityID, base)
		}
		return
	}
	// Player path (M0): existing knockback + 1 damage.
	s.pm.MeleeAttack(attacker, int32(targetEntityRuntimeID))
}

// sweepBedrockAttack fans a sword swing's 50% damage hit across
// every mob within a 1-block horizontal radius of (cx, cy, cz),
// excluding the main target. Mirrors the Java bridge's
// sweepAttack; no knockback and no I-frames on the swept mobs.
func (s *Server) sweepBedrockAttack(attacker [16]byte, cx, cy, cz float64, excludeID int64, baseDamage float32) {
	amount := float32(combat.SweepDamage(float64(baseDamage)))
	if amount <= 0 {
		return
	}
	for _, m := range s.wm.Mobs().All() {
		if m.EntityID == excludeID {
			continue
		}
		if math.Abs(m.Y-cy) > 1.0 {
			continue
		}
		dx := m.X - cx
		dz := m.Z - cz
		if dx*dx+dz*dz > 1.0 {
			continue
		}
		s.wm.Mobs().HurtDirect(m.EntityID, attacker, amount)
		// Hurt flash for the swept mob.
		s.forEachSession(func(v *bedrockSession) {
			v.write(&packet.ActorEvent{
				EntityRuntimeID: uint64(m.EntityID),
				EventType:       packet.ActorEventHurt,
			})
		})
	}
}
