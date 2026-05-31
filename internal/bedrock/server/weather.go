package server

import (
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// startWeatherSync pushes world weather changes to every Bedrock viewer.
func (s *Server) startWeatherSync() {
	s.wm.OnWeatherChange(func(raining, thundering bool) {
		s.forEachSession(func(v *bedrockSession) { sendBedrockWeather(v, raining, thundering) })
	})
}

// sendWeatherTo sends the current world weather to one viewer (on join).
func (s *Server) sendWeatherTo(v *bedrockSession) {
	raining, thundering := s.wm.GetDefaultWorld().Weather()
	sendBedrockWeather(v, raining, thundering)
}

func sendBedrockWeather(v *bedrockSession, raining, thundering bool) {
	if !raining {
		v.write(&packet.LevelEvent{EventType: packet.LevelEventStopRaining})
		v.write(&packet.LevelEvent{EventType: packet.LevelEventStopThunderstorm})
		return
	}
	v.write(&packet.LevelEvent{EventType: packet.LevelEventStartRaining, EventData: 65535})
	if thundering {
		v.write(&packet.LevelEvent{EventType: packet.LevelEventStartThunderstorm, EventData: 65535})
	} else {
		v.write(&packet.LevelEvent{EventType: packet.LevelEventStopThunderstorm})
	}
}
