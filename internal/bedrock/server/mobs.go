package server

import (
	"livingworld/internal/mobs"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// startMobSync renders shared mob spawns/despawns to all Bedrock viewers
// with M0.7 per-session AOI. Each session has a mobTracker that records
// which mobs are currently inside its 80-block AOI; OnMove only sends
// MoveActorAbsolute to sessions where the mob is in range, and spawns
// new mobs / despawns exiting ones on the fly.
func (s *Server) startMobSync() {
	store := s.wm.Mobs()
	store.OnSpawn(func(m mobs.Mob) {
		// OnSpawn: try to add to every session in range. Sessions
		// outside the AOI just don't get the AddActor and only
		// learn about the mob when the player walks close.
		s.forEachSession(func(v *bedrockSession) {
			bs := s.playerPosOf(v)
			if !mobInAOI(bs.x, bs.z, m.X, m.Z) {
				return
			}
			v.write(addMobActor(m))
			v.mobViewer.markSpawned(m.EntityID)
		})
	})
	store.OnDespawn(func(id int64) {
		// OnDespawn: every session that knows about the mob needs
		// a RemoveActor. Most are silent because the mob was never
		// in their AOI.
		s.forEachSession(func(v *bedrockSession) {
			if v.mobViewer.markDespawned(id) {
				v.write(&packet.RemoveActor{EntityUniqueID: id})
			}
		})
	})
	store.OnMove(func(m mobs.Mob) {
		// OnMove: for every session, decide add / move / remove.
		// The mob enters a session's AOI when the distance shrinks
		// below 80 b; it leaves when the distance grows past 80 b.
		s.forEachSession(func(v *bedrockSession) {
			bs := s.playerPosOf(v)
			inRange := mobInAOI(bs.x, bs.z, m.X, m.Z)
			if inRange {
				if v.mobViewer.isSpawned(m.EntityID) {
					// OnGround flag: without it the client treats the entity as
					// airborne and plays the falling animation — most visible on
					// chickens, which flap their wings continuously.
					var moveFlags byte
					if m.OnGround {
						moveFlags |= packet.MoveFlagOnGround
					}
					v.write(&packet.MoveActorAbsolute{
						EntityRuntimeID: uint64(m.EntityID),
						Flags:           moveFlags,
						Position:        mgl32.Vec3{float32(m.X), float32(m.Y), float32(m.Z)},
						// Rotation = {pitch, body yaw, head yaw}. The rombak AI
						// decouples HeadYaw so the head can track a player while
						// the body faces its movement heading; HeadPitch tilts
						// the head up/down toward the look target.
						Rotation: mgl32.Vec3{float32(m.HeadPitch), float32(m.Yaw), float32(m.HeadYaw)},
					})
					// On-fire flag only on transition (sun-burn etc.).
					if v.mobViewer.fireChanged(m.EntityID, m.FireTicks > 0) {
						v.write(&packet.SetActorData{
							EntityRuntimeID: uint64(m.EntityID),
							EntityMetadata:  mobEntityMetadata(m),
						})
					}
				} else {
					v.write(addMobActor(m))
					v.mobViewer.markSpawned(m.EntityID)
				}
			} else if v.mobViewer.markDespawned(m.EntityID) {
				v.write(&packet.RemoveActor{EntityUniqueID: m.EntityID})
			}
		})
	})
	// Skeleton arrows + M1 projectiles (M3). Same AOI pattern, but
	// AddActor's EntityType routes on p.Kind so the client renders
	// the right visual (arrow vs small_fireball vs trident).
	proj := s.wm.Projectiles()
	proj.OnSpawn(func(p mobs.Projectile) {
		s.forEachSession(func(v *bedrockSession) {
			bs := s.playerPosOf(v)
			if !mobInAOI(bs.x, bs.z, p.X, p.Z) {
				return
			}
			v.write(addProjectileActor(p))
		})
	})
	proj.OnMove(func(p mobs.Projectile) {
		s.forEachSession(func(v *bedrockSession) {
			bs := s.playerPosOf(v)
			if !mobInAOI(bs.x, bs.z, p.X, p.Z) {
				return
			}
			rot := mgl32.Vec3{0, 0, 0}
			if !isBedrockArrowKind(p.Kind) {
				rot = mgl32.Vec3{float32(p.Pitch), float32(p.Yaw), float32(p.Yaw)}
			}
			v.write(&packet.MoveActorAbsolute{
				EntityRuntimeID: uint64(p.EntityID),
				Position:        mgl32.Vec3{float32(p.X), float32(p.Y), float32(p.Z)},
				Rotation:        rot,
			})
		})
	})
	proj.OnDespawn(func(id int64) {
		// Arrows have no per-session tracker (they're transient);
		// broadcast RemoveActor to all sessions, the culled ones
		// never spawned the arrow so the client silently no-ops.
		s.forEachSession(func(v *bedrockSession) { v.write(&packet.RemoveActor{EntityUniqueID: id}) })
	})
	// Creeper explosion event (sound + particles). Damage and
	// knockback are applied server-side; the visual is a LevelEvent
	// broadcast. Explosions are infrequent and global, so AOI isn't
	// worth the bookkeeping.
	s.wm.OnExplosion(func(r mobs.ExplosionResult) {
		s.forEachSession(func(v *bedrockSession) {
			v.write(&packet.LevelEvent{
				EventType: packet.LevelEventParticlesExplosion,
				Position:  mgl32.Vec3{float32(r.X), float32(r.Y), float32(r.Z)},
				EventData: 0,
			})
		})
	})
}

func addMobActor(m mobs.Mob) *packet.AddActor {
	return &packet.AddActor{
		EntityUniqueID:  m.EntityID,
		EntityRuntimeID: uint64(m.EntityID),
		EntityType:      m.Type,
		Position:        mgl32.Vec3{float32(m.X), float32(m.Y), float32(m.Z)},
		Velocity:        mgl32.Vec3{},
		Pitch:           float32(m.HeadPitch),
		Yaw:             float32(m.Yaw),
		HeadYaw:         float32(m.HeadYaw),
		EntityMetadata:  mobEntityMetadata(m),
	}
}

// mobEntityMetadata returns the per-mob Bedrock EntityMetadata
// (M4) for visual variants that the entity-type identifier alone
// doesn't carry:
//   - slime / magma_cube: EntityDataKeyVariant (key 2) set to
//     the slime's size index. Bedrock's variant index is
//     0-based: 0=small (size 1), 1=medium (size 2), 2=large
//     (size 3), 3=huge (size 4). Without this metadata the
//     client renders a default-size slime regardless of
//     Mob.Size.
//   - drowned: EntityDataKeyVariant = 1 (trident mode). v1
//     always renders the trident; the variant=0 mode (no
//     trident, vanilla 85% case) is not implemented.
//   - phantom: EntityDataKeyScale (key 42) = 0 (default size).
//     Phantom size is dynamic in vanilla (grows on each attack
//     cycle); the M1 AI does not yet model that.
//
// All other mob types get an empty metadata map, which
// gophertunnel serializes to the default (flags=0,
// flags2=0, player_flags=0) and the client interprets as
// "use the entity-type default look".
func mobEntityMetadata(m mobs.Mob) protocol.EntityMetadata {
	md := protocol.NewEntityMetadata()
	switch m.Type {
	case "minecraft:slime", "minecraft:magma_cube":
		// Variant = size - 1 (0-based). Clamp to 0..3 (Bedrock
		// variant range). Size 0/negative → 0 (small).
		variant := int32(0)
		if m.Size >= 4 {
			variant = 3
		} else if m.Size >= 1 {
			variant = int32(m.Size - 1)
		}
		md[protocol.EntityDataKeyVariant] = variant
	case "minecraft:drowned":
		// Variant 1 = trident. v1 always uses 1 (M4 limitation).
		md[protocol.EntityDataKeyVariant] = int32(1)
	}
	// On-fire flag (rombak): the AI's FireTicks drives the flame overlay.
	// SetFlag toggles the bit in the shared EntityDataKeyFlags bitfield.
	if m.FireTicks > 0 {
		md.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagOnFire)
	}
	return md
}

// bedrockProjectileEntityType maps a ProjectileKind (M1) to its
// Bedrock namespace-typed entity identifier. Tipped arrow variants
// (arrow_slowness / arrow_poison) reuse arrow's identifier; the
// effect metadata is shipped in a follow-up packet (M3.6+).
var bedrockProjectileEntityType = map[string]string{
	mobs.ProjectileArrow:         "minecraft:arrow",
	mobs.ProjectileArrowSlowness: "minecraft:arrow",
	mobs.ProjectileArrowPoison:   "minecraft:arrow",
	mobs.ProjectileSmallFireball: "minecraft:small_fireball",
	mobs.ProjectileLargeFireball: "minecraft:fireball",
	mobs.ProjectileTrident:       "minecraft:thrown_trident",
	mobs.ProjectilePotion:        "minecraft:splash_potion",
}

func addProjectileActor(p mobs.Projectile) *packet.AddActor {
	t, ok := bedrockProjectileEntityType[p.Kind]
	if !ok {
		t = "minecraft:arrow"
	}
	rotYaw := float32(0)
	rotPitch := float32(0)
	if !isBedrockArrowKind(p.Kind) {
		rotYaw = float32(p.Yaw)
		rotPitch = float32(p.Pitch)
	}
	return &packet.AddActor{
		EntityUniqueID:  p.EntityID,
		EntityRuntimeID: uint64(p.EntityID),
		EntityType:      t,
		Position:        mgl32.Vec3{float32(p.X), float32(p.Y), float32(p.Z)},
		Velocity:        mgl32.Vec3{float32(p.VX), float32(p.VY), float32(p.VZ)},
		Yaw:             rotYaw,
		HeadYaw:         rotYaw,
		Pitch:           rotPitch,
	}
}

// isBedrockArrowKind reports whether p.Kind is one of the arrow /
// trident variants. Arrows interpolate orientation from velocity;
// fireballs and potions need an explicit yaw/pitch to face the
// target.
func isBedrockArrowKind(kind string) bool {
	switch kind {
	case mobs.ProjectileArrow,
		mobs.ProjectileArrowSlowness,
		mobs.ProjectileArrowPoison,
		mobs.ProjectileTrident:
		return true
	}
	return false
}

// spawnExistingMobs is called when a Bedrock session joins. It uses
// the per-session mobViewer to track which mobs were just spawned on
// this viewer, so the OnMove loop is consistent with the existing
// state (no double-spawn, no miss-spawn).
func (s *Server) spawnExistingMobs(v *bedrockSession) {
	bs := s.playerPosOf(v)
	for _, m := range s.wm.Mobs().All() {
		if !mobInAOI(bs.x, bs.z, m.X, m.Z) {
			continue
		}
		v.write(addMobActor(m))
		v.mobViewer.markSpawned(m.EntityID)
	}
	for _, p := range s.wm.Projectiles().All() {
		if !mobInAOI(bs.x, bs.z, p.X, p.Z) {
			continue
		}
		v.write(addProjectileActor(p))
	}
}

// playerPos is a small struct returned by playerPosOf so callers
// can read x/z without taking the session lock twice.
type playerPos struct {
	x, y, z float64
}

// playerPosOf returns the viewer's last known position from the
// session's lastX/lastY/lastZ fields. These are written by
// publishBedrockMove under s.mu, so we take the lock here too.
func (s *Server) playerPosOf(v *bedrockSession) playerPos {
	v.mu.Lock()
	defer v.mu.Unlock()
	return playerPos{x: v.lastX, y: v.lastY, z: v.lastZ}
}
