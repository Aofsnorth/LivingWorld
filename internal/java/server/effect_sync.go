package server

import (
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

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
	}
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
