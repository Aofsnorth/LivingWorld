package server

import (
	"time"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"

	"livingworld/internal/drops"
	"livingworld/internal/player"
)

// pickup tuning
const (
	javaPickupRadius   = 1.5 // blocks; 3D distance to collect
	javaPickupDelayTks = 40  // store ticks (2s at 20Hz) before a drop is collectable
	javaPickupHz       = 20
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

	go func() {
		ticker := time.NewTicker(time.Second / javaPickupHz)
		defer ticker.Stop()
		for range ticker.C {
			j.pickupTick(store)
		}
	}()
}

// pickupTick advances the store clock and collects any drop a player is standing
// on (after a short delay) into that player's inventory.
func (j *javaBridge) pickupTick(store *drops.Store) {
	now := store.Tick()
	for _, d := range store.All() {
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

// nearestPlayer returns a player within pickup radius of the drop, or nil.
func (j *javaBridge) nearestPlayer(d drops.Drop) *player.Player {
	var best *player.Player
	bestSq := javaPickupRadius * javaPickupRadius
	for _, p := range j.pm.GetAllPlayers() {
		dx := p.Position.X - d.X
		dy := p.Position.Y - d.Y
		dz := p.Position.Z - d.Z
		sq := dx*dx + dy*dy + dz*dz
		if sq <= bestSq {
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
	// Notify Bedrock server about pickup for Bedrock players (animation + inventory sync).
	j.wm.NotifyItemPickup(collector.UUID, d.EntityID, collector.EntityRuntimeID)
}
