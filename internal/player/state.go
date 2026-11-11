package player

import (
	"math"

	"livingworld/internal/shared/constants/gameplay"
	"livingworld/internal/world"

	"github.com/google/uuid"
)

// isPlayerInWater checks whether any part of a player's bounding box (width 0.6,
// height 1.8 / swimming height 0.6) intersects a water block. Checking the AABB
// corners in addition to the center prevents false "left water" transitions when
// swimming along block edges or near the water surface.
func isPlayerInWater(w *world.World, x, y, z float64) bool {
	if w == nil {
		return false
	}
	yOffsets := [4]float64{0.0, 0.4, 1.2, 1.62}
	xzOffsets := [5][2]float64{
		{0, 0},
		{-0.29, -0.29},
		{-0.29, 0.29},
		{0.29, -0.29},
		{0.29, 0.29},
	}
	for _, yo := range yOffsets {
		by := int(math.Floor(y + yo))
		for _, xzo := range xzOffsets {
			bx := int(math.Floor(x + xzo[0]))
			bz := int(math.Floor(z + xzo[1]))
			if world.IsWaterBlockID(w.GetBlock(bx, by, bz).ID()) {
				return true
			}
		}
	}
	return false
}

func (m *Manager) UpdatePosition(id uuid.UUID, x, y, z float64, pitch, yaw float32, onGround bool) {
	m.mu.Lock()
	p := m.players[id]
	isTeleport := false
	swimChanged := false
	var snap PlayerSnapshot
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
		if p.World != nil {
			inWater := isPlayerInWater(p.World, x, y, z)
			if !inWater && p.Swimming {
				p.Swimming = false
				swimChanged = true
			} else if inWater && p.Sprinting && !p.Swimming {
				p.Swimming = true
				swimChanged = true
			}
		}
		snap = p.Snapshot()
	}
	m.mu.Unlock()
	if p != nil {
		m.publish(Event{Type: EventMove, Player: snap, Teleport: isTeleport})
		if swimChanged {
			m.publish(Event{Type: EventSneak, Player: snap})
			m.syncEnvironment(snap)
		}
	}
}

func (m *Manager) UpdateSneak(id uuid.UUID, sneaking bool) {
	m.mu.Lock()
	p := m.players[id]
	var snap PlayerSnapshot
	changed := false
	if p != nil {
		if p.Sneaking != sneaking {
			p.Sneaking = sneaking
			changed = true
			snap = p.Snapshot()
		}
	}
	m.mu.Unlock()
	if changed {
		m.publish(Event{Type: EventSneak, Player: snap})
		m.syncEnvironment(snap)
	}
}

func (m *Manager) UpdateSprinting(id uuid.UUID, sprinting bool) {
	m.mu.Lock()
	p := m.players[id]
	var snap PlayerSnapshot
	changed := false
	if p != nil {
		inWater := isPlayerInWater(p.World, p.Position.X, p.Position.Y, p.Position.Z)
		if p.Sprinting != sprinting {
			p.Sprinting = sprinting
			changed = true
		}
		wantSwim := sprinting && inWater
		if p.Swimming != wantSwim {
			p.Swimming = wantSwim
			changed = true
		}
		if changed {
			snap = p.Snapshot()
		}
	}
	m.mu.Unlock()
	if changed {
		m.publish(Event{Type: EventSneak, Player: snap})
		m.syncEnvironment(snap)
	}
}

func (m *Manager) UpdateSwimming(id uuid.UUID, swimming bool) {
	m.mu.Lock()
	p := m.players[id]
	var snap PlayerSnapshot
	changed := false
	if p != nil {
		if swimming && p.World != nil {
			if !isPlayerInWater(p.World, p.Position.X, p.Position.Y, p.Position.Z) {
				swimming = false
			}
		}
		if p.Swimming != swimming {
			p.Swimming = swimming
			changed = true
			snap = p.Snapshot()
		}
	}
	m.mu.Unlock()
	if changed {
		m.publish(Event{Type: EventSneak, Player: snap})
		m.syncEnvironment(snap)
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

// UpdateBedrockSkin updates a Bedrock player's normalized skin data and local
// skinbridge URL, clears any stale texture property so the updated skin takes
// effect immediately, and broadcasts EventSkin to all viewers.
func (m *Manager) UpdateBedrockSkin(id uuid.UUID, skinURL string, skinData *SkinData) {
	m.mu.Lock()
	p := m.players[id]
	if p != nil {
		p.BedrockSkinURL = skinURL
		p.Skin = skinData
		filtered := p.ProfileProperties[:0]
		for _, prop := range p.ProfileProperties {
			if prop.Name != "textures" {
				filtered = append(filtered, prop)
			}
		}
		p.ProfileProperties = filtered
	}
	m.mu.Unlock()
	if p != nil {
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
