// Mob-sound packet builder for the Java bridge. The world tick
// publishes a []mobs.SoundEmit per tick via worlds.PublishMobSounds;
// the Java bridge subscribes and broadcasts the per-emit
// ClientboundGameSoundEntity (id 116) packet to every session.
//
// Wire format for ClientboundGameSoundEntity in protocol 775:
//   SoundEvent{Type=VarInt(0), SoundName=Identifier, FixedRange=None}
//   SoundSource VarInt     (0=master, 5=hostile, 8=ambient, 10=ui, …)
//   EntityID    VarInt     (the mob's entity id, so the client
//                           attenuates by distance to the mob)
//   Volume      Float
//   Pitch       Float
//   Seed        Long       (random; vanilla clients don't care)

package server

import (
	"livingworld/internal/mobs"

	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"

	"github.com/Tnze/go-mc/level/component"
)

// soundCategoryFor picks the right Java SoundSource enum for a mob
// sound. Vanilla 1.21 SoundSource has 11 entries (indices 0-10):
//
//	MASTER=0, MUSIC=1, RECORDS=2, WEATHER=3, BLOCKS=4, HOSTILE=5,
//	NEUTRAL=6, PLAYERS=7, AMBIENT=8, VOICE=9, UI=10
//
// Out-of-range VarInts trigger a readEnum crash on the client
// (IndexOutOfBoundsException: Index N out of bounds for length 11).
// M0.7 original code used the protocol 1.8 ids (Hostile=20,
// Ambient=11) which worked then; the 1.21 enum is the compact
// set above. Hostile mob sounds stay at HOSTILE=5, passive mob
// ambient sounds at AMBIENT=8, hurt/death events at MASTER=0
// (the client doesn't apply category attenuation to entity
// sounds regardless).
func soundCategoryFor(e mobs.SoundEmit) int32 {
	switch e.Sound {
	case string(mobs.SoundMobZombieSay), string(mobs.SoundMobSkeletonSay),
		string(mobs.SoundMobCreeperSay), string(mobs.SoundMobShoot):
		return 5 // HOSTILE
	case string(mobs.SoundMobCowSay), string(mobs.SoundMobPigSay),
		string(mobs.SoundMobSheepSay), string(mobs.SoundMobChickenSay):
		return 8 // AMBIENT
	case string(mobs.SoundMobHurt), string(mobs.SoundMobDeath):
		return 0 // MASTER
	}
	return 0
}

// buildSoundEntityPacket produces the per-emit ClientboundGameSoundEntity
// for the Java bridge. v1 does not use the SoundEvent registry
// (Type=0 with the full namespaced id is the safest for any vanilla
// client).
func buildSoundEntityPacket(e mobs.SoundEmit) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameSoundEntity,
		component.SoundEvent{
			Type:       0,
			SoundName:  pk.Identifier(e.Sound),
			FixedRange: pk.Option[pk.Float, *pk.Float]{},
		},
		pk.VarInt(soundCategoryFor(e)),
		pk.VarInt(int32(e.MobID)),
		pk.Float(float32(e.Volume)),
		pk.Float(float32(e.Pitch)),
		pk.Long(0), // seed: clients don't read this
	)
}

// publishMobSounds is the bridge-side OnMobSound hook. It builds a
// packet per emit and broadcasts to every session. The bridge
// registers it via worlds.OnMobSound at boot.
func (j *javaBridge) publishMobSounds(emits []mobs.SoundEmit) {
	if len(emits) == 0 {
		return
	}
	for _, e := range emits {
		j.sessions.Broadcast(buildSoundEntityPacket(e))
	}
}
