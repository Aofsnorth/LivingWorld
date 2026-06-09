package server

import (
	"math"

	"livingworld/internal/mobs"

	"github.com/Tnze/go-mc/data/entity"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// yawToAngle converts a yaw in Minecraft degrees to the protocol's single-byte
// angle (256 steps over 360°). Used for the head-rotation packet and AddEntity,
// which encode rotation as a byte rather than a float.
func yawToAngle(deg float64) pk.Angle {
	v := int(math.Round(deg * 256.0 / 360.0))
	return pk.Angle(int8(v & 0xff))
}

// javaMobTypeIDs maps a namespaced mob type to its Java (protocol 775) entity
// type id. Unknown types fall back to a pig.
var javaMobTypeIDs = map[string]int32{
	"minecraft:pig":                 int32(entity.Pig.ID),
	"minecraft:cow":                 int32(entity.Cow.ID),
	"minecraft:chicken":             int32(entity.Chicken.ID),
	"minecraft:sheep":               int32(entity.Sheep.ID),
	"minecraft:creeper":             int32(entity.Creeper.ID),
	"minecraft:zombie":              int32(entity.Zombie.ID),
	"minecraft:skeleton":            int32(entity.Skeleton.ID),
	// M1 variants
	"minecraft:husk":                int32(entity.Husk.ID),
	"minecraft:stray":               int32(entity.Stray.ID),
	"minecraft:bogged":              int32(entity.Bogged.ID),
	"minecraft:zombie_villager":     int32(entity.ZombieVillager.ID),
	"minecraft:drowned":             int32(entity.Drowned.ID),
	"minecraft:cave_spider":         int32(entity.CaveSpider.ID),
	"minecraft:spider":              int32(entity.Spider.ID),
	"minecraft:slime":               int32(entity.Slime.ID),
	"minecraft:magma_cube":          int32(entity.MagmaCube.ID),
	"minecraft:phantom":             int32(entity.Phantom.ID),
	"minecraft:blaze":               int32(entity.Blaze.ID),
	"minecraft:ghast":               int32(entity.Ghast.ID),
	"minecraft:witch":               int32(entity.Witch.ID),
	"minecraft:enderman":            int32(entity.Enderman.ID),
	"minecraft:piglin":              int32(entity.Piglin.ID),
	"minecraft:wither_skeleton":     int32(entity.WitherSkeleton.ID),
	"minecraft:iron_golem":          int32(entity.IronGolem.ID),
}

// javaProjectileTypeIDs maps a ProjectileKind (M1) to its Java (protocol 775)
// entity type id for AddEntity. Tipped arrow variants (arrow_slowness /
// arrow_poison) reuse the SpectralArrow entity type — the actual potion
// effect metadata is shipped in a follow-up entity-metadata packet
// (M3.6 follow-up; v1 just renders them as spectral arrows).
var javaProjectileTypeIDs = map[string]int32{
	mobs.ProjectileArrow:         int32(entity.Arrow.ID),
	mobs.ProjectileArrowSlowness: int32(entity.SpectralArrow.ID),
	mobs.ProjectileArrowPoison:   int32(entity.SpectralArrow.ID),
	mobs.ProjectileSmallFireball: int32(entity.SmallFireball.ID),
	mobs.ProjectileLargeFireball: int32(entity.Fireball.ID),
	mobs.ProjectileTrident:       int32(entity.Trident.ID),
	mobs.ProjectilePotion:        int32(entity.SplashPotion.ID),
}

// javaProjectileTypeID returns the Java entity type id for the given
// projectile, falling back to Arrow for unknown kinds. The bridges
// call this from OnSpawn to pick the right entity id per Kind.
func javaProjectileTypeID(p mobs.Projectile) int32 {
	if id, ok := javaProjectileTypeIDs[p.Kind]; ok {
		return id
	}
	return int32(entity.Arrow.ID)
}

// startMobSync renders shared mob spawns/despawns to all Java sessions
// with M0.7 per-session AOI. Each session's mobViewer tracks which
// mobs are currently inside its 80-block AOI; OnMove only sends
// TeleportEntity to sessions where the mob is in range, and spawns
// new mobs / despawns exiting ones on the fly.
func (j *javaBridge) startMobSync() {
	store := j.wm.Mobs()
	store.OnSpawn(func(m mobs.Mob) {
		j.sessions.ForEach(func(s *PlayerSession) {
			if !mobInAOI(s.X, s.Z, m.X, m.Z) {
				return
			}
			_ = s.SendPacket(spawnMobPacket(m))
			// M4: send the held-item SetEntityData right after
			// the AddEntity so the client renders the bow /
			// trident / stone sword / poppy / golden sword.
			for _, p := range mobHeldItemPackets(m) {
				_ = s.SendPacket(p)
			}
			s.mobViewer.markSpawned(m.EntityID)
		})
	})
	store.OnDespawn(func(id int64) {
		j.sessions.ForEach(func(s *PlayerSession) {
			if s.mobViewer.markDespawned(id) {
				_ = s.SendPacket(removeMobPacket(id))
			}
		})
	})
	store.OnMove(func(m mobs.Mob) {
		j.sessions.ForEach(func(s *PlayerSession) {
			inRange := mobInAOI(s.X, s.Z, m.X, m.Z)
			if inRange {
				if s.mobViewer.isSpawned(m.EntityID) {
					_ = s.SendPacket(moveMobPacket(m))
					_ = s.SendPacket(headRotatePacket(m))
					// On-fire metadata only on transition (sun-burn etc.).
					if s.mobViewer.fireChanged(m.EntityID, m.FireTicks > 0) {
						_ = s.SendPacket(mobFlagsPacket(m))
					}
				} else {
					_ = s.SendPacket(spawnMobPacket(m))
					// M4: held item for entering AOI.
					for _, p := range mobHeldItemPackets(m) {
						_ = s.SendPacket(p)
					}
					s.mobViewer.markSpawned(m.EntityID)
				}
			} else if s.mobViewer.markDespawned(m.EntityID) {
				_ = s.SendPacket(removeMobPacket(m.EntityID))
			}
		})
	})
	// Skeleton arrows + M1 projectiles: per-session AOI but no per-session
	// tracker (projectiles are transient — the cost of a stale RemoveActor
	// on a client that never saw the projectile is essentially zero).
	// M3 routes AddEntity's type id on p.Kind (Arrow / SpectralArrow /
	// SmallFireball / Fireball / SplashPotion / Trident).
	proj := j.wm.Projectiles()
	proj.OnSpawn(func(p mobs.Projectile) {
		j.sessions.ForEach(func(s *PlayerSession) {
			if !mobInAOI(s.X, s.Z, p.X, p.Z) {
				return
			}
			_ = s.SendPacket(spawnProjectilePacket(p))
		})
	})
	proj.OnMove(func(p mobs.Projectile) {
		j.sessions.ForEach(func(s *PlayerSession) {
			if !mobInAOI(s.X, s.Z, p.X, p.Z) {
				return
			}
			_ = s.SendPacket(teleportProjectilePacket(p))
		})
	})
	proj.OnDespawn(func(id int64) {
		// Broadcast RemoveEntities for projectiles (transient, no tracker).
		j.sessions.Broadcast(removeProjectilePacket(id))
	})
	// Creeper explosions: damage + knockback are applied in the world
	// tick; the bridge broadcasts the visual/sound packet here.
	// Explosions are global events, no AOI.
	j.wm.OnExplosion(func(r mobs.ExplosionResult) {
		j.sessions.Broadcast(explosionPacket(r))
	})
}

func moveMobPacket(m mobs.Mob) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameTeleportEntity,
		pk.VarInt(int32(m.EntityID)),
		pk.Double(m.X), pk.Double(m.Y), pk.Double(m.Z),
		pk.Double(0), pk.Double(0), pk.Double(0), // velocity
		pk.Float(float32(m.Yaw)), pk.Float(float32(m.HeadPitch)), // body yaw (degrees), pitch (head tilt)
		pk.Int(0),        // flags
		pk.Boolean(true), // onGround
	)
}

// headRotatePacket turns the mob's head to its HeadYaw, which the rombak AI
// decouples from the body Yaw (a mob can stroll along Yaw while its head
// tracks a nearby player via the look goals). Falls back to Yaw only when the
// AI never set a head yaw (HeadYaw defaults to Yaw on the first look tick).
func headRotatePacket(m mobs.Mob) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameRotateHead,
		pk.VarInt(int32(m.EntityID)),
		yawToAngle(m.HeadYaw),
	)
}

// mobFlagsByte builds the protocol-775 shared entity-flags byte (metadata
// index 0). Bit 0x01 = on fire. The rombak AI tracks FireTicks (sun-burn,
// future fire sources); this surfaces the flame overlay to Java clients.
func mobFlagsByte(m mobs.Mob) byte {
	var f byte
	if m.FireTicks > 0 {
		f |= 0x01
	}
	return f
}

// mobFlagsPacket emits SetEntityData carrying only the shared-flags byte
// (index 0, type 0 = Byte). Sent on a fire-state transition so a burning mob
// shows flames and stops showing them when the fire lapses.
func mobFlagsPacket(m mobs.Mob) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameSetEntityData,
		pk.VarInt(int32(m.EntityID)),
		pk.UnsignedByte(0), // index 0 = shared flags
		pk.VarInt(0),       // type 0 = Byte
		pk.Byte(mobFlagsByte(m)),
		pk.UnsignedByte(0xff), // terminator
	)
}

func spawnMobPacket(m mobs.Mob) pk.Packet {
	typeID, ok := javaMobTypeIDs[m.Type]
	if !ok {
		typeID = int32(entity.Pig.ID)
	}
	yaw := yawToAngle(m.Yaw)
	return pk.Marshal(
		packetid.ClientboundGameAddEntity,
		pk.VarInt(int32(m.EntityID)),
		pk.UUID(m.UUID),
		pk.VarInt(typeID),
		pk.Double(m.X), pk.Double(m.Y), pk.Double(m.Z),
		pk.Byte(0),  // head pitch
		pk.Angle(0), // pitch
		yaw,         // yaw
		yaw,         // head yaw
		pk.VarInt(mobDataVarInt(m)),
	)
}

// mobHeldItemPackets returns the SetEntityData packet(s) needed
// to render a mob's held item. M4: only the main-hand slot is
// supported (index 8). For mobs with no held item this returns
// nil. The packet is sent right after the AddEntity so the
// client renders the held item as soon as the mob appears.
func mobHeldItemPackets(m mobs.Mob) []pk.Packet {
	itemID, count := javaHeldItem(m)
	if itemID == 0 {
		return nil
	}
	return []pk.Packet{spawnMobDataPacket(m.EntityID, itemID, count)}
}

// mobDataVarInt returns the AddEntity data field for the mob. M3:
// slime / magma_cube use their Size as the size index (vanilla
// size 1 → 0, size 2 → 1, size 4 → 3). All other mob types return
// 0 — phantom size is dynamic, enderman held block is in metadata
// (M3.5+ follow-up), baby zombie flag is in metadata.
func mobDataVarInt(m mobs.Mob) int32 {
	switch m.Type {
	case "minecraft:slime", "minecraft:magma_cube":
		// Clamp to vanilla size index 0..3 (size 1..4). Larger
		// sizes are clamped; spawning a "size 5" slime is not
		// supported in vanilla and the client would just clamp
		// anyway.
		if m.Size <= 1 {
			return 0
		}
		if m.Size >= 5 {
			return 3
		}
		return int32(m.Size - 1)
	}
	return 0
}

func removeMobPacket(id int64) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameRemoveEntities,
		pk.Ary[pk.VarInt]{Ary: []pk.VarInt{pk.VarInt(int32(id))}},
	)
}

// spawnExistingMobs is called when a Java session joins. M0.7: filter
// by AOI so the client only gets AddEntity for mobs in its 80 b
// window, and update the per-session mobViewer so the OnMove loop
// knows which mobs this client has been told about. M4: also send
// the held-item SetEntityData for mobs that carry items.
func (s *PlayerSession) spawnExistingMobs() {
	for _, m := range s.Bridge.wm.Mobs().All() {
		if !mobInAOI(s.X, s.Z, m.X, m.Z) {
			continue
		}
		_ = s.SendPacket(spawnMobPacket(m))
		for _, p := range mobHeldItemPackets(m) {
			_ = s.SendPacket(p)
		}
		s.mobViewer.markSpawned(m.EntityID)
	}
	for _, p := range s.Bridge.wm.Projectiles().All() {
		if !mobInAOI(s.X, s.Z, p.X, p.Z) {
			continue
		}
		_ = s.SendPacket(spawnProjectilePacket(p))
	}
}

// --- Skeleton arrows (ProjectileStore.OnSpawn/OnMove/OnDespawn) ---

// spawnProjectilePacket: AddEntity with type routed on p.Kind. M3
// routes Arrow / SpectralArrow / SmallFireball / Fireball /
// SplashPotion / Trident. The arrows retain yaw=0 because the
// client interpolates orientation from velocity; fireballs get
// the projectile's Yaw so the visible flame ring orients toward
// the target. We zero the UUID — each projectile gets its own
// entity id; the UUID is for cross-session tracking that vanilla
// doesn't require.
func spawnProjectilePacket(p mobs.Projectile) pk.Packet {
	typeID := javaProjectileTypeID(p)
	yaw := yawToAngle(p.Yaw)
	// Fireballs and potions need their initial yaw to face the
	// target so the client draws the right flame/potion shape;
	// arrows (and tipped-arrow variants) interpolate from velocity
	// so yaw=0 is fine.
	if isArrowKind(p.Kind) {
		yaw = pk.Angle(0)
	}
	return pk.Marshal(
		packetid.ClientboundGameAddEntity,
		pk.VarInt(int32(p.EntityID)),
		pk.UUID([16]byte{}),
		pk.VarInt(typeID),
		pk.Double(p.X), pk.Double(p.Y), pk.Double(p.Z),
		pk.Byte(0),  // head pitch
		pk.Angle(0), // pitch
		yaw,         // yaw (fireballs/potions face target; arrows 0)
		yaw,         // head yaw
		pk.VarInt(0), // data
	)
}

// isArrowKind reports whether p.Kind is one of the arrow variants.
// Used to decide whether the AddEntity yaw is meaningful (arrows
// interpolate from velocity; fireballs/potions need a yaw).
func isArrowKind(kind string) bool {
	switch kind {
	case mobs.ProjectileArrow,
		mobs.ProjectileArrowSlowness,
		mobs.ProjectileArrowPoison,
		mobs.ProjectileTrident:
		return true
	}
	return false
}

// teleportProjectilePacket: same packet as moveMobPacket (TeleportEntity), just
// parameterized on the projectile. Arrows don't rotate visibly so yaw=0;
// fireballs/potions get the projectile's Yaw so the visual stays consistent.
func teleportProjectilePacket(p mobs.Projectile) pk.Packet {
	yaw := float32(0)
	pitch := float32(0)
	if !isArrowKind(p.Kind) {
		yaw = float32(p.Yaw)
	}
	return pk.Marshal(
		packetid.ClientboundGameTeleportEntity,
		pk.VarInt(int32(p.EntityID)),
		pk.Double(p.X), pk.Double(p.Y), pk.Double(p.Z),
		pk.Double(p.VX), pk.Double(p.VY), pk.Double(p.VZ), // velocity
		pk.Float(yaw), pk.Float(pitch), // body yaw, pitch
		pk.Int(0),         // flags
		pk.Boolean(false), // onGround
	)
}

func removeProjectilePacket(id int64) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameRemoveEntities,
		pk.Ary[pk.VarInt]{Ary: []pk.VarInt{pk.VarInt(int32(id))}},
	)
}

// --- Creeper explosions (Manager.OnExplosion) ---

// explosionPacket: ClientboundGameExplode (packet id 36 in vendored go-mc).
// Wire format: x,y,z (doubles), radius (float), recordCount (int = 0 in v1
// since we don't destroy blocks), playerKnockback (x,y,z as floats).
// The version of go-mc vendored doesn't ship a struct for this packet; we
// marshal it manually. If the client rejects the packet (newer protocol),
// the explosion will play the knockback but no visual — the player still
// gets damaged because that's applied server-side.
func explosionPacket(r mobs.ExplosionResult) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameExplode,
		pk.Double(r.X), pk.Double(r.Y), pk.Double(r.Z),
		pk.Float(float32(r.Radius)),
		pk.Int(0), // recordCount = 0 (no block destruction in v1)
		// no per-block record bytes since recordCount is 0
		pk.Float(0), pk.Float(0), pk.Float(0), // player knockback (we apply server-side)
	)
}
