package server

import (
	"time"

	"livingworld/internal/java/protocol"
	"livingworld/internal/player"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

func (j *javaBridge) startPlayerEventLoop() {
	if j.playerEvents != nil {
		return
	}
	j.playerEvents = j.pm.Subscribe("java-bridge", 256)
	go func() {
		for ev := range j.playerEvents {
			p := ev.Player
			switch ev.Type {
			case player.EventJoin:
				// AOI: spawn only if the joining player is already in range; the
				// move-driven diff spawns it later if they approach.
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.reconcileViewerFor(p) }) })
			case player.EventMove:
				// AOI: spawn-if-entered / move-if-spawned / despawn-if-left.
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.reconcileViewerFor(p) }) })
			case player.EventLeave:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.despawnViewer(p) }) })
			case player.EventSwing:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.swingForeignAvatar(p) }) })
			case player.EventSneak:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.updateForeignMetadata(p) }) })
			case player.EventEquipment:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.updateForeignEquipment(p) }) })
			case player.EventHurt:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.hurtForeignAvatar(p) }) })
			case player.EventCrit:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.critForeignAvatar(p) }) })
			case player.EventSkin:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.refreshViewer(p) }) })
			}
		}
	}()
}

// spawnExistingForeignPlayers runs the initial AOI sweep on join: spawn every
// already-online player that is within view distance (out-of-range ones spawn
// later via the move-driven diff). Enqueued onto the sendLoop so it is serialized
// with the event-driven reconciles — otherwise the join goroutine and the
// event-loop goroutine could spawn the same target concurrently.
func (s *PlayerSession) spawnExistingForeignPlayers() {
	s.enqueue(func() { s.reconcileViewers() })
}

func (s *PlayerSession) spawnForeignAvatar(p player.PlayerSnapshot) {
	if !s.Ready {
		return
	}
	_ = s.version.SpawnForeignAvatar(s, p)
	_ = s.version.UpdateForeignEquipment(s, p) // show the player's held item immediately
}

func (s *PlayerSession) sendPlayerInfoAdd(p player.PlayerSnapshot) error {
	return s.version.SendPlayerInfoAdd(s, p)
}

func (s *PlayerSession) moveForeignAvatar(p player.PlayerSnapshot) {
	if !s.Ready {
		return
	}
	s.mu.Lock()
	oldPos, exists := s.lastSentPos[p.UUID]
	s.lastSentPos[p.UUID] = p.Position
	s.mu.Unlock()
	_ = s.version.MoveForeignAvatar(s, p, oldPos, exists)
	if h777, ok := s.version.(*protocol.Handler777); ok {
		_ = h777.SyncPlayerWaypoint(s, p, false)
	}
}

func (s *PlayerSession) removeForeignAvatar(p player.PlayerSnapshot) {
	if !s.Ready {
		return
	}
	_ = s.version.RemoveForeignAvatar(s, p)
}

func (s *PlayerSession) sendPlayerInfoRemove(p player.PlayerSnapshot) error {
	return s.version.SendPlayerInfoRemove(s, p)
}

func (s *PlayerSession) swingForeignAvatar(p player.PlayerSnapshot) {
	if !s.Ready {
		return
	}
	_ = s.version.SwingForeignAvatar(s, p)
}

// playDeathAndRemoveForeignAvatar plays the vanilla 20-tick fall-over death
// animation (PoseDying metadata + HurtAnimation + EntityEvent status 3) on a
// foreign player's avatar and despawns the entity after ~950ms so Java viewers
// see the full death animation instead of an instant pop-out.
func (s *PlayerSession) playDeathAndRemoveForeignAvatar(p player.PlayerSnapshot) {
	if !s.Ready || p.UUID == s.UUID() {
		return
	}
	snap, ok := s.viewers.markDespawnedIfPresent(p.UUID)
	if !ok {
		return
	}
	s.mu.Lock()
	delete(s.lastSentPos, p.UUID)
	s.mu.Unlock()

	p.Dead = true
	p.Health = 0
	_ = s.version.UpdateForeignMetadata(s, p)
	_ = s.SendPacket(hurtAnimationPacket(int32(snap.EntityRuntimeID)))
	_ = s.SendPacket(entityEventPacket(int32(snap.EntityRuntimeID), 3))

	deadUUID := snap.UUID
	time.AfterFunc(950*time.Millisecond, func() {
		if !s.Ready {
			return
		}
		if s.Bridge != nil && s.Bridge.pm != nil {
			if pl := s.Bridge.pm.GetPlayer(deadUUID); pl != nil && pl.Health > 0 {
				return
			}
		}
		s.removeForeignAvatar(snap)
	})
}

// hurtForeignAvatar plays the red hurt flash on another player's avatar.
func (s *PlayerSession) hurtForeignAvatar(p player.PlayerSnapshot) {
	if !s.Ready || p.UUID == s.UUID() {
		return
	}
	if p.Dead || p.Health <= 0 {
		s.playDeathAndRemoveForeignAvatar(p)
		return
	}
	_ = s.SendPacket(hurtAnimationPacket(int32(p.EntityRuntimeID)))
}

// critForeignAvatar plays critical hit particles on a player's avatar.
func (s *PlayerSession) critForeignAvatar(p player.PlayerSnapshot) {
	if !s.Ready {
		return
	}
	entityID := int32(p.EntityRuntimeID)
	if p.UUID == s.UUID() {
		entityID = s.EntityID()
	}
	_ = s.SendPacket(critAnimationPacket(entityID))
}

func critAnimationPacket(entityID int32) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameAnimate,
		pk.VarInt(entityID),
		pk.UnsignedByte(4), // 4 = critical effect particles
	)
}

func (s *PlayerSession) updateForeignMetadata(p player.PlayerSnapshot) {
	if !s.Ready || p.UUID == s.UUID() {
		return
	}
	if p.Dead || p.Health <= 0 {
		s.playDeathAndRemoveForeignAvatar(p)
		return
	}
	_ = s.version.UpdateForeignMetadata(s, p)
}

func (s *PlayerSession) updateForeignEquipment(p player.PlayerSnapshot) {
	if !s.Ready {
		return
	}
	_ = s.version.UpdateForeignEquipment(s, p)
}
