package server

import (
	javaworld "livingworld/internal/java/world"
	"livingworld/internal/world"
)

// startLightEventLoop re-sends a chunk's light to players when the world's light
// engine recomputes it (e.g. a cross-chunk relight that brightens a seam after a
// neighbour loads — see world.LightEngine.ProcessUpdates / World.queueNeighborRelight).
//
// The Java client trusts server-provided light and only receives it inside a
// chunk packet (there is no standalone light path here), so we rebuild and
// re-send the whole LevelChunkWithLight packet — but only to sessions that
// already have the chunk loaded, and only for chunks whose light actually
// changed (ProcessUpdates filters no-op relights), so an open-terrain walk does
// not generate redundant traffic.
func (j *javaBridge) startLightEventLoop() {
	ch := j.wm.SubscribeLightUpdates("java-bridge", 256)
	go func() {
		for ev := range ch {
			j.resendChunkLight(ev)
		}
	}()
}

func (j *javaBridge) resendChunkLight(ev world.LightUpdateEvent) {
	pos := world.ChunkPos{X: ev.X, Z: ev.Z}

	// Collect the sessions that currently have this chunk loaded; skip the
	// rebuild entirely if nobody does.
	var targets []*PlayerSession
	j.sessions.ForEach(func(s *PlayerSession) {
		s.mu.Lock()
		loaded := s.LoadedChunks[pos]
		s.mu.Unlock()
		if loaded {
			targets = append(targets, s)
		}
	})
	if len(targets) == 0 {
		return
	}

	w := j.wm.GetDefaultWorld()
	lChunk := javaworld.ConvertToLevelChunk(w.LoadChunk(ev.X, ev.Z))
	packet := javaworld.BuildLevelChunkWithLightPacket(int32(ev.X), int32(ev.Z), lChunk)
	for _, s := range targets {
		_ = s.SendPacket(packet)
	}
}
