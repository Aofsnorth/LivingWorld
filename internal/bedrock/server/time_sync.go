package server

import (
	"time"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// startTimeLoop periodically pushes the authoritative world time to every Bedrock
// session. doDaylightCycle is false (set in GameData), so the client does not
// advance its own sun — the server stays the single source of truth and both
// editions stay frame-aligned.
func (s *Server) startTimeLoop() {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if !s.running {
				return
			}
			t := int32(s.wm.GetDefaultWorld().GetDayTime())
			s.forEachSession(func(bs *bedrockSession) {
				bs.write(&packet.SetTime{Time: t})
			})
		}
	}()
}
