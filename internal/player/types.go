package player

import (
	"livingworld/internal/world"

	"github.com/google/uuid"
)

type EventType string

const (
	EventJoin      EventType = "join"
	EventMove      EventType = "move"
	EventLeave     EventType = "leave"
	EventSwing     EventType = "swing"
	EventSneak     EventType = "sneak"
	EventSkin      EventType = "skin"
	EventEquipment EventType = "equipment"
	EventHurt      EventType = "hurt"
)

type Event struct {
	Type     EventType
	Player   PlayerSnapshot
	Teleport bool
}

type ProfileProperty struct {
	Name      string
	Value     string
	Signature string
}

type Edition string

const (
	EditionJava    Edition = "java"
	EditionBedrock Edition = "bedrock"
)

type PlayerSnapshot struct {
	UUID              uuid.UUID
	Username          string
	Edition           Edition
	EntityRuntimeID   uint64
	Position          world.Position
	Rotation          world.Rotation
	OnGround          bool
	Sneaking          bool
	ProfileProperties []ProfileProperty
	BedrockSkinURL    string
	Skin              *SkinData
	SkinParts         byte
	Creative          bool
}

type Player struct {
	UUID              uuid.UUID
	Username          string
	Edition           Edition
	XUID              uint64
	EntityRuntimeID   uint64
	World             *world.World
	Position          world.Position
	Rotation          world.Rotation
	OnGround          bool
	Sneaking          bool
	Health            float32
	Food              int
	Saturation        float32
	Inventory         *Inventory
	Creative          bool
	Op                bool
	Flying            bool
	Skin              *SkinData
	ProfileProperties []ProfileProperty
	BedrockSkinURL    string
	SkinParts         byte

	// Phase 3 player model extension: explicit XP + gamemode fields. These
	// are independent from `Creative` so a future Creative-mode-only refactor
	// (or a Spectator-mode player) round-trips through persistence correctly.
	XPLevel    int
	XPProgress float32
	Gamemode   int // 0 survival, 1 creative, 2 adventure, 3 spectator

	// HeldItemSlot tracks currently held hotbar slot for equipment broadcasting
	HeldItemSlot int
}

// Player construction + event defaults (player-internal; distinct from the
// configurable world spawn). Names make the magic numbers self-documenting.
const (
	defaultSpawnY      = 64 // safe construction height before real spawn placement
	defaultEventBuffer = 64 // buffered Event channel size for Subscribe

	msgJoinedGame = "%s joined the game"
	msgLeftGame   = "%s left the game"
)
