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
	"strings"

	"livingworld/internal/mobs"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// Bedrock hardcoded LevelSoundEvent names. These correspond to the
// string identifiers in gophertunnel v1.57+ (modern Bedrock).
const (
	bedrockSoundBow        = packet.SoundEventBow
	bedrockSoundHurtFlesh  = packet.SoundEventHurt
	bedrockSoundDeath      = packet.SoundEventDeath
	bedrockSoundAmbient    = packet.SoundEventAmbient
	bedrockSoundMobWarning = packet.SoundEventMobWarning // creeper primed-hiss
)

// mobPosAndTypeOf looks up the mob's current position and species type by its
// entityID so the sound can be played at the right place with the right species.
func (s *Server) mobPosAndTypeOf(id int64) (x, y, z float32, mobType string, ok bool) {
	for _, m := range s.wm.Mobs().All() {
		if m.EntityID == id {
			return float32(m.X), float32(m.Y), float32(m.Z), m.Type, true
		}
	}
	return 0, 0, 0, "", false
}

// buildBedrockSoundPackets returns the per-emit packet for a
// SoundEmit. v1 may return more than one packet (e.g. hurt + flesh
// splatter) but in practice it's one or zero.
func (s *Server) buildBedrockSoundPackets(e mobs.SoundEmit) []packet.Packet {
	x, y, z, mobType, ok := s.mobPosAndTypeOf(e.MobID)
	if !ok {
		if e.X != 0 || e.Y != 0 || e.Z != 0 || e.MobType != "" {
			x, y, z, mobType, ok = float32(e.X), float32(e.Y), float32(e.Z), e.MobType, true
		}
	}
	if !ok {
		return nil
	}
	if mobType == "" {
		mobType = e.MobType
	}
	if mobType == "" {
		mobType = "minecraft:zombie"
	}
	pos := mgl32.Vec3{x, y, z}
	vol := e.Volume
	pitch := e.Pitch
	switch {
	case e.Sound == string(mobs.SoundMobShoot):
		// Bow shoot.
		return []packet.Packet{&packet.LevelSoundEvent{
			SoundType:             bedrockSoundBow,
			Position:              pos,
			ExtraData:             0,
			EntityType:            mobType,
			DisableRelativeVolume: false,
			EntityUniqueID:        e.MobID,
		}}
	case e.Sound == string(mobs.SoundMobCreeperSay):
		// Creeper primed-hiss. Bedrock uses MobWarning for the
		// "fuse lit" sound.
		return []packet.Packet{&packet.LevelSoundEvent{
			SoundType:      bedrockSoundMobWarning,
			Position:       pos,
			ExtraData:      0,
			EntityType:     "minecraft:creeper",
			EntityUniqueID: e.MobID,
		}}
	case e.Sound == string(mobs.SoundMobHurt) || strings.HasSuffix(e.Sound, ".hurt") || e.Sound == "minecraft:entity.allay.hurt_without_item" || e.Sound == "minecraft:entity.creaking.sway":
		// Species-specific hurt sound via LevelSoundEvent(SoundEventHurt) with the mob's EntityType.
		return []packet.Packet{&packet.LevelSoundEvent{
			SoundType:      bedrockSoundHurtFlesh,
			Position:       pos,
			ExtraData:      -1,
			EntityType:     mobType,
			EntityUniqueID: e.MobID,
		}}
	case e.Sound == string(mobs.SoundMobDeath) || strings.HasSuffix(e.Sound, ".death"):
		// Species-specific death sound via LevelSoundEvent(SoundEventDeath) with the mob's EntityType.
		return []packet.Packet{&packet.LevelSoundEvent{
			SoundType:      bedrockSoundDeath,
			Position:       pos,
			ExtraData:      -1,
			EntityType:     mobType,
			EntityUniqueID: e.MobID,
		}}
	case strings.HasSuffix(e.Sound, ".ambient"):
		// Species-specific ambient sound via LevelSoundEvent(SoundEventAmbient) with the mob's EntityType.
		return []packet.Packet{&packet.LevelSoundEvent{
			SoundType:      bedrockSoundAmbient,
			Position:       pos,
			ExtraData:      -1,
			EntityType:     mobType,
			EntityUniqueID: e.MobID,
		}}
	default:
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
