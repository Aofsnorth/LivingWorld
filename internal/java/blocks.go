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

	// Java sends start digging first, then finish digging when the client-side
	// survival break timer completes. Breaking on start made survival mining
	// instant. Until server-authoritative hardness exists, only accept finish.
	if status == 2 { // finish digging
		s.Bridge.wm.SetBlockAndPublish(world.BlockUpdateSourceJava, pos.X, pos.Y, pos.Z, world.BlockAir{})
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

	stateID := s.getBlockStateForPlacement()
	if stateID == 0 {
		// Survival placeholder: until real held-item placement is implemented,
		// don't conjure stone. Re-sync both clicked and target block to roll back
		// client prediction.
		currentID := s.Bridge.wm.GetDefaultWorld().GetBlock(pos.X, pos.Y, pos.Z).ID()
		_ = s.SendPacket(pk.Marshal(packetid.ClientboundGameBlockUpdate, pos, pk.VarInt(livingWorldBlockIDToJavaStateID(currentID))))
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
		targetID := s.Bridge.wm.GetDefaultWorld().GetBlock(x, y, z).ID()
		_ = s.SendPacket(pk.Marshal(packetid.ClientboundGameBlockUpdate, pk.Position{X: x, Y: y, Z: z}, pk.VarInt(livingWorldBlockIDToJavaStateID(targetID))))
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
	blockID := javaStateIDToLivingWorldBlockID(int32(stateID))
	s.Bridge.wm.SetBlockAndPublish(world.BlockUpdateSourceJava, x, y, z, world.PlaceholderBlock{IDValue: blockID})
}

func (s *PlayerSession) getBlockStateForPlacement() block.StateID {
	return 0
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
