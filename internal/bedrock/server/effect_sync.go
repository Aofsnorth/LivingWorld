package server

import (
	"livingworld/internal/world"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

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
			s.broadcastBlockCracking(pos, packet.LevelEventStopBlockCracking)
		} else {
			s.broadcastBlockCracking(pos, packet.LevelEventStartBlockCracking)
		}
	case world.EffectBlockDestroy:
		s.broadcastBlockBreakEffect(int32(ev.X), int32(ev.Y), int32(ev.Z), ev.BlockID)
	}
}
