package server

import (
	"testing"

	"livingworld/internal/mobs"

	"github.com/Tnze/go-mc/data/entity"
	"github.com/Tnze/go-mc/data/item"
	pk "github.com/Tnze/go-mc/net/packet"
)

// TestM3_ProjectileTypeID verifies the Java AddEntity type-id
// routing for all 7 M1 projectile kinds. Each kind maps to a
// distinct Java entity type id so the client renders the right
// visual. Tipped arrow variants reuse SpectralArrow (M1.6 will
// ship a follow-up metadata packet for the actual potion effect).
func TestM3_ProjectileTypeID(t *testing.T) {
	cases := []struct {
		kind string
		want int32
	}{
		{mobs.ProjectileArrow, int32(entity.Arrow.ID)},
		{mobs.ProjectileArrowSlowness, int32(entity.SpectralArrow.ID)},
		{mobs.ProjectileArrowPoison, int32(entity.SpectralArrow.ID)},
		{mobs.ProjectileSmallFireball, int32(entity.SmallFireball.ID)},
		{mobs.ProjectileLargeFireball, int32(entity.Fireball.ID)},
		{mobs.ProjectileTrident, int32(entity.Trident.ID)},
		{mobs.ProjectilePotion, int32(entity.SplashPotion.ID)},
	}
	for _, c := range cases {
		got := javaProjectileTypeID(mobs.Projectile{Kind: c.kind})
		if got != c.want {
			t.Errorf("javaProjectileTypeID(kind=%q) = %d; want %d", c.kind, got, c.want)
		}
	}
	// Unknown kind must fall back to Arrow, not panic.
	if got := javaProjectileTypeID(mobs.Projectile{Kind: "wither_skull"}); got != int32(entity.Arrow.ID) {
		t.Errorf("unknown kind fallback: got %d want %d", got, entity.Arrow.ID)
	}
	if got := javaProjectileTypeID(mobs.Projectile{}); got != int32(entity.Arrow.ID) {
		t.Errorf("empty kind fallback: got %d want %d", got, entity.Arrow.ID)
	}
}

// TestM3_MobDataVarInt_SlimeSize verifies the AddEntity data
// varint for slime / magma_cube encodes the size index. Vanilla
// size 1 → 0, size 2 → 1, size 3 → 2, size 4 → 3. Larger sizes
// are clamped to 3. Other mob types always return 0.
func TestM3_MobDataVarInt_SlimeSize(t *testing.T) {
	cases := []struct {
		mobType string
		size    int
		want    int32
	}{
		// Slime: size 1 (M1 default) → 0
		{"minecraft:slime", 1, 0},
		// Split child: size 2 → 1
		{"minecraft:slime", 2, 1},
		// Split child: size 4 → 3
		{"minecraft:slime", 4, 3},
		// Above vanilla max clamps to 3
		{"minecraft:slime", 5, 3},
		{"minecraft:slime", 99, 3},
		// Below 1 (shouldn't happen but defensive) → 0
		{"minecraft:slime", 0, 0},
		{"minecraft:slime", -1, 0},
		// Magma cube — same encoding
		{"minecraft:magma_cube", 1, 0},
		{"minecraft:magma_cube", 3, 2},
		// Other mob types: always 0 regardless of Size
		{"minecraft:zombie", 5, 0},
		{"minecraft:skeleton", 5, 0},
		{"minecraft:enderman", 5, 0},
		{"minecraft:phantom", 5, 0},
		{"minecraft:pig", 5, 0},
		{"", 5, 0},
	}
	for _, c := range cases {
		m := mobs.Mob{Type: c.mobType, Size: c.size}
		got := mobDataVarInt(m)
		if got != c.want {
			t.Errorf("mobDataVarInt(type=%q, size=%d) = %d; want %d", c.mobType, c.size, got, c.want)
		}
	}
}

// TestM3_IsArrowKind sanity-checks the arrow-vs-fireball switch
// used for orientation routing on the Java side.
func TestM3_IsArrowKind(t *testing.T) {
	arrowKinds := []string{
		mobs.ProjectileArrow,
		mobs.ProjectileArrowSlowness,
		mobs.ProjectileArrowPoison,
		mobs.ProjectileTrident,
	}
	nonArrowKinds := []string{
		mobs.ProjectileSmallFireball,
		mobs.ProjectileLargeFireball,
		mobs.ProjectilePotion,
		"unknown_kind",
		"",
	}
	for _, k := range arrowKinds {
		if !isArrowKind(k) {
			t.Errorf("isArrowKind(%q) = false; want true", k)
		}
	}
	for _, k := range nonArrowKinds {
		if isArrowKind(k) {
			t.Errorf("isArrowKind(%q) = true; want false", k)
		}
	}
}

// --- M4: held item / SetEntityData ---

// TestM4_JavaHeldItem covers the per-mob held item lookup. All
// humanoid mob variants in the 16-mob M1 roster that carry an
// item in vanilla (skeleton / stray / bogged / drowned /
// wither_skeleton / piglin / iron_golem) must map to the right
// vanilla item id. Mobs with no held item return (0, 0).
func TestM4_JavaHeldItem(t *testing.T) {
	cases := []struct {
		mobType  string
		wantID   int32
		wantCnt  int8
	}{
		{"minecraft:skeleton", int32(item.Bow.ID), 1},
		{"minecraft:stray", int32(item.Bow.ID), 1},
		{"minecraft:bogged", int32(item.Bow.ID), 1},
		{"minecraft:drowned", int32(item.Trident.ID), 1},
		{"minecraft:wither_skeleton", int32(item.StoneSword.ID), 1},
		{"minecraft:piglin", int32(item.GoldenSword.ID), 1},
		{"minecraft:iron_golem", int32(item.Poppy.ID), 1},
		// Non-humanoid mobs have no held item.
		{"minecraft:zombie", 0, 0},
		{"minecraft:husk", 0, 0},
		{"minecraft:zombie_villager", 0, 0},
		{"minecraft:creeper", 0, 0},
		{"minecraft:spider", 0, 0},
		{"minecraft:cave_spider", 0, 0},
		{"minecraft:slime", 0, 0},
		{"minecraft:magma_cube", 0, 0},
		{"minecraft:phantom", 0, 0},
		{"minecraft:blaze", 0, 0},
		{"minecraft:ghast", 0, 0},
		{"minecraft:witch", 0, 0},
		{"minecraft:enderman", 0, 0},
		{"minecraft:zombie", 0, 0},
		{"minecraft:pig", 0, 0},
		{"minecraft:cow", 0, 0},
		{"", 0, 0},
	}
	for _, c := range cases {
		m := mobs.Mob{Type: c.mobType}
		gotID, gotCnt := javaHeldItem(m)
		if gotID != c.wantID || gotCnt != c.wantCnt {
			t.Errorf("javaHeldItem(%q) = (%d, %d); want (%d, %d)",
				c.mobType, gotID, gotCnt, c.wantID, c.wantCnt)
		}
	}
}

// TestM4_MobHeldItemPackets verifies the helper that decides
// whether to send a SetEntityData packet. Mobs with a held item
// must return one packet; mobs without must return nil.
func TestM4_MobHeldItemPackets(t *testing.T) {
	// Skeleton: 1 packet.
	pk1 := mobHeldItemPackets(mobs.Mob{EntityID: 1, Type: "minecraft:skeleton"})
	if len(pk1) != 1 {
		t.Errorf("skeleton: got %d packets; want 1", len(pk1))
	}
	// Drowned: 1 packet.
	pk2 := mobHeldItemPackets(mobs.Mob{EntityID: 2, Type: "minecraft:drowned"})
	if len(pk2) != 1 {
		t.Errorf("drowned: got %d packets; want 1", len(pk2))
	}
	// Pig: 0 packets.
	pk3 := mobHeldItemPackets(mobs.Mob{EntityID: 3, Type: "minecraft:pig"})
	if pk3 != nil {
		t.Errorf("pig: got %d packets; want nil", len(pk3))
	}
	// Spider: 0 packets.
	pk4 := mobHeldItemPackets(mobs.Mob{EntityID: 4, Type: "minecraft:spider"})
	if pk4 != nil {
		t.Errorf("spider: got %d packets; want nil", len(pk4))
	}
}

// TestM4_SpawnMobDataPacket_Layout verifies the wire bytes of
// the SetEntityData packet for a representative held item. The
// vendored go-mc has no Slot Field type, so we hand-encode the
// packet and this test pins the layout. Layout (in order):
//   packet id 99  (VarInt, 1 byte: 0x63)
//   entity id     (VarInt, 1 byte for ids 0..127)
//   index 8       (UnsignedByte: 0x08)
//   type 6 (Slot) (VarInt, 1 byte: 0x06)
//   present 1     (VarInt, 1 byte: 0x01)
//   item id       (VarInt, multi-byte for item.Bow.ID = 895)
//   count 1       (Byte: 0x01)
//   NBT end       (Byte: 0x00)
//   terminator    (UnsignedByte: 0xFF)
//
// VarInt(895): 895 = 0b110_1111111
//   byte 0: 895 & 0x7F = 0x7F, set MSB (continuation) → 0xFF
//   byte 1: (895 >> 7) & 0x7F = 6 → 0x06
func TestM4_SpawnMobDataPacket_Layout(t *testing.T) {
	want := []byte{0x63, 0x01, 0x08, 0x06, 0x01, 0xFF, 0x06, 0x01, 0x00, 0xFF}

	pkt := spawnMobDataPacket(1, int32(item.Bow.ID), 1)
	got, err := pkMarshal(pkt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Errorf("wire bytes mismatch:\n got  %v\n want %v", got, want)
	}
}

// TestM4_SpawnMobDataPacket_TridentVarInt verifies the wire
// layout for the trident (item id 1332, a 2-byte VarInt).
func TestM4_SpawnMobDataPacket_TridentVarInt(t *testing.T) {
	// Trident = 1332 = 0x534 = 0b0101_0011_0100
	// VarInt: 7 bits at a time, MSB=continuation
	//   byte 0: 0x34 | 0x80 = 0xB4   (bits 0..6 + continuation)
	//   byte 1: 0x0A                  (bits 7..13)
	want := []byte{0x63, 0x02, 0x08, 0x06, 0x01, 0xB4, 0x0A, 0x01, 0x00, 0xFF}

	pkt := spawnMobDataPacket(2, int32(item.Trident.ID), 1)
	got, err := pkMarshal(pkt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytesEqual(got, want) {
		t.Errorf("trident wire bytes mismatch:\n got  %v\n want %v", got, want)
	}
}

// pkMarshal re-renders a pk.Packet to raw bytes. The vendored
// go-mc pk.Packet is a struct with public ID and Data fields;
// the data buffer is exactly the wire bytes for the packet
// payload (the packet id is in ID; the data buffer does not
// re-include it). To get the on-wire bytes we concatenate
// the VarInt-encoded ID with the Data field.
func pkMarshal(p pk.Packet) ([]byte, error) {
	buf := make([]byte, pk.VarInt(p.ID).Len())
	pk.VarInt(p.ID).WriteToBytes(buf)
	out := make([]byte, 0, len(buf)+len(p.Data))
	out = append(out, buf...)
	out = append(out, p.Data...)
	return out, nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
