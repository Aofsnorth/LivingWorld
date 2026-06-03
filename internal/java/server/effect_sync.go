package server

import (
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// M6: Java wire format for status effects. The vanilla packet ids
// live in third_party/go-mc/data/packetid as
// ClientboundGameUpdateMobEffect (132) and
// ClientboundGameRemoveMobEffect (78). Effect ids follow the
// vanilla 1.21 ordering (1..27) and are identical to the
// gophertunnel EffectSpeed..EffectSlowFalling numbering, so a
// single id flows through both bridges unchanged.
//
// ClientboundGameUpdateMobEffect wire format (Java 1.21):
//   entity id: VarInt, effect id: VarInt, amplifier: VarInt
//   duration: VarInt, flags: Byte
// Flags: bit 0 = ambient, bit 1 = show particles, bit 2 = show icon.
//
// ClientboundGameRemoveMobEffect wire format:
//   entity id: VarInt, effect id: VarInt.

// startEffectEventLoop renders cross-edition action effects (crack overlay, break
// particles+sound) that originated on the OTHER edition. Java-sourced events are
// skipped — the acting Java client already predicted those locally, and replaying
// the LevelEvent would double the break sound.
func (j *javaBridge) startEffectEventLoop() {
	ch := j.wm.SubscribeWorldEffects("java-bridge-effects", 256)
	go func() {
		for ev := range ch {
			if ev.Source == world.BlockUpdateSourceJava {
				continue
			}
			j.broadcastEffect(ev)
		}
	}()
}

func (j *javaBridge) broadcastEffect(ev world.WorldEffectEvent) {
	switch ev.Kind {
	case world.EffectCrackProgress:
		stage := ev.Stage
		if stage < 0 {
			stage = 10 // a stage outside 0..9 clears the overlay
		}
		j.sessions.Broadcast(javaBlockDestructionPacket(synthEntityID(ev.Breaker), ev.X, ev.Y, ev.Z, stage))
	case world.EffectBlockDestroy:
		// LevelEvent 2001 = block-break particles + sound. effectId and data are
		// FIXED 32-bit ints (pk.Int, NOT pk.VarInt); data is the Java block state id
		// (== canonical BlockID). disableRelativeVolume = false.
		j.sessions.Broadcast(pk.Marshal(
			packetid.ClientboundGameLevelEvent,
			pk.Int(2001),
			pk.Position{X: ev.X, Y: ev.Y, Z: ev.Z},
			pk.Int(livingWorldBlockIDToJavaStateID(ev.BlockID)),
			pk.Boolean(false),
		))
	case world.EffectStatus:
		j.broadcastStatusEffectAdd(ev)
	case world.EffectStatusRemove:
		j.broadcastStatusEffectRemove(ev)
	}
}

// broadcastStatusEffectAdd sends a single ClientboundGameUpdateMobEffect
// (packet id 132) to the targeted player.
func (j *javaBridge) broadcastStatusEffectAdd(ev world.WorldEffectEvent) {
	sess := j.sessions.Get(ev.Target)
	if sess == nil {
		return
	}
	_ = sess.SendPacket(javaUpdateMobEffectPacket(sess.EntityIDVal, ev.EffectID, ev.Data, ev.Aux))
}

// broadcastStatusEffectRemove sends a single
// ClientboundRemoveMobEffect (packet id 78) to the targeted
// player. The bridge doesn't need a flags byte — remove is just
// (entity, effectId).
func (j *javaBridge) broadcastStatusEffectRemove(ev world.WorldEffectEvent) {
	sess := j.sessions.Get(ev.Target)
	if sess == nil {
		return
	}
	_ = sess.SendPacket(javaRemoveMobEffectPacket(sess.EntityIDVal, ev.EffectID))
}

// javaUpdateMobEffectPacket builds the wire bytes for
// ClientboundGameUpdateMobEffect (id 132). Effect ids follow the
// vanilla 1.21 ordering (1..27) and are identical to the
// gophertunnel EffectSpeed..EffectSlowFalling numbering. amplifier
// is 0-based (level 1 = amplifier 0), duration is in ticks.
//
// Wire layout (Java 1.21):
//   packet id 132   (VarInt, 2 bytes: 0x84 0x01)
//   entity id       (VarInt)
//   effect id       (VarInt)
//   amplifier       (VarInt)
//   duration (ticks)(VarInt)
//   flags           (Byte)
//
// Flags 0x06 = bit 1 (show particles) | bit 2 (show icon). Bit 0
// (ambient) is off — v1 effects are always sourced from a mob hit.
func javaUpdateMobEffectPacket(entityID int32, effectID, amplifier, duration int32) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameUpdateMobEffect,
		pk.VarInt(entityID),
		pk.VarInt(effectID),
		pk.VarInt(amplifier),
		pk.VarInt(duration),
		pk.Byte(0x06),
	)
}

// javaRemoveMobEffectPacket builds the wire bytes for
// ClientboundRemoveMobEffect (id 78). Layout:
//   packet id 78    (VarInt, 1 byte: 0x4E)
//   entity id       (VarInt)
//   effect id       (VarInt)
func javaRemoveMobEffectPacket(entityID int32, effectID int32) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameRemoveMobEffect,
		pk.VarInt(entityID),
		pk.VarInt(effectID),
	)
}

// javaBlockDestructionPacket sets the crack overlay (stage 0..9) for entityID at a
// block, or clears it (stage >= 10). Vanilla keys the overlay by entityID, so the
// same id must be used for start and stop of one breaker.
func javaBlockDestructionPacket(entityID int32, x, y, z int, stage int32) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameBlockDestruction,
		pk.VarInt(entityID),
		pk.Position{X: x, Y: y, Z: z},
		pk.Byte(stage),
	)
}

// synthEntityID derives a stable entity id from a breaker's UUID. A cross-edition
// breaker has no real Java entity on this client, so we need a consistent
// synthetic key to start and later clear its crack overlay. We force the result
// into a reserved high range (bit 30 set) so it can't collide with real entity
// ids — players (small ints from 2), drops (1<<20) or mobs (1<<22) — and clobber a
// genuine player's crack overlay.
func synthEntityID(u uuid.UUID) int32 {
	v := int32(u[0])<<24 | int32(u[1])<<16 | int32(u[2])<<8 | int32(u[3])
	return (v & 0x3FFFFFFF) | 0x40000000 // ~1.07e9 .. ~2.14e9, well clear of real ids
}
