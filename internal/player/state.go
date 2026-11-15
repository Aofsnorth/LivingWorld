package player

import (
	"math"
	"time"

	"livingworld/internal/shared/constants/gameplay"
	"livingworld/internal/world"

	"github.com/google/uuid"
)

// isWaterAtOffset checks whether the player's horizontal footprint (width 0.6)
// intersects a water block at y + yOffset.
func isWaterAtOffset(w *world.World, x, y, z, yOffset float64) bool {
	if w == nil {
		return false
	}
	xzOffsets := [5][2]float64{
		{0, 0},
		{-0.29, -0.29},
		{-0.29, 0.29},
		{0.29, -0.29},
		{0.29, 0.29},
	}
	by := int(math.Floor(y + yOffset))
	for _, xzo := range xzOffsets {
		bx := int(math.Floor(x + xzo[0]))
		bz := int(math.Floor(z + xzo[1]))
		if world.IsWaterBlockID(w.GetBlock(bx, by, bz).ID()) {
			return true
		}
	}
	return false
}

// isPlayerInWater checks whether any part of a player's bounding box (width 0.6,
// height 1.8 / swimming height 0.6) intersects a water block.
func isPlayerInWater(w *world.World, x, y, z float64) bool {
	if w == nil {
		return false
	}
	for _, yo := range [4]float64{0.0, 0.4, 1.2, 1.62} {
		if isWaterAtOffset(w, x, y, z, yo) {
			return true
		}
	}
	return false
}

// canStartSwimming reports whether a non-swimming player is submerged deeply
// enough to enter horizontal swim mode when sprinting. Both lower body (y+0.4)
// and standing eye level (y+1.62) must be in water; floating on the water
// surface or wading in 1-block shallow water must NOT enter swim mode.
func canStartSwimming(w *world.World, x, y, z float64) bool {
	return isWaterAtOffset(w, x, y, z, 0.4) && isWaterAtOffset(w, x, y, z, 1.62)
}

// canKeepSwimming reports whether an already-swimming player (height 0.6,
// swimming eye/body level at y+0.4) remains submerged enough to stay in swim
// mode. Once the player floats onto the top of the water surface so y+0.4 is in
// air (even if feet y+0.0 still touch water), swimming ends.
func canKeepSwimming(w *world.World, x, y, z float64) bool {
	return isWaterAtOffset(w, x, y, z, 0.4)
}

// resolveSwimState computes the authoritative swimming state for a player.
func resolveSwimState(w *world.World, x, y, z float64, sprinting, currentlySwimming bool) bool {
	if !sprinting || w == nil {
		return false
	}
	if currentlySwimming {
		return canKeepSwimming(w, x, y, z)
	}
	return canStartSwimming(w, x, y, z)
}

const minCritFallDistance = 0.08

// CanCrit reports whether the player currently satisfies Vanilla 1.20 critical
// hit conditions: airborne descending past the jump apex (or within the short
// packet-batch grace window where movement lands in the same frame as the
// attack packet), not sprinting, not swimming, not flying, not in water/lava/
// climbable, and not blinded.
func (p *Player) CanCrit() bool {
	if p == nil || p.Health <= 0 || p.Sprinting || p.Swimming || p.Flying || p.Creative || p.Gamemode == 1 || p.Gamemode == 3 {
		return false
	}
	if p.effects.has(EffectBlindness) {
		return false
	}
	airborneCrit := !p.OnGround && p.FallDistance >= minCritFallDistance
	recentLandingCrit := p.OnGround && !p.LastAirborneAt.IsZero() && time.Since(p.LastAirborneAt) <= 150*time.Millisecond && p.LastFallDistance >= minCritFallDistance
	if !airborneCrit && !recentLandingCrit {
		return false
	}
	if p.World != nil {
		if isPlayerInWater(p.World, p.Position.X, p.Position.Y, p.Position.Z) {
			return false
		}
		bx, by, bz := int(math.Floor(p.Position.X)), int(math.Floor(p.Position.Y)), int(math.Floor(p.Position.Z))
		name := world.StateName(p.World.GetBlock(bx, by, bz).ID())
		switch name {
		case "minecraft:lava", "minecraft:ladder", "minecraft:vine", "minecraft:scaffolding",
			"minecraft:cobweb", "minecraft:powder_snow", "minecraft:sweet_berry_bush",
			"minecraft:weeping_vines", "minecraft:weeping_vines_plant",
			"minecraft:twisting_vines", "minecraft:twisting_vines_plant",
			"minecraft:cave_vines", "minecraft:cave_vines_plant":
			return false
		}
	}
	return true
}

// ConsumeCrit checks whether the player can land a critical hit right now and,
// if so, consumes the pending fall-crit state so the same jump cannot trigger
// multiple critical hits without descending again.
func (m *Manager) ConsumeCrit(id uuid.UUID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.players[id]
	if p == nil || !p.CanCrit() {
		return false
	}
	p.FallDistance = 0
	p.LastFallDistance = 0
	p.LastAirborneAt = time.Time{}
	return true
}

func (m *Manager) UpdatePosition(id uuid.UUID, x, y, z float64, pitch, yaw float32, onGround bool) {
	m.mu.Lock()
	p := m.players[id]
	isTeleport := false
	swimChanged := false
	var exhaustionDelta float32
	var exhaustionFn func(uuid.UUID, float32)
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
		if !isTeleport && p.Health > 0 && !p.Creative && p.Gamemode != 1 && p.Gamemode != 3 {
			if p.Sprinting && p.Food <= 6 && (m.canSprintFn == nil || !m.canSprintFn(id)) {
				p.Sprinting = false
				swimChanged = true
			}
			horizDist := math.Sqrt(dx*dx + dz*dz)
			if p.Swimming {
				dist3D := math.Sqrt(distSq)
				if dist3D > 0.001 {
					exhaustionDelta = 0.01 * float32(dist3D)
				}
			} else if p.Sprinting && horizDist > 0.001 {
				exhaustionDelta = 0.1 * float32(horizDist)
			}
			exhaustionFn = m.exhaustionFn
		}
		prevOnGround := p.OnGround
		p.Position = world.Position{X: x, Y: y, Z: z}
		p.Rotation = world.Rotation{Pitch: pitch, Yaw: yaw}
		p.OnGround = onGround
		now := time.Now()
		if isTeleport || p.Swimming || p.Flying || p.Health <= 0 {
			p.FallDistance = 0
			p.LastFallDistance = 0
			p.LastAirborneAt = time.Time{}
		} else if onGround {
			if !prevOnGround && p.FallDistance >= minCritFallDistance {
				p.LastFallDistance = p.FallDistance
				p.LastAirborneAt = now
			} else if prevOnGround && !p.LastAirborneAt.IsZero() && now.Sub(p.LastAirborneAt) > 150*time.Millisecond {
				p.LastFallDistance = 0
				p.LastAirborneAt = time.Time{}
			}
			p.FallDistance = 0
		} else {
			if dy > 0.01 {
				p.FallDistance = 0
				p.LastFallDistance = 0
				p.LastAirborneAt = time.Time{}
			} else if dy < -0.001 {
				p.FallDistance += -dy
				if p.FallDistance >= minCritFallDistance {
					p.LastFallDistance = p.FallDistance
					p.LastAirborneAt = now
				}
			}
		}
		if p.World != nil && p.Health > 0 {
			if isPlayerInWater(p.World, x, y, z) {
				p.FallDistance = 0
				p.LastFallDistance = 0
				p.LastAirborneAt = time.Time{}
			}
			wantSwim := resolveSwimState(p.World, x, y, z, p.Sprinting, p.Swimming)
			if p.Swimming != wantSwim {
				p.Swimming = wantSwim
				swimChanged = true
			}
		}
		snap = p.Snapshot()
	}
	m.mu.Unlock()
	if exhaustionDelta > 0 && exhaustionFn != nil {
		exhaustionFn(id, exhaustionDelta)
	}
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
	resyncClient := false
	if p != nil {
		if sprinting {
			if p.Health <= 0 {
				sprinting = false
				resyncClient = true
			} else if !p.Creative && p.Gamemode != 1 && p.Gamemode != 3 {
				canSprint := p.Food > 6
				if m.canSprintFn != nil {
					canSprint = m.canSprintFn(id)
				}
				if !canSprint {
					sprinting = false
					resyncClient = true
				}
			}
		}
		if p.Sprinting != sprinting {
			p.Sprinting = sprinting
			changed = true
		}
		wantSwim := resolveSwimState(p.World, p.Position.X, p.Position.Y, p.Position.Z, sprinting, p.Swimming)
		if p.Swimming != wantSwim {
			p.Swimming = wantSwim
			changed = true
		}
		if changed || resyncClient {
			snap = p.Snapshot()
		}
	}
	m.mu.Unlock()
	if changed {
		m.publish(Event{Type: EventSneak, Player: snap})
		m.syncEnvironment(snap)
	} else if resyncClient {
		m.syncEnvironment(snap)
	}
}

// AddJumpExhaustion applies vanilla jump exhaustion (0.2 when sprint-jumping,
// 0.05 for a regular jump).
func (m *Manager) AddJumpExhaustion(id uuid.UUID) {
	m.mu.RLock()
	p := m.players[id]
	fn := m.exhaustionFn
	var amount float32
	if p != nil && p.Health > 0 && !p.Creative && p.Gamemode != 1 && p.Gamemode != 3 {
		if p.Sprinting {
			amount = 0.2
		} else {
			amount = 0.05
		}
	}
	m.mu.RUnlock()
	if amount > 0 && fn != nil {
		fn(id, amount)
	}
}

// SyncFallState updates a player's authoritative OnGround and FallDistance
// immediately (even on movement frames whose position broadcast is rate-limited)
// so combat critical-hit checks always see the latest airborne descent state.
func (m *Manager) SyncFallState(id uuid.UUID, onGround bool, fallDistance float64) {
	m.mu.Lock()
	if p := m.players[id]; p != nil && p.Health > 0 {
		now := time.Now()
		prevOnGround := p.OnGround
		p.OnGround = onGround
		if p.Swimming || p.Flying {
			p.FallDistance = 0
			p.LastFallDistance = 0
			p.LastAirborneAt = time.Time{}
		} else if onGround {
			if !prevOnGround && p.FallDistance >= minCritFallDistance {
				p.LastFallDistance = p.FallDistance
				p.LastAirborneAt = now
			} else if prevOnGround && !p.LastAirborneAt.IsZero() && now.Sub(p.LastAirborneAt) > 150*time.Millisecond {
				p.LastFallDistance = 0
				p.LastAirborneAt = time.Time{}
			}
			p.FallDistance = 0
		} else {
			if fallDistance <= 0 {
				p.FallDistance = 0
				p.LastFallDistance = 0
				p.LastAirborneAt = time.Time{}
			} else {
				p.FallDistance = fallDistance
				if fallDistance >= minCritFallDistance {
					p.LastFallDistance = fallDistance
					p.LastAirborneAt = now
				}
			}
		}
	}
	m.mu.Unlock()
}

func (m *Manager) UpdateSwimming(id uuid.UUID, swimming bool) {
	m.mu.Lock()
	p := m.players[id]
	var snap PlayerSnapshot
	changed := false
	resyncClient := false
	if p != nil {
		if swimming && p.World != nil {
			if !resolveSwimState(p.World, p.Position.X, p.Position.Y, p.Position.Z, true, p.Swimming) {
				swimming = false
				// Client tried to start swimming while at the water surface or out
				// of deep water; force a metadata resync so Bedrock clears its
				// local swimming flag and restores normal 1.8m hitbox height.
				resyncClient = true
			}
		}
		if p.Swimming != swimming {
			p.Swimming = swimming
			changed = true
		}
		if changed || resyncClient {
			snap = p.Snapshot()
		}
	}
	m.mu.Unlock()
	if changed {
		m.publish(Event{Type: EventSneak, Player: snap})
		m.syncEnvironment(snap)
	} else if resyncClient {
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
