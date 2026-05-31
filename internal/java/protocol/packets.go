package protocol

import (
	"math"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

func (h *Handler775) HandlePacket(s Session, p pk.Packet) {
	switch packetid.ServerboundPacketID(p.ID) {
	case packetid.ServerboundGameMovePlayerPos:
		s.HandleMovePos(p)
	case packetid.ServerboundGameMovePlayerPosRot:
		s.HandleMovePosRot(p)
	case packetid.ServerboundGameMovePlayerRot:
		s.HandleMoveRot(p)
	case packetid.ServerboundGameMovePlayerStatusOnly:
		s.HandleMoveStatusOnly(p)
	case packetid.ServerboundGamePlayerAction:
		s.HandlePlayerAction(p)
	case packetid.ServerboundGameInteract:
		s.HandleInteract(p)
	case packetid.ServerboundGameUseItemOn:
		s.HandleUseItemOn(p)
	case packetid.ServerboundGameChat:
		s.HandleChat(p)
	case packetid.ServerboundGameChatCommand:
		s.HandleChatCommand(p)
	case packetid.ServerboundGameSetCreativeModeSlot:
		s.HandleCreativeSlot(p)
	case packetid.ServerboundGameSetCarriedItem:
		s.HandleSetCarriedItem(p)
	case packetid.ServerboundGameSwing:
		s.HandleSwing(p)
	case packetid.ServerboundGamePlayerCommand:
		s.HandlePlayerCommand(p)
	case packetid.ServerboundGamePlayerInput:
		var flags pk.Byte
		if err := p.Scan(&flags); err == nil {
			sneaking := (flags & 0x20) != 0
			s.PlayerManager().UpdateSneak(s.UUID(), sneaking)
		}
	case packetid.ServerboundGameKeepAlive:
	case packetid.ServerboundGameAcceptTeleportation:
	case packetid.ServerboundGamePlayerAbilities:
	case packetid.ServerboundGamePlayerLoaded:
	case packetid.ServerboundGameChunkBatchReceived:
	case packetid.ServerboundGameClientInformation:
		var (
			locale        pk.String
			viewDist      pk.Byte
			chatMode      pk.VarInt
			chatColors    pk.Boolean
			skinParts     pk.Byte
			mainHand      pk.VarInt
			textFiltering pk.Boolean
			allowListings pk.Boolean
		)
		if err := p.Scan(&locale, &viewDist, &chatMode, &chatColors, &skinParts, &mainHand, &textFiltering, &allowListings); err == nil {
			s.PlayerManager().UpdateSkinParts(s.UUID(), byte(skinParts))
		}
	case packetid.ServerboundGamePingRequest:
	case packetid.ServerboundGamePong:
	case packetid.ServerboundGameClientTickEnd:
	case packetid.ServerboundGameContainerClose:
	case packetid.ServerboundGameContainerSlotStateChanged:
	default:
	}
}

func degToAngle(deg float32) pk.Byte {
	v := int(math.Round(float64(deg) * 256.0 / 360.0))
	return pk.Byte(int8(v & 0xff))
}
