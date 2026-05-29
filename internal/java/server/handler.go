package server

import (
	pk "github.com/Tnze/go-mc/net/packet"
)

func (s *PlayerSession) HandlePacket(p pk.Packet) {
	s.version.HandlePacket(s, p)
}
