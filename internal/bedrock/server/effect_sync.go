package server

import (
	"livingworld/internal/world"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// M6: Bedrock wire format for status effects. gophertunnel ships a
// built-in packet.MobEffect with Operation = MobEffectAdd |
// MobEffectModify | MobEffectRemove. The EffectType field is one
// of EffectSpeed..EffectSlowFalling (gophertunnel/minecraft/protocol/
// packet/mob_effect.go:13-41) — these number 1..27 identically to
// vanilla Java, so a single EffectID flows through both bridges.
// Amplifier is 0-based (level 1 = 0), Duration in seconds. Particles
// is true for client-side rendering.
//
// The session manager's FindForUUID lookup avoids running the
// broadcast fan-out — the EffectStatus event targets exactly one
// player, so we send the MobEffect packet to that one session.

// startEffectEventLoop renders cross-edition action effects (crack overlay, break
// particles+sound) that originated on the OTHER edition (Java). Bedrock-sourced
// events are skipped — Bedrock already rendered those via its own direct
// broadcast, and replaying the LevelEvent would double the break sound.
func (s *Server) startEffectEventLoop() {
	ch := s.wm.SubscribeWorldEffects("bedrock-effects", 256)
	go func() {
		for ev := range ch {
			if ev.Source == world.BlockUpdateSourceBedrock {
				continue
			}
			s.broadcastEffect(ev)
		}
	}()
}

func (s *Server) broadcastEffect(ev world.WorldEffectEvent) {
	pos := protocol.BlockPos{int32(ev.X), int32(ev.Y), int32(ev.Z)}
	switch ev.Kind {
	case world.EffectCrackProgress:
		if ev.Stage < 0 {
			s.broadcastBlockCracking(pos, packet.LevelEventStopBlockCracking, 0)
		} else {
			// Per-block break duration: derived from the block at the
			// crack position so the Bedrock viewer's per-tick increment
			// (LevelEventStartBlockCracking's EventData) matches the
			// hardness the breaking player sees on their side. The block
			// is still stone (or whatever) at this moment because the
			// crack start fires before SetBlockAndPublish turns it to air.
			breakSecs := bedrockCrackSecondsFor(s.wm.GetDefaultWorld().GetBlock(ev.X, ev.Y, ev.Z).ID())
			s.broadcastBlockCracking(pos, packet.LevelEventStartBlockCracking, breakSecs)
		}
	case world.EffectBlockDestroy:
		s.broadcastBlockBreakEffect(int32(ev.X), int32(ev.Y), int32(ev.Z), ev.BlockID)
	case world.EffectStatus:
		s.sendMobEffect(ev, packet.MobEffectAdd)
	case world.EffectStatusRemove:
		s.sendMobEffect(ev, packet.MobEffectRemove)
	}
}

// sendMobEffect delivers a single packet.MobEffect to the targeted
// player. Operation is one of MobEffectAdd (1), MobEffectModify (2),
// MobEffectRemove (3) — for remove the Duration / Amplifier fields
// are unused on the wire and we leave them at zero. Tick is left at
// 0; the field is read by the client for CorrectPlayerMovePrediction
// math and isn't required for an effect packet. Ambient is false (v1
// effects are always sourced from a mob hit, never from a beacon /
// conduit).
func (s *Server) sendMobEffect(ev world.WorldEffectEvent, operation byte) {
	s.sessionsMu.RLock()
	sess, ok := s.sessions[ev.Target.String()]
	s.sessionsMu.RUnlock()
	if !ok {
		return
	}
	pk := buildMobEffectPacket(sess.runtimeID, ev, operation)
	if err := sess.conn.WritePacket(pk); err != nil {
		// Best-effort: drop the packet on a closed conn. The next
		// AddEffect will re-broadcast the add on the new conn.
		_ = err
	}
}

// buildMobEffectPacket constructs the wire-format packet.MobEffect
// from a WorldEffectEvent + the targeted runtime id. Exposed as a
// free function so unit tests can assert on the field values
// without spinning up a real Bedrock conn (gophertunnel's
// WritePacket dereferences a context / mutex on the conn, so a
// nil conn is not a usable test fixture).
func buildMobEffectPacket(runtimeID uint64, ev world.WorldEffectEvent, operation byte) *packet.MobEffect {
	duration := int32(0)
	amplifier := int32(0)
	particles := false
	if operation == packet.MobEffectAdd {
		duration = ev.Aux / 20 // ticks → seconds
		amplifier = ev.Data
		particles = true
	}
	return &packet.MobEffect{
		EntityRuntimeID: runtimeID,
		Operation:       operation,
		EffectType:      ev.EffectID,
		Amplifier:       amplifier,
		Particles:       particles,
		Duration:        duration,
		Tick:            0,
		Ambient:         false,
	}
}
