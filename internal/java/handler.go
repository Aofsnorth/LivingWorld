package java

import (
	"log"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

func (s *PlayerSession) HandlePacket(p pk.Packet) {
	switch packetid.ServerboundPacketID(p.ID) {
	case packetid.ServerboundGameMovePlayerPos:
		s.handleMovePos(p)
	case packetid.ServerboundGameMovePlayerPosRot:
		s.handleMovePosRot(p)
	case packetid.ServerboundGameMovePlayerRot:
		s.handleMoveRot(p)
	case packetid.ServerboundGameMovePlayerStatusOnly:
		s.handleMoveStatusOnly(p)
	case packetid.ServerboundGamePlayerAction:
		s.handlePlayerAction(p)
	case packetid.ServerboundGameUseItemOn:
		s.handleUseItemOn(p)
	case packetid.ServerboundGameChat:
		s.handleChat(p)
	case packetid.ServerboundGameSetCreativeModeSlot:
		s.handleCreativeSlot(p)
	case packetid.ServerboundGameSetCarriedItem:
		s.handleSetCarriedItem(p)
	case packetid.ServerboundGameSwing:
		s.handleSwing(p)
	case packetid.ServerboundGameKeepAlive:
	case packetid.ServerboundGameAcceptTeleportation:
	case packetid.ServerboundGamePlayerAbilities:
	case packetid.ServerboundGamePlayerLoaded:
	case packetid.ServerboundGameChunkBatchReceived:
	case packetid.ServerboundGameClientInformation:
	case packetid.ServerboundGamePingRequest:
	case packetid.ServerboundGamePong:
	case packetid.ServerboundGameClientTickEnd:
	case packetid.ServerboundGameContainerClose:
	case packetid.ServerboundGameContainerSlotStateChanged:
	default:
		log.Printf("[Java] Unhandled packet: 0x%02x", p.ID)
	}
}
