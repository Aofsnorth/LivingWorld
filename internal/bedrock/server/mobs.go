package server

import (
	"livingworld/internal/mobs"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// startMobSync renders shared mob spawns/despawns to all Bedrock viewers.
func (s *Server) startMobSync() {
	store := s.wm.Mobs()
	store.OnSpawn(func(m mobs.Mob) {
		s.forEachSession(func(v *bedrockSession) { v.write(addMobActor(m)) })
	})
	store.OnDespawn(func(id int64) {
		s.forEachSession(func(v *bedrockSession) { v.write(&packet.RemoveActor{EntityUniqueID: id}) })
	})
	store.OnMove(func(m mobs.Mob) {
		s.forEachSession(func(v *bedrockSession) {
			v.write(&packet.MoveActorAbsolute{
				EntityRuntimeID: uint64(m.EntityID),
				Position:        mgl32.Vec3{float32(m.X), float32(m.Y), float32(m.Z)},
			})
		})
	})
}

func addMobActor(m mobs.Mob) *packet.AddActor {
	return &packet.AddActor{
		EntityUniqueID:  m.EntityID,
		EntityRuntimeID: uint64(m.EntityID),
		EntityType:      m.Type, // Bedrock uses the namespaced identifier directly
		Position:        mgl32.Vec3{float32(m.X), float32(m.Y), float32(m.Z)},
		Velocity:        mgl32.Vec3{},
	}
}

// spawnExistingMobs sends all current mobs to a Bedrock viewer on join.
func (s *Server) spawnExistingMobs(v *bedrockSession) {
	for _, m := range s.wm.Mobs().All() {
		v.write(addMobActor(m))
	}
}
