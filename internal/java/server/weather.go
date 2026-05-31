package server

import (
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// Vanilla GameEvent IDs (ClientboundGameGameEvent).
const (
	gameEventBeginRain    = 1
	gameEventEndRain      = 2
	gameEventRainLevel    = 7
	gameEventThunderLevel = 8
)

func weatherPacket(event byte, level float32) pk.Packet {
	return pk.Marshal(packetid.ClientboundGameGameEvent, pk.UnsignedByte(event), pk.Float(level))
}

// sendWeather pushes the current world weather to one session (used on join).
func (s *PlayerSession) sendWeather() {
	raining, thundering := s.Bridge.wm.GetDefaultWorld().Weather()
	writeWeather(func(p pk.Packet) { _ = s.SendPacket(p) }, raining, thundering)
}

// broadcastWeather pushes a weather change to every connected Java session.
func (j *javaBridge) broadcastWeather(raining, thundering bool) {
	writeWeather(j.sessions.Broadcast, raining, thundering)
}

func writeWeather(send func(pk.Packet), raining, thundering bool) {
	rainLevel, thunderLevel := float32(0), float32(0)
	if raining {
		send(weatherPacket(gameEventBeginRain, 0))
		rainLevel = 1
		if thundering {
			thunderLevel = 1
		}
	} else {
		send(weatherPacket(gameEventEndRain, 0))
	}
	// Always set explicit rain + thunder LEVELS. Sending only stop-rain left the
	// client's rain level stuck at its last value (1.0), so it kept raining after
	// a clear ("hujan terus"); zeroing the level here forces it fully clear.
	send(weatherPacket(gameEventRainLevel, rainLevel))
	send(weatherPacket(gameEventThunderLevel, thunderLevel))
}
