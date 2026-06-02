package server

import (
	"time"

	"livingworld/internal/world"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// startCrackProgressLoop periodically advances the Java-side crack overlay for
// every Bedrock breaker. The Bedrock client only sends PlayerActionStartBreak
// then a final destroy action — it does NOT reliably emit
// PlayerActionContinueDestroyBlock for every tick — so an event-driven update
// path freezes the Java overlay at stage 0. A 75ms tick gives ~10 evenly-spaced
// stage transitions across the default 0.75s break window, matching what the
// Bedrock client renders locally via LevelEventStartBlockCracking's per-tick
// progress data.
func (s *Server) startCrackProgressLoop() {
	go func() {
		ticker := time.NewTicker(75 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if !s.running {
				return
			}
			s.tickCrackProgress()
		}
	}()
}

// tickCrackProgress checks every connected Bedrock session for an active
// breaking state and publishes a stage update to the world effect bus when the
// computed stage advances. PublishCrack carries BlockUpdateSourceBedrock so the
// Bedrock effect subscriber skips it (Bedrock self-animates from the start
// event) and only the Java bridge renders the BlockDestruction packet.
func (s *Server) tickCrackProgress() {
	cm := s.wm.CrackManager()
	s.forEachSession(func(bs *bedrockSession) {
		st := cm.GetBreaking(bs.id)
		if st == nil {
			return
		}
		stage, changed := cm.AdvanceStage(bs.id, bedrockCrackBreakSeconds)
		if !changed {
			return
		}
		pos := protocol.BlockPos{int32(st.BlockPos.X), int32(st.BlockPos.Y), int32(st.BlockPos.Z)}
		s.wm.PublishCrack(world.BlockUpdateSourceBedrock, bs.id, int(pos[0]), int(pos[1]), int(pos[2]), stage)
	})
}
