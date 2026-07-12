package server

// Ops returns the operator list (admins) for runtime management.
func (s *Server) Ops() *OpsList { return s.ops }

// SetOp grants or revokes operator status by name: it updates the ops list,
// keeps the login op-check (Config.Ops) in sync, and applies to the connected
// player immediately. Returns whether membership actually changed.
func (s *Server) SetOp(name string, op bool) bool {
	var changed bool
	if op {
		changed, _ = s.ops.Add(name)
	} else {
		changed, _ = s.ops.Remove(name)
	}
	s.cfg.Ops = s.ops.List()
	if pl := s.players.GetPlayerByName(name); pl != nil {
		pl.Op = op
	}
	return changed
}

// ListOps returns the current operator names.
func (s *Server) ListOps() []string { return s.ops.List() }
