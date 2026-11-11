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
	"strings"

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
	if e.X != 0 || e.Y != 0 || e.Z != 0 {
		return pk.Marshal(
			packetid.ClientboundGameSound,
			component.SoundEvent{
				Type:       0,
				SoundName:  pk.Identifier(e.Sound),
				FixedRange: pk.Option[pk.Float, *pk.Float]{},
			},
			pk.VarInt(soundCategoryFor(e)),
			pk.Int(int32(e.X*8)),
			pk.Int(int32(e.Y*8)),
			pk.Int(int32(e.Z*8)),
			pk.Float(float32(e.Volume)),
			pk.Float(float32(e.Pitch)),
			pk.Long(0),
		)
	}
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
func buildPlayerSoundPacket(soundName string, x, y, z float64, volume, pitch float32) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameSound,
		component.SoundEvent{
			Type:       0,
			SoundName:  pk.Identifier(soundName),
			FixedRange: pk.Option[pk.Float, *pk.Float]{},
		},
		pk.VarInt(7), // PLAYERS
		pk.Int(int32(x*8)),
		pk.Int(int32(y*8)),
		pk.Int(int32(z*8)),
		pk.Float(volume),
		pk.Float(pitch),
		pk.Long(0),
	)
}

func buildBlockSoundPacket(soundName string, x, y, z float64, volume, pitch float32) pk.Packet {
	return pk.Marshal(
		packetid.ClientboundGameSound,
		component.SoundEvent{
			Type:       0,
			SoundName:  pk.Identifier(soundName),
			FixedRange: pk.Option[pk.Float, *pk.Float]{},
		},
		pk.VarInt(4), // BLOCKS
		pk.Int(int32(x*8)),
		pk.Int(int32(y*8)),
		pk.Int(int32(z*8)),
		pk.Float(volume),
		pk.Float(pitch),
		pk.Long(0),
	)
}

func breakSoundForBlockName(name string) string {
	b := strings.TrimPrefix(name, "minecraft:")
	switch {
	case strings.Contains(b, "wood") || strings.Contains(b, "log") || strings.Contains(b, "plank") ||
		strings.Contains(b, "crafting_table") || strings.Contains(b, "chest") || strings.Contains(b, "barrel") ||
		strings.Contains(b, "door") || strings.Contains(b, "fence") || strings.Contains(b, "bookshelf") ||
		strings.Contains(b, "Sign") || strings.Contains(b, "sign") || strings.Contains(b, "loom") ||
		strings.Contains(b, "cartography") || strings.Contains(b, "fletching") || strings.Contains(b, "lectern") ||
		strings.Contains(b, "composter") || strings.Contains(b, "note_block") || strings.Contains(b, "jukebox"):
		return "minecraft:block.wood.break"
	case strings.Contains(b, "grass") || strings.Contains(b, "leaves") || strings.Contains(b, "sapling") ||
		strings.Contains(b, "flower") || strings.Contains(b, "dandelion") || strings.Contains(b, "poppy") ||
		strings.Contains(b, "orchid") || strings.Contains(b, "allium") || strings.Contains(b, "tulip") ||
		strings.Contains(b, "daisy") || strings.Contains(b, "cornflower") || strings.Contains(b, "lily") ||
		strings.Contains(b, "rose") || strings.Contains(b, "peony") || strings.Contains(b, "lilac") ||
		strings.Contains(b, "fern") || strings.Contains(b, "vine") || strings.Contains(b, "moss") ||
		strings.Contains(b, "petals") || strings.Contains(b, "litter") || strings.Contains(b, "bush") ||
		strings.Contains(b, "wheat") || strings.Contains(b, "carrot") || strings.Contains(b, "potato") ||
		strings.Contains(b, "beetroot") || strings.Contains(b, "sugar_cane") || strings.Contains(b, "cactus"):
		return "minecraft:block.grass.break"
	case strings.Contains(b, "dirt") || strings.Contains(b, "podzol") || strings.Contains(b, "mycelium") ||
		strings.Contains(b, "farmland") || strings.Contains(b, "gravel") || strings.Contains(b, "clay") ||
		strings.Contains(b, "mud"):
		return "minecraft:block.gravel.break"
	case strings.Contains(b, "sand"):
		return "minecraft:block.sand.break"
	case strings.Contains(b, "snow"):
		return "minecraft:block.snow.break"
	case strings.Contains(b, "wool") || strings.Contains(b, "carpet") || strings.Contains(b, "bed"):
		return "minecraft:block.wool.break"
	case strings.Contains(b, "glass") || strings.Contains(b, "ice") || strings.Contains(b, "glowstone") ||
		strings.Contains(b, "sea_lantern"):
		return "minecraft:block.glass.break"
	case strings.Contains(b, "anvil"):
		return "minecraft:block.anvil.destroy"
	case strings.Contains(b, "iron") || strings.Contains(b, "gold_block") || strings.Contains(b, "copper") ||
		strings.Contains(b, "chain") || strings.Contains(b, "lantern") || strings.Contains(b, "cauldron") ||
		strings.Contains(b, "Hopper") || strings.Contains(b, "hopper") || strings.Contains(b, "smithing"):
		return "minecraft:block.metal.break"
	default:
		return "minecraft:block.stone.break"
	}
}

func placeSoundForBlockName(name string) string {
	b := strings.TrimPrefix(name, "minecraft:")
	switch {
	case strings.Contains(b, "wood") || strings.Contains(b, "log") || strings.Contains(b, "plank") ||
		strings.Contains(b, "crafting_table") || strings.Contains(b, "chest") || strings.Contains(b, "barrel") ||
		strings.Contains(b, "door") || strings.Contains(b, "fence") || strings.Contains(b, "bookshelf") ||
		strings.Contains(b, "sign") || strings.Contains(b, "loom") || strings.Contains(b, "cartography") ||
		strings.Contains(b, "fletching") || strings.Contains(b, "lectern") || strings.Contains(b, "composter"):
		return "minecraft:block.wood.place"
	case strings.Contains(b, "grass") || strings.Contains(b, "leaves") || strings.Contains(b, "sapling") ||
		strings.Contains(b, "flower") || strings.Contains(b, "dandelion") || strings.Contains(b, "poppy") ||
		strings.Contains(b, "orchid") || strings.Contains(b, "allium") || strings.Contains(b, "tulip") ||
		strings.Contains(b, "daisy") || strings.Contains(b, "cornflower") || strings.Contains(b, "lily") ||
		strings.Contains(b, "rose") || strings.Contains(b, "peony") || strings.Contains(b, "lilac") ||
		strings.Contains(b, "fern") || strings.Contains(b, "vine") || strings.Contains(b, "moss") ||
		strings.Contains(b, "petals") || strings.Contains(b, "litter") || strings.Contains(b, "bush"):
		return "minecraft:block.grass.place"
	case strings.Contains(b, "dirt") || strings.Contains(b, "podzol") || strings.Contains(b, "mycelium") ||
		strings.Contains(b, "farmland") || strings.Contains(b, "gravel") || strings.Contains(b, "clay") ||
		strings.Contains(b, "mud"):
		return "minecraft:block.gravel.place"
	case strings.Contains(b, "sand"):
		return "minecraft:block.sand.place"
	case strings.Contains(b, "snow"):
		return "minecraft:block.snow.place"
	case strings.Contains(b, "wool") || strings.Contains(b, "carpet") || strings.Contains(b, "bed"):
		return "minecraft:block.wool.place"
	case strings.Contains(b, "glass") || strings.Contains(b, "ice") || strings.Contains(b, "glowstone"):
		return "minecraft:block.glass.place"
	case strings.Contains(b, "anvil"):
		return "minecraft:block.anvil.place"
	case strings.Contains(b, "iron") || strings.Contains(b, "gold_block") || strings.Contains(b, "copper") ||
		strings.Contains(b, "chain") || strings.Contains(b, "lantern") || strings.Contains(b, "cauldron") ||
		strings.Contains(b, "hopper") || strings.Contains(b, "smithing"):
		return "minecraft:block.metal.place"
	default:
		return "minecraft:block.stone.place"
	}
}

func (j *javaBridge) publishMobSounds(emits []mobs.SoundEmit) {
	if len(emits) == 0 {
		return
	}
	for _, e := range emits {
		j.sessions.Broadcast(buildSoundEntityPacket(e))
	}
}
