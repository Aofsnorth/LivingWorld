package server

import "livingworld/internal/world"

// GetBlock returns the block state ID at a world position.
func (s *Server) GetBlock(x, y, z int) int32 {
	return s.worlds.GetDefaultWorld().GetBlock(x, y, z).ID()
}

// SetBlock sets the block state ID at a world position and notifies clients on
// both protocols.
func (s *Server) SetBlock(x, y, z int, stateID int32) {
	s.worlds.SetBlockAndPublish(world.BlockUpdateSourceServer, x, y, z, world.BlockByID(stateID))
}

// StateID resolves a block state ID from a namespaced name ("minecraft:stone").
func (s *Server) StateID(name string) int32 { return world.StateID(name) }
