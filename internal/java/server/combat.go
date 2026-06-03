package server

import (
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// HandleInteract processes ServerboundInteract. Only the ATTACK action is acted
// on: it routes through routeAttack which decides whether the target is a mob
// (M5) or a player (M0). Interact packet: targetEntityID(VarInt),
// type(VarInt: 0=interact, 1=attack, 2=interact_at), then type-specific
// trailing fields we don't need here.
func (s *PlayerSession) HandleInteract(p pk.Packet) {
	var targetID, action pk.VarInt
	if err := p.Scan(&targetID, &action); err != nil {
		return
	}
	if action != 1 { // not ATTACK
		return
	}
	s.Bridge.routeAttack(s.UUID(), int32(targetID))
}

// Hurt implements player.Controller: apply melee damage to this Java player —
// drop its hearts (sendHealth) and play the hurt animation (camera tilt).
func (s *PlayerSession) Hurt(amount float32) {
	s.damage(amount)
	_ = s.SendPacket(hurtAnimationPacket(s.EntityID()))
}

// hurtAnimationPacket plays the red hurt flash / camera tilt for an entity.
func hurtAnimationPacket(entityID int32) pk.Packet {
	return pk.Marshal(packetid.ClientboundGameHurtAnimation, pk.VarInt(entityID), pk.Float(0))
}
