// Mob-sound packet builder for the Bedrock bridge. The world tick
// publishes a []mobs.SoundEmit per tick via worlds.PublishMobSounds;
// the Bedrock bridge subscribes and broadcasts the per-emit packet
// to every session.
//
// Two packet types are used:
//
//   - LevelSoundEvent: hardcoded enum ids for the "common" sounds
//     (Hurt, Bow, MobWarning=creeper primed). These don't depend
//     on a sound registry, are part of every Bedrock build, and
//     have a uint32 SoundType.
//
//   - PlaySound: takes a namespaced string and is used for
//     vanilla-specific sounds (zombie.ambient, skeleton.ambient,
//     cow.ambient, etc.). Modern Bedrock clients translate these
//     via the resource-pack sound registry.

package server

import (
	"livingworld/internal/mobs"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// Bedrock hardcoded LevelSoundEvent ids. From the Bedrock protocol
// reference; values are stable across versions for the legacy events.
const (
	bedrockSoundBow            uint32 = 5
	bedrockSoundHurtFlesh      uint32 = 24
	bedrockSoundMobWarning     uint32 = 33 // creeper primed-hiss
)

// mobPosOf looks up the mob's current position by its entityID so
// the sound can be played at the right place. The mob store is
// edition-neutral and exposes All() for a snapshot. For v1, we
// only use the position field; if the mob has been removed (rare
// race), the sound is silently dropped.
func (s *Server) mobPosOf(id int64) (x, y, z float32, ok bool) {
	for _, m := range s.wm.Mobs().All() {
		if m.EntityID == id {
			return float32(m.X), float32(m.Y), float32(m.Z), true
		}
	}
	return 0, 0, 0, false
}

// buildBedrockSoundPackets returns the per-emit packet for a
// SoundEmit. v1 may return more than one packet (e.g. hurt + flesh
// splatter) but in practice it's one or zero.
func (s *Server) buildBedrockSoundPackets(e mobs.SoundEmit) []packet.Packet {
	x, y, z, ok := s.mobPosOf(e.EntityID)
	if !ok {
		return nil
	}
	pos := mgl32.Vec3{x, y, z}
	vol := e.Volume
	pitch := e.Pitch
	switch e.Sound {
	case mobs.SoundMobShoot:
		// Bow shoot.
		return []packet.Packet{&packet.LevelSoundEvent{
			SoundType:           bedrockSoundBow,
			Position:            pos,
			ExtraData:           0,
			EntityType:          "minecraft:skeleton",
			DisableRelativeVolume: false,
			EntityUniqueID:      e.EntityID,
		}}
	case mobs.SoundMobCreeperSay:
		// Creeper primed-hiss. Bedrock uses MobWarning for the
		// "fuse lit" sound.
		return []packet.Packet{&packet.LevelSoundEvent{
			SoundType:      bedrockSoundMobWarning,
			Position:       pos,
			ExtraData:      0,
			EntityType:     "minecraft:creeper",
			EntityUniqueID: e.EntityID,
		}}
	case mobs.SoundMobHurt:
		// Generic flesh hurt. HurtFlesh plays a universal "ow"
		// regardless of mob species.
		return []packet.Packet{&packet.LevelSoundEvent{
			SoundType:      bedrockSoundHurtFlesh,
			Position:       pos,
			ExtraData:      0,
			EntityType:     "minecraft:zombie", // client uses this for tie-break
			EntityUniqueID: e.EntityID,
		}}
	case mobs.SoundMobDeath:
		// Vanilla Bedrock has no specific "generic death" sound;
		// PlaySound with the namespaced "minecraft:entity.generic.death"
		// works on modern clients.
		return []packet.Packet{&packet.PlaySound{
			SoundName: string(e.Sound),
			Position:  pos,
			Volume:    vol,
			Pitch:     pitch,
			Handle:    protocol.Optional[uint64]{},
		}}
	default:
		// Mob ambients (zombie/skeleton/cow/pig/sheep/chicken) and
		// any unknown type: use PlaySound with the namespaced id.
		// Modern clients (1.21+) translate these via the resource
		// pack; older clients get silence. That's acceptable for v1.
		if e.Sound == "" {
			return nil
		}
		return []packet.Packet{&packet.PlaySound{
			SoundName: string(e.Sound),
			Position:  pos,
			Volume:    vol,
			Pitch:     pitch,
			Handle:    protocol.Optional[uint64]{},
		}}
	}
}

// publishMobSounds is the bridge-side OnMobSound hook. The bridge
// registers it via worlds.OnMobSound at boot. v1 broadcasts to every
// session; per-session AOI for sounds is overkill (the client
// already culls by position-based attenuation).
func (s *Server) publishMobSounds(emits []mobs.SoundEmit) {
	if len(emits) == 0 {
		return
	}
	for _, e := range emits {
		pkts := s.buildBedrockSoundPackets(e)
		for _, p := range pkts {
			s.forEachSession(func(v *bedrockSession) { v.write(p) })
		}
	}
}
