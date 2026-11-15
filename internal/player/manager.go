package player

import (
	"sync"

	"livingworld/internal/shared/constants/gameplay"

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

// EnvironmentController is an optional extension implemented by sessions that
// push local player metadata (air supply bubbles, on-fire flag, swimming pose)
// back to the owning client.
type EnvironmentController interface {
	SyncEnvironment(snap PlayerSnapshot)
}

// HungerController is an optional extension implemented by protocol sessions
// that push food, saturation, and exhaustion updates to the client HUD.
type HungerController interface {
	SyncHunger(food int, saturation, exhaustion float32)
}

type Manager struct {
	mu      sync.RWMutex
	players map[uuid.UUID]*Player

	subMu       sync.RWMutex
	subscribers map[string]chan Event

	ctrlMu      sync.RWMutex
	controllers map[uuid.UUID]Controller

	// M6: bus publishes EffectStatus / EffectStatusRemove world events
	// to the bridges when effects are added/expired. The pointer is
	// read under busMu so a late SetEffectBus call from server.go
	// never races a TickEffects in flight.
	busMu     sync.RWMutex
	effectBus effectBus

	dataMu  sync.RWMutex
	dataDir string // player-data directory; empty disables persistence

	// Hunger hooks wired by the server so movement, jumping, combat, and
	// status effects feed into the world's HungerTracker.
	exhaustionFn func(id uuid.UUID, amount float32)
	canSprintFn  func(id uuid.UUID) bool
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

func (m *Manager) syncEnvironment(snap PlayerSnapshot) {
	m.ctrlMu.RLock()
	c := m.controllers[snap.UUID]
	m.ctrlMu.RUnlock()
	if ec, ok := c.(EnvironmentController); ok {
		ec.SyncEnvironment(snap)
	}
}

// SetHungerHooks wires callbacks into the world's HungerTracker so movement,
// jumping, combat, damage, and EffectHunger can add exhaustion and enforce the
// food > 6 sprint gate.
func (m *Manager) SetHungerHooks(addExhaustion func(uuid.UUID, float32), canSprint func(uuid.UUID) bool) {
	m.mu.Lock()
	m.exhaustionFn = addExhaustion
	m.canSprintFn = canSprint
	m.mu.Unlock()
}

// AddExhaustion forwards an exhaustion increment to the registered hunger hook
// if the player is in survival/adventure mode and alive.
func (m *Manager) AddExhaustion(id uuid.UUID, amount float32) {
	if amount <= 0 {
		return
	}
	m.mu.RLock()
	p := m.players[id]
	fn := m.exhaustionFn
	canApply := p != nil && p.Health > 0 && !p.Creative && p.Gamemode != 1 && p.Gamemode != 3
	m.mu.RUnlock()
	if canApply && fn != nil {
		fn(id, amount)
	}
}

// SyncHunger updates the authoritative Player food/saturation fields, enforces
// stopping sprint when food drops to <= 6, and notifies the session's
// HungerController to push HUD updates.
func (m *Manager) SyncHunger(id uuid.UUID, food int, saturation, exhaustion float32) {
	m.mu.Lock()
	p := m.players[id]
	sprintStopped := false
	var snap PlayerSnapshot
	if p != nil {
		p.Food = food
		p.Saturation = saturation
		if p.Sprinting && !p.Creative && p.Gamemode != 1 && p.Gamemode != 3 && food <= 6 {
			p.Sprinting = false
			p.Swimming = false
			sprintStopped = true
			snap = p.Snapshot()
		}
	}
	m.mu.Unlock()

	if sprintStopped {
		m.publish(Event{Type: EventSneak, Player: snap})
		m.syncEnvironment(snap)
	}

	m.ctrlMu.RLock()
	c := m.controllers[id]
	m.ctrlMu.RUnlock()
	if hc, ok := c.(HungerController); ok {
		hc.SyncHunger(food, saturation, exhaustion)
	}
}

// SetHealth synchronizes the authoritative Player.Health in Manager with a
// session-level health update. When health reaches <= 0, active fire/sprint/swim
// flags are cleared and death events are broadcast so all viewers play the death
// animation and despawn the corpse.
func (m *Manager) SetHealth(id uuid.UUID, health float32) {
	m.mu.Lock()
	p := m.players[id]
	var snap PlayerSnapshot
	metaChanged := false
	justDied := false
	if p != nil {
		wasAlive := p.Health > 0
		if health < 0 {
			health = 0
		} else if health > gameplay.MaxHealth {
			health = gameplay.MaxHealth
		}
		p.Health = health
		if health <= 0 {
			if wasAlive {
				justDied = true
				metaChanged = true
			}
			if p.OnFire || p.FireTicks > 0 || p.Sprinting || p.Swimming || p.Sneaking {
				p.OnFire = false
				p.FireTicks = 0
				p.LavaTickCooldown = 0
				p.Sprinting = false
				p.Swimming = false
				p.Sneaking = false
				metaChanged = true
			}
			p.FallDistance = 0
			snap = p.Snapshot()
		}
	}
	m.mu.Unlock()
	if justDied {
		m.publish(Event{Type: EventHurt, Player: snap})
	}
	if metaChanged {
		m.publish(Event{Type: EventSneak, Player: snap})
		m.syncEnvironment(snap)
	}
}

// RespawnPlayer resets a dead player's authoritative state in Manager (full HP,
// full food/saturation, full air supply, cleared fire/movement flags and status
// effects) at the given spawn coordinates.
func (m *Manager) RespawnPlayer(id uuid.UUID, x, y, z float64, pitch, yaw float32) PlayerSnapshot {
	m.ClearEffects(id)
	m.mu.Lock()
	p := m.players[id]
	var snap PlayerSnapshot
	if p != nil {
		p.Health = gameplay.MaxHealth
		p.Food = 20
		p.Saturation = 5.0
		p.AirSupply = 300
		p.Breathing = true
		p.OnFire = false
		p.FireTicks = 0
		p.LavaTickCooldown = 0
		p.Sneaking = false
		p.Sprinting = false
		p.Swimming = false
		p.Position.X = x
		p.Position.Y = y
		p.Position.Z = z
		p.Rotation.Pitch = pitch
		p.Rotation.Yaw = yaw
		p.OnGround = true
		snap = p.Snapshot()
	}
	m.mu.Unlock()
	if snap.UUID != (uuid.UUID{}) {
		m.publish(Event{Type: EventMove, Player: snap, Teleport: true})
		m.publish(Event{Type: EventSneak, Player: snap})
		m.syncEnvironment(snap)
	}
	return snap
}
