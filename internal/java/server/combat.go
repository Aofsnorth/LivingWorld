package server

import (
	"livingworld/internal/java/protocol"
	"livingworld/internal/player"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// HandleAttack processes ServerboundGameAttack (minecraft:attack, 26.1+ dedicated
// left-click entity attack packet carrying targetEntityID(VarInt)).
func (s *PlayerSession) HandleAttack(p pk.Packet) {
	var targetID pk.VarInt
	if err := p.Scan(&targetID); err != nil {
		return
	}
	s.broadcastSwing(false)
	s.Bridge.routeAttack(s.UUID(), int32(targetID))
}

// HandleInteract processes ServerboundGameInteract (minecraft:interact).
// In Minecraft 26.1 (protocol 775), ServerboundGameInteract is exclusively a
// right-click entity interaction packet with layout:
//
//	entityId(VarInt), hand(VarInt: 0=MAIN_HAND, 1=OFF_HAND), location(lpVec3), usingSecondaryAction(Boolean).
//
// Left-click entity attacks arrive on ServerboundGameAttack (HandleAttack), so
// HandleInteract must never trigger melee damage or knockback.
func (s *PlayerSession) HandleInteract(p pk.Packet) {
	var targetID, hand pk.VarInt
	if err := p.Scan(&targetID, &hand); err != nil {
		return
	}
	_ = targetID
	_ = hand
}

// Hurt implements player.Controller: apply melee damage to this Java player —
// drop its hearts (sendHealth) and play the hurt animation (camera tilt).
func (s *PlayerSession) Hurt(amount float32) {
	s.damage(amount)
}

// hurtAnimationPacket plays the red hurt flash / camera tilt for an entity.
func hurtAnimationPacket(entityID int32) pk.Packet {
	return pk.Marshal(packetid.ClientboundGameHurtAnimation, pk.VarInt(entityID), pk.Float(0))
}

// SyncEnvironment implements player.EnvironmentController: pushes updated
// server-authoritative environment metadata (on-fire flag and air supply bubbles)
// to this Java player's own client without overwriting client-controlled pose
// or sprinting/swimming flags.
func (s *PlayerSession) SyncEnvironment(snap player.PlayerSnapshot) {
	if !s.Ready || s.version == nil {
		return
	}
	air := snap.AirSupply
	if air == 0 && snap.Breathing {
		air = 300
	}
	s.mu.Lock()
	fireChanged := snap.OnFire != s.lastOnFire
	airChanged := air != s.lastAirSupply
	s.lastOnFire = snap.OnFire
	s.lastAirSupply = air
	s.mu.Unlock()

	if !fireChanged && !airChanged {
		return
	}
	snap.AirSupply = air
	data := protocol.EncodeLocalEnvironmentMetadata(s.EntityID(), snap, fireChanged, airChanged)
	_ = s.SendPacket(pk.Packet{
		ID:   int32(packetid.ClientboundGameSetEntityData),
		Data: data,
	})
}
