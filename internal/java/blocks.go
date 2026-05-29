package java

import (
	"livingworld/internal/world"

	"github.com/Tnze/go-mc/data/packetid"
	"github.com/Tnze/go-mc/level/block"
	pk "github.com/Tnze/go-mc/net/packet"
)

var airStateID = block.ToStateID[block.Air{}]

func (s *PlayerSession) handlePlayerAction(p pk.Packet) {
	var status pk.VarInt
	var pos pk.Position
	var face pk.Byte
	var sequence pk.VarInt
	if err := p.Scan(&status, &pos, &face, &sequence); err != nil {
		return
	}
	if status == 2 {
		s.Bridge.wm.GetDefaultWorld().SetBlock(pos.X, pos.Y, pos.Z, world.BlockAir{})
		s.Bridge.sessions.Broadcast(pk.Marshal(
			packetid.ClientboundGameBlockUpdate,
			pos, pk.VarInt(airStateID),
		))
	}
}

func (s *PlayerSession) handleUseItemOn(p pk.Packet) {
	var hand pk.VarInt
	var pos pk.Position
	var face pk.VarInt
	var cursorX, cursorY, cursorZ pk.Float
	var insideBlock, worldBorderHit pk.Boolean
	var sequence pk.VarInt
	if err := p.Scan(&hand, &pos, &face, &cursorX, &cursorY, &cursorZ, &insideBlock, &worldBorderHit, &sequence); err != nil {
		return
	}

	x, y, z := pos.X, pos.Y, pos.Z
	switch face {
	case 0:
		y--
	case 1:
		y++
	case 2:
		z--
	case 3:
		z++
	case 4:
		x--
	case 5:
		x++
	}

	stateID := s.getBlockStateForPlacement()
	if stateID == 0 {
		stateID = block.ToStateID[block.Stone{}]
	}
	s.Bridge.wm.GetDefaultWorld().SetBlock(x, y, z, world.PlaceholderBlock{IDValue: int32(stateID)})
	s.Bridge.sessions.Broadcast(pk.Marshal(
		packetid.ClientboundGameBlockUpdate,
		pk.Position{X: x, Y: y, Z: z}, pk.VarInt(stateID),
	))
}

func (s *PlayerSession) getBlockStateForPlacement() block.StateID {
	return block.ToStateID[block.Stone{}]
}

func (s *PlayerSession) handleSwing(p pk.Packet) {
	var hand pk.VarInt
	if err := p.Scan(&hand); err != nil {
		return
	}
	s.Bridge.sessions.BroadcastExcept(s.UUID, pk.Marshal(
		packetid.ClientboundGameAnimate,
		pk.VarInt(s.EntityID), pk.UnsignedByte(0),
	))
}
