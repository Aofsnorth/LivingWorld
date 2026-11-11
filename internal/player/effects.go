// M6: status effect data model + per-player effect bag.
//
// Effects live entirely on the server. The bridges translate adds/removes
// to their edition's wire packet (Java ClientboundUpdateMobEffect /
// ClientboundRemoveMobEffect; Bedrock packet.MobEffect with MobEffectAdd /
// MobEffectRemove). The actual damage (poison 0.5 HP/s, wither 0.5 HP/s,
// etc.) is applied from the world tick so the bridges don't need to be
// aware of per-tick damage math.

package player

import (
	"math"

	"livingworld/internal/world"

	"github.com/google/uuid"
)

// Vanilla effect type ids. The numbering matches the vanilla 1.21 MobEffect
// list (1..27) and is identical between Java 1.21 and Bedrock gophertunnel's
// EffectSpeed..EffectSlowFalling — a happy accident we lean on so a single
// id travels through both bridges unchanged.
const (
	EffectSpeed         int32 = 1
	EffectSlowness      int32 = 2
	EffectHaste         int32 = 3
	EffectMiningFatigue int32 = 4
	EffectStrength      int32 = 5
	EffectInstantHealth int32 = 6
	EffectInstantDamage int32 = 7
	EffectJumpBoost     int32 = 8
	EffectNausea        int32 = 9
	EffectRegeneration  int32 = 10
	EffectResistance    int32 = 11
	EffectFireResist    int32 = 12
	EffectWaterBreath   int32 = 13
	EffectInvisibility  int32 = 14
	EffectBlindness     int32 = 15
	EffectNightVision   int32 = 16
	EffectHunger        int32 = 17
	EffectWeakness      int32 = 18
	EffectPoison        int32 = 19
	EffectWither        int32 = 20
)

// Effect is one active status effect on one player.
//
// M6 scope: damage-tick effects (poison, wither, hunger-via-damage), plus
// instant damage (which never enters the bag — see HitEffectIDFor +
// applyInstant in server.go). Effects that have no per-tick impact in M6
// (slowness, regeneration, strength) still occupy a slot so the client
// renders the icon / particle; the v1 damage engine only acts on the
// tickable subset.
type Effect struct {
	Type         int32 // EffectSpeed..EffectWither
	Level        int   // amplifier; 0 = I, 1 = II, ...
	TicksLeft    int   // remaining duration in ticks
	TickInterval int   // ticks between damage applications; 0 = no per-tick damage
	NextTickIn   int   // ticks until next damage tick
}

// EffectIDForHitEffectType maps the mob-def HitEffect.Type string to a
// vanilla effect id. Returns 0 for unknown / empty strings — the caller
// should treat 0 as "no effect to apply".
func EffectIDForHitEffectType(t string) int32 {
	switch t {
	case "speed":
		return EffectSpeed
	case "slowness":
		return EffectSlowness
	case "haste":
		return EffectHaste
	case "mining_fatigue":
		return EffectMiningFatigue
	case "strength":
		return EffectStrength
	case "jump_boost":
		return EffectJumpBoost
	case "nausea":
		return EffectNausea
	case "regeneration":
		return EffectRegeneration
	case "resistance":
		return EffectResistance
	case "fire_resistance":
		return EffectFireResist
	case "water_breathing":
		return EffectWaterBreath
	case "invisibility":
		return EffectInvisibility
	case "blindness":
		return EffectBlindness
	case "night_vision":
		return EffectNightVision
	case "hunger":
		return EffectHunger
	case "weakness":
		return EffectWeakness
	case "poison":
		return EffectPoison
	case "wither":
		return EffectWither
	}
	return 0
}

// effectConfig returns the per-effect damage-tick metadata. Returns
// (interval, per-tick-damage). interval=0 means "no per-tick damage in
// M6 scope" — the effect still occupies a slot so the client renders it.
func effectConfig(typeID int32) (interval int, perTick float32) {
	switch typeID {
	case EffectPoison:
		return 10, 0.5 // 0.5 HP / 0.5s, matching vanilla (1 HP / s reduced by resistance)
	case EffectWither:
		return 20, 0.5 // 0.5 HP / s, matching vanilla
	}
	return 0, 0
}

// effectBag is the per-player active-effect list. Slots are sparse by
// vanilla effect id (1..30). We store at index (id-1) so an EffectSpeed
// occupies slot 0. A 30-slot array is small and avoids map allocations on
// the tick path.
type effectBag struct {
	effects [30]Effect
	active  int // number of slots occupied; helps TickEffects skip empty bags
}

func (b *effectBag) has(typeID int32) bool {
	if typeID < 1 || typeID > 30 {
		return false
	}
	idx := int(typeID - 1)
	e := b.effects[idx]
	return e.Type == typeID && e.TicksLeft > 0
}

func (b *effectBag) add(typeID int32, level, ticks int) {
	if typeID < 1 || typeID > 30 {
		return
	}
	idx := int(typeID - 1)
	interval, _ := effectConfig(typeID)
	b.effects[idx] = Effect{
		Type:         typeID,
		Level:        level,
		TicksLeft:    ticks,
		TickInterval: interval,
		NextTickIn:   interval,
	}
	b.recomputeActive()
}

func (b *effectBag) remove(typeID int32) {
	if typeID < 1 || typeID > 30 {
		return
	}
	idx := int(typeID - 1)
	b.effects[idx] = Effect{}
	b.recomputeActive()
}

func (b *effectBag) recomputeActive() {
	n := 0
	for _, e := range b.effects {
		if e.Type != 0 && e.TicksLeft > 0 {
			n++
		}
	}
	b.active = n
}

// tick advances one 20 Hz tick. Returns a list of (effectId, damage) to
// apply, plus a list of effect ids that just expired. The caller is
// responsible for translating these into bridge-side damage calls and
// effect-remove events. Returning slices keeps the lock scope tight: the
// bag's internal state is mutated here, and the caller walks the slices
// outside the lock.
func (b *effectBag) tick() (damages []effectTickDamage, expired []int32) {
	if b.active == 0 {
		return nil, nil
	}
	for i := range b.effects {
		e := b.effects[i]
		if e.Type == 0 || e.TicksLeft <= 0 {
			continue
		}
		e.TicksLeft--
		if e.TickInterval > 0 {
			e.NextTickIn--
			if e.NextTickIn <= 0 {
				_, dmg := effectConfig(e.Type)
				damages = append(damages, effectTickDamage{Type: e.Type, Damage: dmg})
				e.NextTickIn = e.TickInterval
			}
		}
		if e.TicksLeft <= 0 {
			expired = append(expired, e.Type)
			e = Effect{}
		}
		b.effects[i] = e
	}
	b.recomputeActive()
	return damages, expired
}

type effectTickDamage struct {
	Type   int32
	Damage float32
}

// effectBus publishes EffectStatus / EffectStatusRemove events to the
// world effect bus so bridges can translate them to their edition's
// packet. The player manager is wired with a non-nil bus in
// server/server.go (SetEffectBus); tests can pass nil and skip publish.
type effectBus interface {
	PublishWorldEffect(ev world.WorldEffectEvent)
}

// AddEffect applies (or refreshes) a status effect on a connected player.
// level=0 means I, duration is in ticks. Unknown typeIds are silently
// ignored. If the player is offline (no controller), the effect is
// dropped — effects don't persist across logout in v1.
//
// On success this publishes an EffectStatus event so each connected
// bridge can render the icon and start its per-effect tick on the
// client. The server's own damage ticker is driven separately by the
// world tick (Manager.TickEffects).
func (m *Manager) AddEffect(id uuid.UUID, effectType int32, level, ticks int) {
	if effectType < 1 || effectType > 30 {
		return
	}
	m.mu.Lock()
	p := m.players[id]
	if p == nil {
		m.mu.Unlock()
		return
	}
	p.effects.add(effectType, level, ticks)
	m.mu.Unlock()

	m.busMu.RLock()
	bus := m.effectBus
	m.busMu.RUnlock()
	if bus != nil {
		bus.PublishWorldEffect(world.WorldEffectEvent{
			Kind:     world.EffectStatus,
			Source:   world.BlockUpdateSourceServer,
			Target:   id,
			EffectID: effectType,
			Data:     int32(level),
			Aux:      int32(ticks),
		})
	}
}

// RemoveEffect forcibly clears a status effect (e.g. drinking milk in
// a future patch). Publishes EffectStatusRemove for the bridges.
func (m *Manager) RemoveEffect(id uuid.UUID, effectType int32) {
	if effectType < 1 || effectType > 30 {
		return
	}
	m.mu.Lock()
	p := m.players[id]
	if p == nil {
		m.mu.Unlock()
		return
	}
	p.effects.remove(effectType)
	m.mu.Unlock()

	m.busMu.RLock()
	bus := m.effectBus
	m.busMu.RUnlock()
	if bus != nil {
		bus.PublishWorldEffect(world.WorldEffectEvent{
			Kind:     world.EffectStatusRemove,
			Source:   world.BlockUpdateSourceServer,
			Target:   id,
			EffectID: effectType,
		})
	}
}

// TickEffects is called once per 20 Hz tick by the world tick. It
// iterates every connected player's effect bag, applies per-tick damage
// via the controller, and publishes EffectStatusRemove for any effect
// that just expired.
//
// This is the only place per-tick damage happens; aiHitEffect only
// kicks off AddEffect. The split keeps the mob-melee path free of any
// damage math (the mob just "marks" the effect; the tick engine
// actually dings the player).
func (m *Manager) TickEffects() {
	m.mu.RLock()
	players := make([]*Player, 0, len(m.players))
	for _, p := range m.players {
		players = append(players, p)
	}
	m.mu.RUnlock()

	for _, p := range players {
		damages, expired := p.effects.tick()
		if len(damages) == 0 && len(expired) == 0 {
			continue
		}
		for _, d := range damages {
			if d.Damage > 0 {
				m.applyDamage(p.UUID, d.Damage)
			}
		}
		for _, eid := range expired {
			m.busMu.RLock()
			bus := m.effectBus
			m.busMu.RUnlock()
			if bus != nil {
				bus.PublishWorldEffect(world.WorldEffectEvent{
					Kind:     world.EffectStatusRemove,
					Source:   world.BlockUpdateSourceServer,
					Target:   p.UUID,
					EffectID: eid,
				})
			}
		}
	}
}

// applyDamage routes a per-tick damage event to the player's controller
// (which deals with client-side red flash + HP sync). Falls through to
// the in-memory player model for offline-safe damage accrual — but in
// v1 the player model has no effect tick; controllers are required.
func (m *Manager) applyDamage(id uuid.UUID, amount float32) {
	m.ctrlMu.RLock()
	c := m.controllers[id]
	m.ctrlMu.RUnlock()
	if c == nil {
		return
	}
	c.Hurt(amount)
}

// HitIFrames stamps a fresh invulnerability window on the player.
// Called from the bridge routeAttack path whenever a melee swing
// lands (or would land, after armor). The value passed is
// combat.IFramesTicks (20); a future M-phase can reduce it for
// specific damage types (e.g. fire bypasses I-frames in vanilla).
func (m *Manager) HitIFrames(id uuid.UUID, ticks int) {
	if ticks <= 0 {
		return
	}
	m.mu.Lock()
	if p := m.players[id]; p != nil {
		p.IFrames = ticks
	}
	m.mu.Unlock()
}

// IFramesTick decrements the I-frames counter on every connected
// player. Called once per 20 Hz tick from Phase 4e in the world
// tick. Cheap (one pass over the player map) and never blocks on
// I/O. Players with IFrames == 0 are skipped.
func (m *Manager) IFramesTick() {
	m.mu.RLock()
	players := make([]*Player, 0, len(m.players))
	for _, p := range m.players {
		if p.IFrames > 0 {
			players = append(players, p)
		}
	}
	m.mu.RUnlock()
	if len(players) == 0 {
		return
	}
	m.mu.Lock()
	for _, p := range players {
		p.IFrames--
	}
	m.mu.Unlock()
}

// SetEffectBus wires the world effect bus so AddEffect / TickEffects
// can publish EffectStatus / EffectStatusRemove. Safe to call once at
// server boot; the pointer is read under busMu.
func (m *Manager) SetEffectBus(bus effectBus) {
	m.busMu.Lock()
	m.effectBus = bus
	m.busMu.Unlock()
}

type envTickOutcome struct {
	id          uuid.UUID
	snap        PlayerSnapshot
	metaChanged bool
	damage      float32
}

// TickEnvironment runs at 20 Hz from the world tick to update underwater air
// supply (breathing bubbles & drowning), water swimming state, lava contact
// damage, and fire burn ticks across both Java and Bedrock players.
func (m *Manager) TickEnvironment(defaultWorld *world.World) {
	m.mu.Lock()
	if len(m.players) == 0 {
		m.mu.Unlock()
		return
	}
	outcomes := make([]envTickOutcome, 0, len(m.players))
	for _, p := range m.players {
		w := p.World
		if w == nil {
			w = defaultWorld
		}
		if w == nil {
			continue
		}

		var metaChanged bool
		var dmg float32

		// Creative / Spectator players do not drown or burn.
		if p.Creative || p.Gamemode == 1 || p.Gamemode == 3 {
			if p.AirSupply != 300 || !p.Breathing || p.OnFire || p.FireTicks != 0 {
				p.AirSupply = 300
				p.Breathing = true
				p.OnFire = false
				p.FireTicks = 0
				p.LavaTickCooldown = 0
				metaChanged = true
			}
			inWater := isPlayerInWater(w, p.Position.X, p.Position.Y, p.Position.Z)
			if !inWater && p.Swimming {
				p.Swimming = false
				metaChanged = true
			} else if inWater && p.Sprinting && !p.Swimming {
				p.Swimming = true
				metaChanged = true
			}
			if metaChanged {
				outcomes = append(outcomes, envTickOutcome{
					id:          p.UUID,
					snap:        p.Snapshot(),
					metaChanged: true,
				})
			}
			continue
		}

		if p.AirSupply == 0 && p.Breathing {
			p.AirSupply = 300
		}

		bx := int(math.Floor(p.Position.X))
		bz := int(math.Floor(p.Position.Z))
		feetY := int(math.Floor(p.Position.Y))
		bodyY := int(math.Floor(p.Position.Y + 0.4))
		eyeOffset := 1.62
		if p.Swimming {
			eyeOffset = 0.4
		} else if p.Sneaking {
			eyeOffset = 1.27
		}
		eyeY := int(math.Floor(p.Position.Y + eyeOffset))

		feetID := w.GetBlock(bx, feetY, bz).ID()
		bodyID := w.GetBlock(bx, bodyY, bz).ID()
		eyeID := w.GetBlock(bx, eyeY, bz).ID()

		inWater := world.IsWaterBlockID(feetID) || world.IsWaterBlockID(bodyID) || isPlayerInWater(w, p.Position.X, p.Position.Y, p.Position.Z)
		eyeInWater := world.IsWaterBlockID(eyeID)
		inLava := world.IsLavaBlockID(feetID) || world.IsLavaBlockID(bodyID) || world.IsLavaBlockID(eyeID)

		// 1. Water swimming state transition
		if !inWater && !eyeInWater {
			if p.Swimming {
				p.Swimming = false
				metaChanged = true
			}
		} else if p.Sprinting && !p.Swimming {
			p.Swimming = true
			metaChanged = true
		}

		// 2. Underwater air supply (breathing bubbles) & drowning (1:1 vanilla)
		if eyeInWater && !p.effects.has(EffectWaterBreath) {
			if p.Breathing {
				p.Breathing = false
				metaChanged = true
			}
			p.AirSupply--
			if p.AirSupply <= -20 {
				p.AirSupply = 0
				dmg += 2.0 // 1 heart drowning damage per second
				metaChanged = true
			} else if p.AirSupply >= 0 && p.AirSupply%10 == 0 {
				metaChanged = true
			}
		} else {
			if !p.Breathing {
				p.Breathing = true
				metaChanged = true
			}
			if p.AirSupply < 300 {
				p.AirSupply += 4
				if p.AirSupply >= 300 {
					p.AirSupply = 300
					metaChanged = true
				} else if p.AirSupply%12 == 0 {
					metaChanged = true
				}
			}
		}

		// 3. Water extinguishes fire immediately
		if inWater || eyeInWater {
			if p.FireTicks > 0 || p.OnFire {
				p.FireTicks = 0
				p.OnFire = false
				metaChanged = true
			}
		}

		// 4. Lava contact damage & fire ignition (1:1 vanilla: 4 HP per 10 ticks + 15s fire)
		hasFireRes := p.effects.has(EffectFireResist)
		if inLava {
			if !hasFireRes {
				p.FireTicks = 300
				if !p.OnFire {
					p.OnFire = true
					metaChanged = true
				}
				if p.LavaTickCooldown <= 0 {
					dmg += 4.0
					p.LavaTickCooldown = 10
				} else {
					p.LavaTickCooldown--
				}
			}
		} else {
			p.LavaTickCooldown = 0
		}

		// 5. Residual fire ticks (1 HP every 20 ticks when burning outside lava)
		if p.FireTicks > 0 {
			p.FireTicks--
			if !inLava && p.FireTicks%20 == 0 && !hasFireRes {
				dmg += 1.0
			}
			if p.FireTicks == 0 && p.OnFire {
				p.OnFire = false
				metaChanged = true
			}
		}

		if metaChanged || dmg > 0 {
			outcomes = append(outcomes, envTickOutcome{
				id:          p.UUID,
				snap:        p.Snapshot(),
				metaChanged: metaChanged,
				damage:      dmg,
			})
		}
	}
	m.mu.Unlock()

	for _, o := range outcomes {
		if o.metaChanged {
			m.publish(Event{Type: EventSneak, Player: o.snap})
			m.syncEnvironment(o.snap)
		}
		if o.damage > 0 {
			m.ctrlMu.RLock()
			c := m.controllers[o.id]
			m.ctrlMu.RUnlock()
			if c != nil {
				c.Hurt(o.damage)
			}
			m.publish(Event{Type: EventHurt, Player: o.snap})
		}
	}
}
