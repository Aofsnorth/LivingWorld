package server

import (
	"math"
	"time"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"

	"livingworld/internal/drops"
	"livingworld/internal/player"
)

// pickup tuning
const (
	// Vanilla ItemEntity pickup uses ServerPlayer.touch (player AABB inflated by
	// 1.0 horizontally and 0.5 vertically, then intersected with the item AABB).
	// Plain 3D-radius checks magnetised items from too far up/sideways and made
	// pickups feel "snappy". We mirror the vanilla inflation here.
	javaPickupInflateXZ = 1.0
	javaPickupInflateY  = 0.5
	javaPickupDelayTks  = 10   // store ticks (0.5s at 20Hz) before a drop is collectable — vanilla block/mob-drop delay
	javaPickupHz        = 20
	javaDropDespawnTks  = 6000 // store ticks (5 min at 20Hz) before an uncollected drop despawns — vanilla item lifetime

	// Player AABB used for pickup containment (vanilla width 0.6, height 1.8).
	playerHalfWidth = 0.3
	playerHeight    = 1.8
	// Item entity is rendered ~0.25 tall; the pickup AABB treats it as a small
	// box around the position so we keep the same half-extent both axes.
	itemHalfExtent = 0.125
)

// startDropLoop wires the shared drop store to Java clients: new drops are spawned
// as item entities for every session, despawned drops are removed, and a proximity
// loop collects drops into the nearest player's inventory.
func (j *javaBridge) startDropLoop() {
	store := j.wm.Drops()

	store.OnSpawn(func(d drops.Drop) {
		j.sessions.ForEach(func(s *PlayerSession) {
			if s.Ready {
				_ = s.version.SpawnItemEntity(s, int32(d.EntityID), d.Item, d.Count, d.X, d.Y, d.Z)
				// Give the item its pop/scatter velocity so the Java client runs
				// vanilla item physics. The 26.1 AddEntity carries no velocity, so
				// without this the item just plops straight down.
				_ = s.SendPacket(pk.Marshal(
					packetid.ClientboundGameSetEntityMotion,
					pk.VarInt(int32(d.EntityID)),
					toLpVec3(d.VX, d.VY, d.VZ),
				))
			}
		})
	})
	store.OnDespawn(func(id int64) {
		j.sessions.ForEach(func(s *PlayerSession) {
			if s.Ready {
				_ = s.version.RemoveItemEntity(s, int32(id))
			}
		})
	})
	// Server-authoritative drop physics (StartDropPhysics) drives this: each move
	// teleports the item to its exact position WITH its current velocity so the
	// Java client interpolates in the right direction between the 20 Hz updates
	// (zero-velocity teleports like moveMobPacket would make falling items stutter).
	store.OnMove(func(d drops.Drop) {
		j.sessions.ForEach(func(s *PlayerSession) {
			if s.Ready {
				_ = s.SendPacket(moveDropPacket(d))
			}
		})
	})

	go func() {
		ticker := time.NewTicker(time.Second / javaPickupHz)
		defer ticker.Stop()
		for range ticker.C {
			j.pickupTick(store)
		}
	}()
}

// moveDropPacket teleports a drop to its current position, carrying its real
// velocity so the client interpolates smoothly between 20 Hz physics updates.
// Mirrors moveMobPacket's field layout (the proven entity-teleport shape).
func moveDropPacket(d drops.Drop) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameTeleportEntity,
		pk.VarInt(int32(d.EntityID)),
		pk.Double(d.X), pk.Double(d.Y), pk.Double(d.Z),
		pk.Double(d.VX), pk.Double(d.VY), pk.Double(d.VZ),
		pk.Float(0), pk.Float(0), // yaw, pitch (items have no facing)
		pk.Int(0),
		pk.Boolean(d.OnGround),
	)
}

// pickupTick advances the store clock and collects any drop a player is standing
// on (after a short delay) into that player's inventory.
func (j *javaBridge) pickupTick(store *drops.Store) {
	now := store.Tick()
	for _, d := range store.All() {
		if now-d.SpawnTick > javaDropDespawnTks {
			// Vanilla 5-minute despawn for uncollected items. Remove() fires
			// OnDespawn so both editions send RemoveEntities/RemoveActor. This is the
			// central drop loop (runs regardless of player count), so it covers
			// Bedrock-spawned drops too.
			store.Remove(d.EntityID)
			continue
		}
		if now-d.SpawnTick < javaPickupDelayTks {
			continue // pickup delay not elapsed
		}
		collector := j.nearestPlayer(d)
		if collector == nil {
			continue
		}
		// Claim the drop atomically (no despawn packet — the take animation
		// removes it client-side); if another loop/edition took it, skip.
		if !store.Claim(d.EntityID) {
			continue
		}
		j.collectDrop(collector, d)
	}
}

// nearestPlayer returns a player whose vanilla-inflated AABB intersects the
// drop, or nil. The match is on AABB containment (not 3D distance) so an item
// straight above the player at 1.4 blocks no longer magnetises — it has to fall
// within the same touchable box vanilla uses.
func (j *javaBridge) nearestPlayer(d drops.Drop) *player.Player {
	var best *player.Player
	bestSq := math.MaxFloat64
	for _, p := range j.pm.GetAllPlayers() {
		dx := p.Position.X - d.X
		dy := p.Position.Y - d.Y // player Y is feet
		dz := p.Position.Z - d.Z
		// AABB overlap test: player bb (centered at feet, half-width 0.3, height 1.8)
		// inflated by (1.0, 0.5, 1.0) vs the drop's 0.25-box. Equivalent to checking
		// |dx| < 0.3+1.0+0.125, dy in [-0.5-0.125, 1.8+0.5+0.125], |dz| < 0.3+1.0+0.125.
		if math.Abs(dx) > playerHalfWidth+javaPickupInflateXZ+itemHalfExtent {
			continue
		}
		if math.Abs(dz) > playerHalfWidth+javaPickupInflateXZ+itemHalfExtent {
			continue
		}
		if -dy < -javaPickupInflateY-itemHalfExtent || -dy > playerHeight+javaPickupInflateY+itemHalfExtent {
			continue
		}
		// Among multiple players inside the box, pick the closest by 3D distance.
		sq := dx*dx + dy*dy + dz*dz
		if sq < bestSq {
			bestSq = sq
			best = p
		}
	}
	return best
}

// collectDrop adds the drop to the collector's inventory, plays the pickup
// animation to all Java sessions, and despawns the item entity.
func (j *javaBridge) collectDrop(collector *player.Player, d drops.Drop) {
	itemID, ok := itemNetworkID(d.Item)
	if ok && collector.Inventory != nil {
		collector.Inventory.AddItem(player.ItemStack{ID: itemID, Count: int8(d.Count)})
	}
	j.sessions.ForEach(func(s *PlayerSession) {
		if !s.Ready {
			return
		}
		// The take packet flies the item to the collector and the client removes
		// it once the stack shrinks to empty. Sending an explicit remove here (or
		// via the store despawn) deletes the entity first and cancels the
		// animation — that was the "no magnet pickup" bug.
		_ = s.version.TakeItemEntity(s, int32(d.EntityID), int32(collector.EntityRuntimeID), d.Count)
	})
	// Push the new inventory slot to the collector if it's a Java session.
	if cs := j.sessions.Get(collector.UUID); cs != nil {
		cs.syncInventory()
	}
	// Re-render the collector's hand for every other player (both editions): a
	// pickup that lands in (or stacks into) the held slot changes the visible
	// equipment. This is the central pickup loop for BOTH editions, so it covers
	// Bedrock collectors too. Cheap and idempotent — handlers only read the held
	// slot — so it's safe to publish unconditionally.
	j.pm.PublishEquipmentChange(collector.UUID)
	// Notify Bedrock server about pickup for Bedrock players (animation + inventory sync).
	j.wm.NotifyItemPickup(collector.UUID, d.EntityID, collector.EntityRuntimeID)
}
