package player

import (
	"sync"

	"github.com/google/uuid"
	"livingworld/internal/world"
)

type Manager struct {
	mu      sync.RWMutex
	players map[uuid.UUID]*Player

	subMu       sync.RWMutex
	subscribers map[string]chan Event
}

func NewManager() *Manager {
	return &Manager{
		players:     make(map[uuid.UUID]*Player),
		subscribers: make(map[string]chan Event),
	}
}

func (m *Manager) AddPlayer(p *Player) {
	m.mu.Lock()
	m.players[p.UUID] = p
	m.mu.Unlock()
	m.publish(Event{Type: EventJoin, Player: p.Snapshot()})
}

func (m *Manager) RemovePlayer(id uuid.UUID) {
	m.mu.Lock()
	p := m.players[id]
	delete(m.players, id)
	m.mu.Unlock()
	if p != nil {
		m.publish(Event{Type: EventLeave, Player: p.Snapshot()})
	}
}

func (m *Manager) GetPlayer(id uuid.UUID) *Player {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.players[id]
}

func (m *Manager) GetPlayerByName(name string) *Player {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.players {
		if p.Username == name {
			return p
		}
	}
	return nil
}

func (m *Manager) GetAllPlayers() []*Player {
	m.mu.RLock()
	defer m.mu.RUnlock()
	players := make([]*Player, 0, len(m.players))
	for _, p := range m.players {
		players = append(players, p)
	}
	return players
}

func (m *Manager) PlayerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.players)
}

func (m *Manager) BroadcastChat(message string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.players {
		p.SendMessage(message)
	}
}

func (m *Manager) UpdatePosition(id uuid.UUID, x, y, z float64, pitch, yaw float32, onGround bool) {
	m.mu.Lock()
	p := m.players[id]
	if p != nil {
		p.Position = world.Position{X: x, Y: y, Z: z}
		p.Rotation = world.Rotation{Pitch: pitch, Yaw: yaw}
		p.OnGround = onGround
	}
	m.mu.Unlock()
	if p != nil {
		m.publish(Event{Type: EventMove, Player: p.Snapshot()})
	}
}

func (m *Manager) Subscribe(id string, buffer int) <-chan Event {
	if buffer <= 0 {
		buffer = 64
	}
	ch := make(chan Event, buffer)
	m.subMu.Lock()
	m.subscribers[id] = ch
	m.subMu.Unlock()
	return ch
}

func (m *Manager) Unsubscribe(id string) {
	m.subMu.Lock()
	if ch, ok := m.subscribers[id]; ok {
		delete(m.subscribers, id)
		close(ch)
	}
	m.subMu.Unlock()
}

func (m *Manager) publish(ev Event) {
	m.subMu.RLock()
	defer m.subMu.RUnlock()
	for _, ch := range m.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}

type EventType string

const (
	EventJoin  EventType = "join"
	EventMove  EventType = "move"
	EventLeave EventType = "leave"
)

type Event struct {
	Type   EventType
	Player PlayerSnapshot
}

type Edition string

const (
	EditionJava    Edition = "java"
	EditionBedrock Edition = "bedrock"
)

type PlayerSnapshot struct {
	UUID            uuid.UUID
	Username        string
	Edition         Edition
	EntityRuntimeID uint64
	Position        world.Position
	Rotation        world.Rotation
	OnGround        bool
}

type Player struct {
	UUID            uuid.UUID
	Username        string
	Edition         Edition
	XUID            uint64
	EntityRuntimeID uint64
	World           *world.World
	Position        world.Position
	Rotation        world.Rotation
	OnGround        bool
	Health          float32
	Food            int
	Saturation      float32
	Inventory       *Inventory
	Creative        bool
	Op              bool
	Flying          bool
	Skin            *SkinData
}

func NewPlayer(uuid_ uuid.UUID, username string, edition Edition) *Player {
	return &Player{
		UUID:       uuid_,
		Username:   username,
		Edition:    edition,
		Health:     20,
		Food:       20,
		Saturation: 5,
		Inventory:  NewInventory(),
		Position:   world.Position{X: 0, Y: 64, Z: 0},
		Rotation:   world.Rotation{Pitch: 0, Yaw: 0},
		OnGround:   true,
	}
}

func (p *Player) Snapshot() PlayerSnapshot {
	return PlayerSnapshot{
		UUID:            p.UUID,
		Username:        p.Username,
		Edition:         p.Edition,
		EntityRuntimeID: p.EntityRuntimeID,
		Position:        p.Position,
		Rotation:        p.Rotation,
		OnGround:        p.OnGround,
	}
}

func (p *Player) Teleport(x, y, z float64) {
	p.Position = world.Position{X: x, Y: y, Z: z}
}

func (p *Player) SetRotation(pitch, yaw float32) {
	p.Rotation = world.Rotation{Pitch: pitch, Yaw: yaw}
}

func (p *Player) Damage(amount float32) {
	p.Health -= amount
	if p.Health < 0 {
		p.Health = 0
	}
}

func (p *Player) Heal(amount float32) {
	p.Health += amount
	if p.Health > 20 {
		p.Health = 20
	}
}

func (p *Player) SendMessage(message string)       {}
func (p *Player) SendTitle(title, subtitle string) {}
func (p *Player) Kick(reason string)               {}
