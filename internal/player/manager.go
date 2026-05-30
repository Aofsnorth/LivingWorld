package player

import (
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"livingworld/internal/world"
)

// Controller is implemented by a protocol session so the shared player model can
// act on a connected client (send chat, disconnect, ...) without the player
// package depending on any protocol code.
type Controller interface {
	SendMessage(msg string)
	Kick(reason string)
	// Push applies a velocity impulse to the player's own entity, in blocks/tick
	// (the shared Minecraft velocity unit). Implementations convert to their
	// edition's wire format (Java short = blocks/tick*8000, Bedrock mgl32.Vec3).
	Push(vx, vy, vz float64)
}

type Manager struct {
	mu      sync.RWMutex
	players map[uuid.UUID]*Player

	subMu       sync.RWMutex
	subscribers map[string]chan Event

	ctrlMu      sync.RWMutex
	controllers map[uuid.UUID]Controller
}

func NewManager() *Manager {
	return &Manager{
		players:     make(map[uuid.UUID]*Player),
		subscribers: make(map[string]chan Event),
		controllers: make(map[uuid.UUID]Controller),
	}
}

// SetController registers the live session that can act on a player.
func (m *Manager) SetController(id uuid.UUID, c Controller) {
	m.ctrlMu.Lock()
	m.controllers[id] = c
	m.ctrlMu.Unlock()
}

// RemoveController drops a player's session (on disconnect).
func (m *Manager) RemoveController(id uuid.UUID) {
	m.ctrlMu.Lock()
	delete(m.controllers, id)
	m.ctrlMu.Unlock()
}

// Message sends a chat message to a single player if connected.
func (m *Manager) Message(id uuid.UUID, msg string) {
	m.ctrlMu.RLock()
	c := m.controllers[id]
	m.ctrlMu.RUnlock()
	if c != nil {
		c.SendMessage(msg)
	}
}

// Broadcast sends a chat message to every connected player.
func (m *Manager) Broadcast(msg string) {
	m.ctrlMu.RLock()
	ctrls := make([]Controller, 0, len(m.controllers))
	for _, c := range m.controllers {
		ctrls = append(ctrls, c)
	}
	m.ctrlMu.RUnlock()
	for _, c := range ctrls {
		c.SendMessage(msg)
	}
}

// Kick disconnects a player if connected.
func (m *Manager) Kick(id uuid.UUID, reason string) {
	m.ctrlMu.RLock()
	c := m.controllers[id]
	m.ctrlMu.RUnlock()
	if c != nil {
		c.Kick(reason)
	}
}

func (m *Manager) push(id uuid.UUID, vx, vy, vz float64) {
	m.ctrlMu.RLock()
	c := m.controllers[id]
	m.ctrlMu.RUnlock()
	if c != nil {
		c.Push(vx, vy, vz)
	}
}

// Player-push tuning. Values are in blocks; velocity is blocks/tick.
const (
	pushTickHz     = 10  // pushes/second
	pushRadius     = 0.6 // horizontal center distance under which players push apart
	pushVertical   = 1.8 // only push when vertical spans overlap (player height)
	pushStrength   = 0.05
	pushMaxPerTick = 0.4 // clamp so deeply-overlapped players don't launch
)

// StartPushLoop runs the cross-edition player-push loop until Close. Players of
// both editions push each other apart (Java's player-vs-player push is
// server-authoritative, so it must be driven here rather than client-side).
func (m *Manager) StartPushLoop() {
	go func() {
		ticker := time.NewTicker(time.Second / pushTickHz)
		defer ticker.Stop()
		for range ticker.C {
			m.pushTick()
		}
	}()
}

func (m *Manager) pushTick() {
	players := m.GetAllPlayers()
	for i := 0; i < len(players); i++ {
		for j := i + 1; j < len(players); j++ {
			a, b := players[i], players[j]
			if a.EntityRuntimeID == 0 || b.EntityRuntimeID == 0 {
				continue
			}
			if math.Abs(b.Position.Y-a.Position.Y) >= pushVertical {
				continue
			}
			dx := b.Position.X - a.Position.X
			dz := b.Position.Z - a.Position.Z
			distSq := dx*dx + dz*dz
			if distSq >= pushRadius*pushRadius || distSq < 1e-6 {
				continue
			}
			dist := math.Sqrt(distSq)
			f := pushStrength * (1.0 - dist/pushRadius)
			if f <= 0 {
				continue
			}
			vx := clampF((dx/dist)*f, -pushMaxPerTick, pushMaxPerTick)
			vz := clampF((dz/dist)*f, -pushMaxPerTick, pushMaxPerTick)
			m.push(a.UUID, -vx, 0, -vz)
			m.push(b.UUID, vx, 0, vz)
		}
	}
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m *Manager) AddPlayer(p *Player) {
	m.mu.Lock()
	m.players[p.UUID] = p
	m.mu.Unlock()
	m.publish(Event{Type: EventJoin, Player: p.Snapshot()})
	m.Broadcast(p.Username + " joined the game")
}

func (m *Manager) RemovePlayer(id uuid.UUID) {
	m.mu.Lock()
	p := m.players[id]
	delete(m.players, id)
	m.mu.Unlock()
	if p != nil {
		m.publish(Event{Type: EventLeave, Player: p.Snapshot()})
		m.Broadcast(p.Username + " left the game")
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
	m.Broadcast(message)
}

func (m *Manager) UpdatePosition(id uuid.UUID, x, y, z float64, pitch, yaw float32, onGround bool) {
	m.mu.Lock()
	p := m.players[id]
	isTeleport := false
	if p != nil {
		dx := x - p.Position.X
		dy := y - p.Position.Y
		dz := z - p.Position.Z
		distSq := dx*dx + dy*dy + dz*dz
		// If movement is larger than 4 blocks (~16 squared), treat it as a teleport.
		if distSq > 16.0 {
			isTeleport = true
		}
		p.Position = world.Position{X: x, Y: y, Z: z}
		p.Rotation = world.Rotation{Pitch: pitch, Yaw: yaw}
		p.OnGround = onGround
	}
	m.mu.Unlock()
	if p != nil {
		m.publish(Event{Type: EventMove, Player: p.Snapshot(), Teleport: isTeleport})
	}
}

func (m *Manager) UpdateSneak(id uuid.UUID, sneaking bool) {
	m.mu.Lock()
	p := m.players[id]
	changed := false
	if p != nil {
		if p.Sneaking != sneaking {
			p.Sneaking = sneaking
			changed = true
		}
	}
	m.mu.Unlock()
	if changed && p != nil {
		m.publish(Event{Type: EventSneak, Player: p.Snapshot()})
	}
}

func (m *Manager) UpdateSkinParts(id uuid.UUID, parts byte) {
	m.mu.Lock()
	p := m.players[id]
	changed := false
	if p != nil {
		if p.SkinParts != parts {
			p.SkinParts = parts
			changed = true
		}
	}
	m.mu.Unlock()
	if changed && p != nil {
		m.publish(Event{Type: EventSkin, Player: p.Snapshot()})
	}
}

func (m *Manager) UpdateProfileProperty(id uuid.UUID, name, value, signature string) {
	m.mu.Lock()
	p := m.players[id]
	changed := false
	if p != nil {
		found := false
		for i, prop := range p.ProfileProperties {
			if prop.Name == name {
				if p.ProfileProperties[i].Value != value || p.ProfileProperties[i].Signature != signature {
					p.ProfileProperties[i].Value = value
					p.ProfileProperties[i].Signature = signature
					changed = true
				}
				found = true
				break
			}
		}
		if !found {
			p.ProfileProperties = append(p.ProfileProperties, ProfileProperty{Name: name, Value: value, Signature: signature})
			changed = true
		}
	}
	m.mu.Unlock()
	if changed && p != nil {
		m.publish(Event{Type: EventSkin, Player: p.Snapshot()})
	}
}

func (m *Manager) PublishSwing(id uuid.UUID) {
	m.mu.RLock()
	p := m.players[id]
	m.mu.RUnlock()
	if p != nil {
		m.publish(Event{Type: EventSwing, Player: p.Snapshot()})
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
	EventSwing EventType = "swing"
	EventSneak EventType = "sneak"
	EventSkin  EventType = "skin"
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
		SkinParts:  0x7F,
	}
}

func (p *Player) Snapshot() PlayerSnapshot {
	return PlayerSnapshot{
		UUID:              p.UUID,
		Username:          p.Username,
		Edition:           p.Edition,
		EntityRuntimeID:   p.EntityRuntimeID,
		Position:          p.Position,
		Rotation:          p.Rotation,
		OnGround:          p.OnGround,
		Sneaking:          p.Sneaking,
		ProfileProperties: append([]ProfileProperty(nil), p.ProfileProperties...),
		BedrockSkinURL:    p.BedrockSkinURL,
		Skin:              p.Skin,
		SkinParts:         p.SkinParts,
		Creative:          p.Creative,
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
func (p *Player) Push(vx, vy, vz float64)          {}
