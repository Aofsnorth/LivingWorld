package protocol

import (
	"livingworld/config"
	"livingworld/internal/player"
	"livingworld/internal/world"

	gmnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/google/uuid"
)

// Metadata Constants to avoid hardcoded magic numbers
const (
	// Metadata Indexes.
	//
	// These follow the Minecraft Java Edition 26.1 (protocol 775) entity-data
	// layout. In 26.1 an Avatar class was inserted between LivingEntity and
	// Player, so the player-section indices differ from older versions:
	//   index 15 = Main Hand (Byte)
	//   index 16 = Displayed Skin Parts (Byte)
	//   index 17 = Additional Hearts / absorption (Float)  <- NOT skin parts
	// Sending skin parts at the old index 17 makes a 26.1 client crash with
	// "Invalid entity data item type ... old=0.0(Float), new=...(Byte)".
	MetaIndexFlags     = 0
	MetaIndexPose      = 6
	MetaIndexMainHand  = 15
	MetaIndexSkinParts = 16

	// Metadata Types
	MetaTypeByte = 0
	MetaTypePose = 20

	// Entity Flags
	EntityFlagSneaking = 0x02

	// Pose Enum Values
	PoseStanding  = 0
	PoseCrouching = 5
)

// Session interface defines the behavior expected from a Java player session,
// allowing decoupling of the version handlers from the concrete session struct.
type Session interface {
	Conn() *gmnet.Conn
	UUID() uuid.UUID
	Username() string
	EntityID() int32
	Config() *config.Config
	PlayerManager() *player.Manager
	WorldManager() *world.Manager
	GameMode() int32
	SendSpawnPosition() error
	SendHealth() error
	UpdateChunks()
	SendWorldState()

	HandleChatCommand(p pk.Packet)

	HandleMovePos(p pk.Packet)
	HandleMovePosRot(p pk.Packet)
	HandleMoveRot(p pk.Packet)
	HandleMoveStatusOnly(p pk.Packet)
	HandlePlayerAction(p pk.Packet)
	HandleUseItemOn(p pk.Packet)
	HandleChat(p pk.Packet)
	HandleCreativeSlot(p pk.Packet)
	HandleSetCarriedItem(p pk.Packet)
	HandleSwing(p pk.Packet)
	HandlePlayerCommand(p pk.Packet)
	HandleInteract(p pk.Packet)
	SendPacket(p pk.Packet) error
}

// VersionHandler abstracts all version-dependent logic for Minecraft Java clients
type VersionHandler interface {
	ProtocolVersion() int
	ProtocolName() string

	SendInitialPlayPackets(s Session) error
	SpawnForeignAvatar(s Session, p player.PlayerSnapshot) error
	MoveForeignAvatar(s Session, p player.PlayerSnapshot, oldPos world.Position, exists bool) error
	RemoveForeignAvatar(s Session, p player.PlayerSnapshot) error
	SwingForeignAvatar(s Session, p player.PlayerSnapshot) error
	SendPlayerInfoAdd(s Session, p player.PlayerSnapshot) error
	SendPlayerInfoRemove(s Session, p player.PlayerSnapshot) error
	UpdateForeignMetadata(s Session, p player.PlayerSnapshot) error
	UpdateForeignEquipment(s Session, p player.PlayerSnapshot) error
	HandlePacket(s Session, p pk.Packet)

	// Dropped item entities.
	SpawnItemEntity(s Session, entityID int32, itemName string, count int, x, y, z float64) error
	RemoveItemEntity(s Session, entityID int32) error
	TakeItemEntity(s Session, itemEntityID, collectorEntityID int32, count int) error
}

var registeredHandlers = make(map[int]VersionHandler)

// RegisterVersion registers a VersionHandler for a specific protocol version
func RegisterVersion(handler VersionHandler) {
	registeredHandlers[handler.ProtocolVersion()] = handler
}

// GetVersionHandler retrieves the VersionHandler for a given protocol version
func GetVersionHandler(protocol int) (VersionHandler, bool) {
	h, ok := registeredHandlers[protocol]
	return h, ok
}

// GetSupportedProtocols returns a list of all registered protocol versions
func GetSupportedProtocols() []int {
	protocols := make([]int, 0, len(registeredHandlers))
	for p := range registeredHandlers {
		protocols = append(protocols, p)
	}
	return protocols
}
