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
			switch ev.Type {
			case player.EventJoin:
				j.sessions.ForEach(func(s *PlayerSession) { s.spawnForeignAvatar(ev.Player) })
			case player.EventMove:
				j.sessions.ForEach(func(s *PlayerSession) { s.moveForeignAvatar(ev.Player) })
			case player.EventLeave:
				j.sessions.ForEach(func(s *PlayerSession) { s.removeForeignAvatar(ev.Player) })
			case player.EventSwing:
				j.sessions.ForEach(func(s *PlayerSession) { s.swingForeignAvatar(ev.Player) })
			case player.EventSneak:
				j.sessions.ForEach(func(s *PlayerSession) { s.updateForeignMetadata(ev.Player) })
			case player.EventEquipment:
				j.sessions.ForEach(func(s *PlayerSession) { s.updateForeignEquipment(ev.Player) })
			case player.EventSkin:
				j.sessions.ForEach(func(s *PlayerSession) {
					s.removeForeignAvatar(ev.Player)
					s.spawnForeignAvatar(ev.Player)
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
