package server

import (
	"livingworld/internal/player"
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
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.spawnForeignAvatar(p) }) })
			case player.EventMove:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.moveForeignAvatar(p) }) })
			case player.EventLeave:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.removeForeignAvatar(p) }) })
			case player.EventSwing:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.swingForeignAvatar(p) }) })
			case player.EventSneak:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.updateForeignMetadata(p) }) })
			case player.EventEquipment:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.updateForeignEquipment(p) }) })
			case player.EventHurt:
				j.sessions.ForEach(func(s *PlayerSession) { s.enqueue(func() { s.hurtForeignAvatar(p) }) })
			case player.EventSkin:
				j.sessions.ForEach(func(s *PlayerSession) {
					s.enqueue(func() {
						s.removeForeignAvatar(p)
						s.spawnForeignAvatar(p)
					})
				})
			}
		}
	}()
}

func (s *PlayerSession) spawnExistingForeignPlayers() {
	for _, p := range s.Bridge.pm.GetAllPlayers() {
		if p.UUID != s.UUID() {
			s.spawnForeignAvatar(p.Snapshot())
		}
	}
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

// hurtForeignAvatar plays the red hurt flash on another player's avatar.
func (s *PlayerSession) hurtForeignAvatar(p player.PlayerSnapshot) {
	if !s.Ready || p.UUID == s.UUID() {
		return
	}
	_ = s.SendPacket(hurtAnimationPacket(int32(p.EntityRuntimeID)))
}

func (s *PlayerSession) updateForeignMetadata(p player.PlayerSnapshot) {
	if !s.Ready {
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
