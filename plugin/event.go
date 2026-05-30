package plugin

type EventType string

const (
	EventPlayerJoin     EventType = "player.join"
	EventPlayerLeave    EventType = "player.leave"
	EventPlayerChat     EventType = "player.chat"
	EventPlayerMove     EventType = "player.move"
	EventPlayerInteract EventType = "player.interact"
	EventBlockBreak     EventType = "block.break"
	EventBlockPlace     EventType = "block.place"
	EventServerStart    EventType = "server.start"
	EventServerStop     EventType = "server.stop"
	EventPlayerAttack   EventType = "player.attack"   // cancellable
	EventEntityDamage   EventType = "entity.damage"   // cancellable
	EventEntityDeath    EventType = "entity.death"    // not cancellable
	EventPlayerCommand  EventType = "player.command"  // cancellable
	EventContainerClick EventType = "container.click" // cancellable
	EventItemDrop       EventType = "item.drop"       // cancellable
	EventItemPickup     EventType = "item.pickup"     // cancellable
	EventPlayerRespawn  EventType = "player.respawn"  // not cancellable
)

// Event is the common interface for all events.
type Event interface {
	Type() EventType
}

// Cancellable is implemented by events that a handler can veto. When an event is
// cancelled the server skips the default action (e.g. it won't break the block).
type Cancellable interface {
	Cancel()
	Cancelled() bool
}

// BaseEvent provides Type() and cancellation support for embedding events.
type BaseEvent struct {
	Type_     EventType
	cancelled bool
}

func (e *BaseEvent) Type() EventType { return e.Type_ }

// Cancel marks the event cancelled. No-op for events the server doesn't treat
// as cancellable.
func (e *BaseEvent) Cancel() { e.cancelled = true }

// Cancelled reports whether a handler cancelled the event.
func (e *BaseEvent) Cancelled() bool { return e.cancelled }

type PlayerJoinEvent struct {
	BaseEvent
	PlayerName string
	UUID       string
}

type PlayerLeaveEvent struct {
	BaseEvent
	PlayerName string
	Reason     string
}

// PlayerChatEvent is cancellable; cancelling suppresses the message.
type PlayerChatEvent struct {
	BaseEvent
	PlayerName string
	Message    string
}

type PlayerMoveEvent struct {
	BaseEvent
	PlayerName string
	X, Y, Z    float64
}

// BlockBreakEvent is cancellable; cancelling leaves the block in place.
type BlockBreakEvent struct {
	BaseEvent
	PlayerName string
	X, Y, Z    int
	BlockID    int32
}

// BlockPlaceEvent is cancellable; cancelling prevents placement.
type BlockPlaceEvent struct {
	BaseEvent
	PlayerName string
	X, Y, Z    int
	BlockID    int32
}

type ServerStartEvent struct{ BaseEvent }

type ServerStopEvent struct{ BaseEvent }

// PlayerInteractEvent fires when a player right-clicks a block or air. Cancellable.
type PlayerInteractEvent struct {
	BaseEvent
	PlayerName string
	X, Y, Z    int
	BlockID    int32
}

// PlayerAttackEvent fires when a player attacks an entity. Cancellable.
type PlayerAttackEvent struct {
	BaseEvent
	PlayerName string
	TargetID   int32
}

// EntityDamageEvent fires when an entity takes damage. Cancellable.
type EntityDamageEvent struct {
	BaseEvent
	EntityID int32
	Cause    string
	Amount   float64
}

// EntityDeathEvent fires when an entity dies. Not cancellable.
type EntityDeathEvent struct {
	BaseEvent
	EntityID int32
	Cause    string
}

// PlayerCommandEvent fires before a command runs; cancelling suppresses it.
type PlayerCommandEvent struct {
	BaseEvent
	PlayerName string
	Command    string // command line without the leading slash
}

// ContainerClickEvent fires when a player clicks a container slot. Cancellable.
type ContainerClickEvent struct {
	BaseEvent
	PlayerName string
	Slot       int
}

// ItemDropEvent fires when a player drops an item. Cancellable.
type ItemDropEvent struct {
	BaseEvent
	PlayerName string
	ItemID     string
	Count      int
}

// ItemPickupEvent fires when a player picks up an item. Cancellable.
type ItemPickupEvent struct {
	BaseEvent
	PlayerName string
	ItemID     string
	Count      int
}

// PlayerRespawnEvent fires after a player respawns. Not cancellable.
type PlayerRespawnEvent struct {
	BaseEvent
	PlayerName string
	X, Y, Z    float64
}

// newBase is a small helper for constructing events with the right type tag.
func newBase(t EventType) BaseEvent { return BaseEvent{Type_: t} }
