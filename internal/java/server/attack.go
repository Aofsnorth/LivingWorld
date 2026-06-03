package server

import (
	"livingworld/internal/combat"
	"math"

	"github.com/google/uuid"
)

// routeAttack resolves a player→entity attack and applies the
// right downstream effect. targetEntityID is the entity runtime
// id from the Java ServerboundInteract packet (1=ATTACK) or the
// Bedrock InventoryTransaction with UseItemOnEntityTransactionData
// ActionType=Attack.
//
// Routing:
//   - If the target id matches a live mob in any world, apply
//     direct damage to that mob and play the hurt flash.
//     Damage scales by the attacker's held item (sword damage
//     table). Knockback velocity is set on the mob and consumed
//     by the AI integrator on the next tick.
//   - Otherwise, fall through to the player→player path in
//     pm.MeleeAttack (existing M0 behaviour). Sword scaling and
//     shield block are skipped there — player-vs-player combat
//     is a M7.x follow-up.
//
// M7 adds: sword damage scaling, I-frames stamp, knockback
// velocity, optional sweep (1-block horizontal radius, 50%
// base, only when the held item is a sword), and shield block
// when the defender has a shield in the offhand slot.
//
// The mob lookup is O(1) per world via mobStore.Get, so the
// cost is one map probe plus a player lookup. Sweep iterates
// the mob store; with the M1 cap (≤50 hostiles) that's a
// trivial loop.
func (j *javaBridge) routeAttack(attacker uuid.UUID, targetEntityID int32) {
	// Mob routing: probe the world manager's mob store first.
	if m := j.wm.Mobs().Get(int64(targetEntityID)); m.EntityID != 0 {
		var attackerBytes [16]byte
		copy(attackerBytes[:], attacker[:])
		// Read the attacker's held item to pick sword damage.
		// Bare hand / non-sword = 1 HP (combat.SwordDamage
		// returns 1 for any non-sword id).
		base := float32(1.0)
		var heldID int32
		if p := j.pm.GetPlayer(attacker); p != nil {
			held := p.Inventory.GetHeldItem()
			if held != nil {
				heldID = held.ID
				base = float32(combat.SwordDamage(heldID))
			}
		}
		// Compute push direction (attacker → mob). Reuse the
		// direction we walk the mob's attacker delta with.
		dx := float64(m.X) - 0
		dz := float64(m.Z) - 0
		// Use the attacker's position for the direction so a
		// bare hit from a known player direction always
		// staggers the mob away from the player.
		if p := j.pm.GetPlayer(attacker); p != nil {
			dx = float64(m.X) - p.Position.X
			dz = float64(m.Z) - p.Position.Z
		}
		const kbStrength = 0.4 // vanilla bare-hit knockback
		j.wm.Mobs().HurtDirectWithKnockback(m.EntityID, attackerBytes, base, dx, dz, kbStrength)
		// Hurt flash: ClientboundGameHurtAnimation (id 0x1C)
		// with the mob's entity id, plays the red flash on the
		// mob for every viewer.
		j.sessions.Broadcast(hurtAnimationPacket(targetEntityID))
		// Sweep: if the held item is a sword, fan a 50% damage
		// hit out to every other mob within a 1-block
		// horizontal radius. Sweep does NOT trigger knockback
		// or I-frames on the swept mobs.
		if combat.IsSwordItem(heldID) {
			j.sweepAttack(attackerBytes, attacker, m.X, m.Y, m.Z, m.EntityID, base)
		}
		return
	}
	// Player path (M0): existing knockback + 1 damage.
	j.pm.MeleeAttack(attacker, targetEntityID)
}

// sweepAttack fans a single-swing sword hit across every mob
// within a 1-block horizontal radius of (cx, cy, cz), excluding
// the main target. Sweep damage is 50% of the base sword damage
// (combat.SweepDamage). It applies only the damage, no knockback
// and no I-frames — the mob still gets the next-swing invuln
// via the main target's HurtDirect.
func (j *javaBridge) sweepAttack(attacker [16]byte, attackerUUID uuid.UUID, cx, cy, cz float64, excludeID int64, baseDamage float32) {
	amount := float32(combat.SweepDamage(float64(baseDamage)))
	if amount <= 0 {
		return
	}
	for _, m := range j.wm.Mobs().All() {
		if m.EntityID == excludeID {
			continue
		}
		// Y check: don't sweep mobs on different floors.
		if math.Abs(m.Y-cy) > 1.0 {
			continue
		}
		dx := m.X - cx
		dz := m.Z - cz
		if dx*dx+dz*dz > 1.0 {
			continue
		}
		j.wm.Mobs().HurtDirect(m.EntityID, attacker, amount)
		// Hurt flash for the swept mob.
		j.sessions.Broadcast(hurtAnimationPacket(int32(m.EntityID)))
	}
	_ = attackerUUID
}
