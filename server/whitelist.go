package server

// Whitelist returns the join whitelist. Enforcement at login is performed by
// the protocol layer (it should reject players for which Allowed is false);
// toggle it with Whitelist().SetEnabled.
func (s *Server) Whitelist() *Whitelist { return s.whitelist }
