package server

// Broadcast sends a chat message to every connected player.
func (s *Server) Broadcast(msg string) { s.players.Broadcast(msg) }

// Message sends a chat message to a single player by name.
func (s *Server) Message(playerName, msg string) {
	if p := s.players.GetPlayerByName(playerName); p != nil {
		s.players.Message(p.UUID, msg)
	}
}

// Players returns the names of currently connected players.
func (s *Server) Players() []string {
	all := s.players.GetAllPlayers()
	names := make([]string, 0, len(all))
	for _, p := range all {
		names = append(names, p.Username)
	}
	return names
}

// PlayerCount returns the number of connected players.
func (s *Server) PlayerCount() int { return s.players.PlayerCount() }
