package player

import (
	"sync"

	"github.com/google/uuid"
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
	// Hurt applies melee damage to the player's own client: reduces+syncs its
	// health and plays the hurt feedback (red flash + sound).
	Hurt(amount float32)
}

type Manager struct {
	mu      sync.RWMutex
	players map[uuid.UUID]*Player

	subMu       sync.RWMutex
	subscribers map[string]chan Event

	ctrlMu      sync.RWMutex
	controllers map[uuid.UUID]Controller

	dataMu  sync.RWMutex
	dataDir string // player-data directory; empty disables persistence
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
