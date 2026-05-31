package player

import (
	"fmt"

	"livingworld/internal/shared/constants/chat"

	"github.com/google/uuid"
)

func (m *Manager) AddPlayer(p *Player) {
	m.mu.Lock()
	m.players[p.UUID] = p
	m.mu.Unlock()
	m.publish(Event{Type: EventJoin, Player: p.Snapshot()})
	m.Broadcast(chat.ColorYellow + fmt.Sprintf(msgJoinedGame, p.Username))
}

func (m *Manager) RemovePlayer(id uuid.UUID) {
	m.mu.Lock()
	p := m.players[id]
	delete(m.players, id)
	m.mu.Unlock()
	if p != nil {
		m.SavePlayer(p) // persist position/health/inventory on disconnect
		m.publish(Event{Type: EventLeave, Player: p.Snapshot()})
		m.Broadcast(chat.ColorYellow + fmt.Sprintf(msgLeftGame, p.Username))
	}
}

func (m *Manager) GetPlayer(id uuid.UUID) *Player {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.players[id]
}

func (m *Manager) GetPlayerByName(name string) *Player {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.players {
		if p.Username == name {
			return p
		}
	}
	return nil
}

func (m *Manager) GetAllPlayers() []*Player {
	m.mu.RLock()
	defer m.mu.RUnlock()
	players := make([]*Player, 0, len(m.players))
	for _, p := range m.players {
		players = append(players, p)
	}
	return players
}

func (m *Manager) PlayerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.players)
}

func (m *Manager) BroadcastChat(message string) {
	m.Broadcast(message)
}
