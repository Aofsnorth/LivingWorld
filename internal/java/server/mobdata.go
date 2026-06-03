package server

import (
	"bytes"

	"livingworld/internal/mobs"

	"github.com/Tnze/go-mc/data/item"
	"github.com/Tnze/go-mc/data/packetid"
	pk "github.com/Tnze/go-mc/net/packet"
)

// M4: mob held item / equipment rendering. The vendored go-mc
// (replace github.com/Tnze/go-mc => ./third_party/go-mc) does
// not ship a SetEntityData packet struct or a Slot Field type,
// so we build the binary wire format by hand. The protocol
// 775 layout is:
//
//	packet id         VarInt
//	entity id         VarInt
//	metadata entries  terminated by 0xFF
//	  index           UnsignedByte
//	  type            VarInt  (6 = Slot)
//	  slot value
//	    present       VarInt (0 = empty, 1 = present)
//	    [if present:]
//	      item id     VarInt
//	      count       Byte
//	      nbt         NBT (root tag; here we use the empty TagEnd)
//
// The held item field index for most humanoid mobs (skeleton,
// zombie, drowned, wither_skeleton, piglin, player, iron_golem)
// is index 8 in protocol 775. Spider, enderman, phantom, slime,
// magma_cube, ghast, witch, blaze — no held item in vanilla.
// Stray, bogged, husk, zombie_villager — same as their base
// (skeleton / zombie) so we can treat them identically.

// javaHeldItem returns the (item id, count) that the given mob
// type should be rendered holding, or (0, 0) for no held item.
// Count is always 1 for mob equipment (vanilla always renders
// the held item in the mob's main hand).
//
// Drowned has a 15% trident chance in vanilla; v1: we always
// give a trident to avoid per-mob state and the 85% case is
// already covered by the mob's "no held item" path on
// specific mob UUIDs (TODO: weight by hash). For now, all
// drowned get the trident.
func javaHeldItem(m mobs.Mob) (itemID int32, count int8) {
	switch m.Type {
	case "minecraft:skeleton",
		"minecraft:stray",
		"minecraft:bogged":
		return int32(item.Bow.ID), 1
	case "minecraft:drowned":
		return int32(item.Trident.ID), 1
	case "minecraft:wither_skeleton":
		return int32(item.StoneSword.ID), 1
	case "minecraft:piglin":
		// Vanilla piglin 50% hold a golden sword; v1: all hold
		// one. The trade-off is fine — the alternative requires
		// per-mob UUID-based RNG at spawn time and a persistent
		// "is armed" bit.
		return int32(item.GoldenSword.ID), 1
	case "minecraft:iron_golem":
		// Iron golem holds a poppy in vanilla. v1: always.
		return int32(item.Poppy.ID), 1
	}
	return 0, 0
}

// heldItemMetadataSlot is the field index for the main-hand
// held item in protocol 775 across humanoid mob types
// (skeleton, zombie, drowned, wither_skeleton, piglin,
// iron_golem, player). Non-humanoid mobs don't have a held
// item field at this index (they have it absent — sending
// the metadata entry is a no-op on the client).
const heldItemMetadataSlot = 8

// spawnMobDataPacket emits the SetEntityData packet (id 99)
// with a single Slot metadata entry at heldItemMetadataSlot.
// Wire format:
//
//	VarInt  99
//	VarInt  entity_id
//	Byte    8               (index)
//	VarInt  6               (type = Slot)
//	VarInt  1               (present = 1)
//	VarInt  item_id
//	Byte    count
//	NBT     TagEnd (0x00)
//	Byte    0xFF            (terminator)
//
// The NBT root tag is the empty end tag (no compound wrapper).
// The Minecraft 1.20.3+ network NBT format dropped the outer
// name (which is what pk.NBT(nil) emits), so this matches.
func spawnMobDataPacket(entityID int64, itemID int32, count int8) pk.Packet {
	// pk.Marshal writes each Field's bytes. We build the
	// components in a fixed order so the wire layout matches
	// the layout above. The NBT field for an empty end tag is
	// a single 0x00 byte, which is what pk.NBT(nil) emits.
	return pk.Marshal(
		packetid.ClientboundGameSetEntityData,
		pk.VarInt(int32(entityID)),
		pk.UnsignedByte(heldItemMetadataSlot),
		pk.VarInt(6), // Slot type
		pk.VarInt(1), // present
		pk.VarInt(itemID),
		pk.Byte(count),
		pk.NBT(nil), // empty NBT
		pk.UnsignedByte(0xff), // metadata terminator
	)
}

// encodeSlotForTest is a test-only helper that returns the
// raw wire bytes of the Slot payload (everything after the
// type byte), for byte-level assertions in tests.
func encodeSlotForTest(itemID int32, count int8) []byte {
	var buf bytes.Buffer
	// present=1
	buf.WriteByte(0x01)
	// VarInt itemID (varint encoding: 7 bits per byte)
	writeVarInt(&buf, int64(itemID))
	// count byte
	buf.WriteByte(byte(count))
	// NBT end tag
	buf.WriteByte(0x00)
	return buf.Bytes()
}

// writeVarInt writes a 32-bit VarInt to buf. Copied from the
// pk.VarInt encoder logic for the test helper. Standard
// VarInt: 7 bits per byte, MSB = continuation.
func writeVarInt(buf *bytes.Buffer, v int64) {
	u := uint64(v)
	for u >= 0x80 {
		buf.WriteByte(byte(u) | 0x80)
		u >>= 7
	}
	buf.WriteByte(byte(u))
}
