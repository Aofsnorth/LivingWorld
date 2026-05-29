package plugin

type EventType string

const (
	EventPlayerJoin         EventType = "player.join"
	EventPlayerLeave       EventType = "player.leave"
	EventPlayerChat        EventType = "player.chat"
	EventPlayerMove        EventType = "player.move"
	EventPlayerInteract    EventType = "player.interact"
	EventBlockBreak        EventType = "block.break"
	EventBlockPlace        EventType = "block.place"
	EventServerStart       EventType = "server.start"
	EventServerStop        EventType = "server.stop"
)

type Event interface {
	Type() EventType
}

type BaseEvent struct {
	Type_ EventType
}

func (e *BaseEvent) Type() EventType {
	return e.Type_
}

type PlayerJoinEvent struct {
	BaseEvent
	PlayerName string
}

type PlayerLeaveEvent struct {
	BaseEvent
	PlayerName string
	Reason     string
}

type PlayerChatEvent struct {
	BaseEvent
	PlayerName string
	Message    string
}

type BlockBreakEvent struct {
	BaseEvent
	PlayerName string
	X, Y, Z    int
	BlockID    int
}

type BlockPlaceEvent struct {
	BaseEvent
	PlayerName string
	X, Y, Z    int
	BlockID    int
}

type ServerStartEvent struct {
	BaseEvent
}

type ServerStopEvent struct {
	BaseEvent
}