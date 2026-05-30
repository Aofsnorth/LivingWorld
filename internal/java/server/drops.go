package server

import (
	"time"

	"livingworld/internal/drops"
	"livingworld/internal/player"
)

// pickup tuning
const (
	javaPickupRadius   = 1.4 // blocks; horizontal+vertical distance to collect
	javaPickupDelayTks = 10  // store ticks (~0.5s at 20Hz) before a drop is collectable
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
		// Claim the drop atomically; if another loop/edition already took it, skip.
		if !store.Remove(d.EntityID) {
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
		// Pickup animation: item flies to the collector entity.
		_ = s.version.TakeItemEntity(s, int32(d.EntityID), int32(collector.EntityRuntimeID), d.Count)
		_ = s.version.RemoveItemEntity(s, int32(d.EntityID))
	})
	// Push the new inventory slot to the collector if it's a Java session.
	if cs := j.sessions.Get(collector.UUID); cs != nil {
		cs.syncInventory()
	}
}
