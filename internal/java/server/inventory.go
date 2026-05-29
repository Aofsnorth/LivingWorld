package server

import (
	"bytes"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

func (s *PlayerSession) sendInventory() {
	var buf bytes.Buffer
	pk.VarInt(0).WriteTo(&buf)
	pk.VarInt(0).WriteTo(&buf)
	pk.VarInt(46).WriteTo(&buf)
	for i := 0; i < 46; i++ {
		pk.VarInt(0).WriteTo(&buf)
	}
	pk.VarInt(0).WriteTo(&buf)
	_ = s.SendPacket(pk.Packet{
		ID:   int32(packetid.ClientboundGameContainerSetContent),
		Data: buf.Bytes(),
	})
}

func (s *PlayerSession) HandleCreativeSlot(p pk.Packet) {
	var slot pk.Short
	if err := p.Scan(&slot); err != nil {
		return
	}
}

func (s *PlayerSession) HandleSetCarriedItem(p pk.Packet) {
	var slot pk.Short
	if err := p.Scan(&slot); err != nil {
		return
	}
	s.mu.Lock()
	s.SelectedSlot = int32(slot)
	s.mu.Unlock()
}
