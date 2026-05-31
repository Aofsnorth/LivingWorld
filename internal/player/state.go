package player

import (
	"livingworld/internal/shared/constants/gameplay"
	"livingworld/internal/world"

	"github.com/google/uuid"
)

func (m *Manager) UpdatePosition(id uuid.UUID, x, y, z float64, pitch, yaw float32, onGround bool) {
	m.mu.Lock()
	p := m.players[id]
	isTeleport := false
	if p != nil {
		dx := x - p.Position.X
		dy := y - p.Position.Y
		dz := z - p.Position.Z
		distSq := dx*dx + dy*dy + dz*dz
		// If movement is larger than threshold, treat it as a teleport.
		if distSq > gameplay.TeleportDistanceSquared {
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

// UpdateHeldSlot updates a player's held hotbar slot and publishes equipment change event.
func (m *Manager) UpdateHeldSlot(id uuid.UUID, slot int) {
	m.mu.Lock()
	p := m.players[id]
	changed := false
	if p != nil && p.HeldItemSlot != slot && slot >= 0 && slot < HotbarSize {
		p.HeldItemSlot = slot
		if p.Inventory != nil {
			p.Inventory.HeldSlot = slot
		}
		changed = true
	}
	m.mu.Unlock()
	if changed && p != nil {
		m.publish(Event{Type: EventEquipment, Player: p.Snapshot()})
	}
}

// PublishEquipmentChange broadcasts equipment change (for pickup, inventory changes).
func (m *Manager) PublishEquipmentChange(id uuid.UUID) {
	m.mu.RLock()
	p := m.players[id]
	m.mu.RUnlock()
	if p != nil {
		m.publish(Event{Type: EventEquipment, Player: p.Snapshot()})
	}
}

func (m *Manager) Subscribe(id string, buffer int) <-chan Event {
	if buffer <= 0 {
		buffer = defaultEventBuffer
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
