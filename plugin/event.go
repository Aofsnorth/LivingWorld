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

// newBase is a small helper for constructing events with the right type tag.
func newBase(t EventType) BaseEvent { return BaseEvent{Type_: t} }
