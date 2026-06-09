package server

import (
	"testing"

	"livingworld/internal/mobs"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// TestM3_ProjectileEntityType verifies the Bedrock AddActor
// EntityType routing for all 7 M1 projectile kinds. This is the
// contract for the visual that the Bedrock client renders when a
// skeleton / blaze / ghast / witch / drowned / pillager / phantom
// fires its projectile. Tipped arrow variants reuse minecraft:arrow
// — the effect metadata is a follow-up packet (M3.6+).
func TestM3_ProjectileEntityType(t *testing.T) {
	cases := []struct {
		kind string
		want string
	}{
		{mobs.ProjectileArrow, "minecraft:arrow"},
		{mobs.ProjectileArrowSlowness, "minecraft:arrow"},
		{mobs.ProjectileArrowPoison, "minecraft:arrow"},
		{mobs.ProjectileSmallFireball, "minecraft:small_fireball"},
		{mobs.ProjectileLargeFireball, "minecraft:fireball"},
		{mobs.ProjectileTrident, "minecraft:thrown_trident"},
		{mobs.ProjectilePotion, "minecraft:splash_potion"},
	}
	for _, c := range cases {
		got, ok := bedrockProjectileEntityType[c.kind]
		if !ok {
			t.Errorf("missing Bedrock entity type for kind=%q", c.kind)
			continue
		}
		if got != c.want {
			t.Errorf("kind=%q: got %q want %q", c.kind, got, c.want)
		}
	}
}

// TestM3_ProjectileActor_FireballUsesYaw verifies that the
// AddActor for a fireball sets Yaw/Pitch from the Projectile
// (so the visible flame ring orients toward the target), while
// arrow kinds set Yaw=0 (the client interpolates from velocity).
func TestM3_ProjectileActor_FireballUsesYaw(t *testing.T) {
	// Fireball — yaw/pitch must be propagated.
	fb := mobs.Projectile{
		EntityID: 1, X: 0, Y: 0, Z: 0,
		VX: 0.4, VY: 0, VZ: 0.3,
		Yaw:   45,
		Pitch: 30,
		Kind:  mobs.ProjectileSmallFireball,
	}
	pkt := addProjectileActor(fb)
	if pkt.EntityType != "minecraft:small_fireball" {
		t.Errorf("fireball type: got %q want minecraft:small_fireball", pkt.EntityType)
	}
	if pkt.Yaw != 45 || pkt.Pitch != 30 {
		t.Errorf("fireball yaw/pitch: got yaw=%v pitch=%v want yaw=45 pitch=30", pkt.Yaw, pkt.Pitch)
	}
	// Arrow — yaw/pitch must be 0.
	arr := mobs.Projectile{
		EntityID: 2, X: 0, Y: 0, Z: 0,
		VX: 1.6, VY: 0, VZ: 0,
		Yaw:   45,
		Pitch: 30,
		Kind:  mobs.ProjectileArrow,
	}
	pkt2 := addProjectileActor(arr)
	if pkt2.EntityType != "minecraft:arrow" {
		t.Errorf("arrow type: got %q want minecraft:arrow", pkt2.EntityType)
	}
	if pkt2.Yaw != 0 || pkt2.Pitch != 0 {
		t.Errorf("arrow yaw/pitch: got yaw=%v pitch=%v want 0/0", pkt2.Yaw, pkt2.Pitch)
	}
	// Tipped arrow — also yaw/pitch 0 (arrow-like).
	ta := mobs.Projectile{
		EntityID: 3, X: 0, Y: 0, Z: 0,
		Kind:  mobs.ProjectileArrowSlowness,
		Yaw:   99,
		Pitch: 88,
	}
	pkt3 := addProjectileActor(ta)
	if pkt3.EntityType != "minecraft:arrow" {
		t.Errorf("tipped-arrow type: got %q want minecraft:arrow", pkt3.EntityType)
	}
	if pkt3.Yaw != 0 || pkt3.Pitch != 0 {
		t.Errorf("tipped-arrow yaw/pitch: got yaw=%v pitch=%v want 0/0", pkt3.Yaw, pkt3.Pitch)
	}
	// Unknown kind — must fall back to minecraft:arrow, not panic.
	un := mobs.Projectile{EntityID: 4, Kind: "wither_skull", Yaw: 12}
	pkt4 := addProjectileActor(un)
	if pkt4.EntityType != "minecraft:arrow" {
		t.Errorf("unknown-kind type: got %q want minecraft:arrow (fallback)", pkt4.EntityType)
	}
	if pkt4.Yaw != 12 {
		t.Errorf("unknown-kind yaw (non-arrow): got %v want 12 (propagated)", pkt4.Yaw)
	}
}

// TestM3_ProjectileActor_PotionOrientation verifies the splash
// potion kind — used by witch — routes to minecraft:splash_potion
// and uses the projectile's yaw.
func TestM3_ProjectileActor_PotionOrientation(t *testing.T) {
	p := mobs.Projectile{
		EntityID: 10, X: 5, Y: 64, Z: 5,
		Kind:  mobs.ProjectilePotion,
		Yaw:   180,
		Pitch: -10,
	}
	pkt := addProjectileActor(p)
	if pkt.EntityType != "minecraft:splash_potion" {
		t.Errorf("potion type: got %q want minecraft:splash_potion", pkt.EntityType)
	}
	if pkt.Yaw != 180 || pkt.Pitch != -10 {
		t.Errorf("potion yaw/pitch: got yaw=%v pitch=%v want 180/-10", pkt.Yaw, pkt.Pitch)
	}
}

// TestM3_ProjectileActor_TridentUsesYaw verifies the trident
// routes to minecraft:thrown_trident but is treated as arrow-like
// for orientation (yaw=0; client interpolates).
func TestM3_ProjectileActor_TridentUsesYaw(t *testing.T) {
	p := mobs.Projectile{
		EntityID: 20, X: 0, Y: 0, Z: 0,
		Kind:  mobs.ProjectileTrident,
		Yaw:   270,
		Pitch: 15,
	}
	pkt := addProjectileActor(p)
	if pkt.EntityType != "minecraft:thrown_trident" {
		t.Errorf("trident type: got %q want minecraft:thrown_trident", pkt.EntityType)
	}
	if pkt.Yaw != 0 || pkt.Pitch != 0 {
		t.Errorf("trident yaw/pitch: got yaw=%v pitch=%v want 0/0 (arrow-like)", pkt.Yaw, pkt.Pitch)
	}
}

// TestM3_IsBedrockArrowKind sanity-checks the arrow-vs-fireball
// switch used for orientation routing.
func TestM3_IsBedrockArrowKind(t *testing.T) {
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
		if !isBedrockArrowKind(k) {
			t.Errorf("isBedrockArrowKind(%q) = false; want true", k)
		}
	}
	for _, k := range nonArrowKinds {
		if isBedrockArrowKind(k) {
			t.Errorf("isBedrockArrowKind(%q) = true; want false", k)
		}
	}
}

// --- M4: Bedrock EntityMetadata for slime size + drowned variant ---

// TestM4_MobEntityMetadata_SlimeSize covers the per-mob
// EntityMetadata. The key is EntityDataKeyVariant (key 2) for
// slime / magma_cube. Bedrock's variant index is 0-based:
// 0=small (size 1), 1=medium (size 2), 2=large (size 3),
// 3=huge (size 4). Without this metadata the Bedrock client
// renders a default-size slime regardless of Mob.Size.
func TestM4_MobEntityMetadata_SlimeSize(t *testing.T) {
	cases := []struct {
		mobType string
		size    int
		want    int32
	}{
		// Vanilla size 1 (M1 default for slime) → variant 0
		{"minecraft:slime", 1, 0},
		// Slime split child: size 2 → variant 1
		{"minecraft:slime", 2, 1},
		// Slime split child: size 3 → variant 2
		{"minecraft:slime", 3, 2},
		// Slime split child: size 4 → variant 3 (huge)
		{"minecraft:slime", 4, 3},
		// Magma cube — same encoding
		{"minecraft:magma_cube", 1, 0},
		{"minecraft:magma_cube", 2, 1},
		{"minecraft:magma_cube", 4, 3},
		// Above vanilla max clamps to 3 (huge)
		{"minecraft:slime", 99, 3},
		{"minecraft:slime", 5, 3},
		// Below 1 (defensive) clamps to 0 (small)
		{"minecraft:slime", 0, 0},
		{"minecraft:slime", -1, 0},
	}
	for _, c := range cases {
		m := mobs.Mob{Type: c.mobType, Size: c.size}
		md := mobEntityMetadata(m)
		got, ok := md[protocol.EntityDataKeyVariant]
		if !ok {
			t.Errorf("%s size=%d: missing variant metadata; want %d", c.mobType, c.size, c.want)
			continue
		}
		if g := got.(int32); g != c.want {
			t.Errorf("%s size=%d: variant=%d; want %d", c.mobType, c.size, g, c.want)
		}
	}
}

// TestM4_MobEntityMetadata_DrownedVariant covers the drowned
// trident switch. Variant 1 = trident, 0 = no trident (vanilla
// 85% case). v1 always uses 1 — the trade-off is documented
// in mobEntityMetadata.
func TestM4_MobEntityMetadata_DrownedVariant(t *testing.T) {
	m := mobs.Mob{Type: "minecraft:drowned"}
	md := mobEntityMetadata(m)
	got, ok := md[protocol.EntityDataKeyVariant]
	if !ok {
		t.Errorf("drowned: missing variant metadata")
		return
	}
	if g := got.(int32); g != 1 {
		t.Errorf("drowned variant: got %d want 1 (trident)", g)
	}
}

// TestM4_MobEntityMetadata_DefaultIsEmpty verifies that mob
// types without a metadata override get the default
// (NewEntityMetadata) — flags=0, flags2=0, player_flags=0 —
// which the client interprets as "use the entity-type default
// look". This is important for skeleton (auto-renders bow via
// entity model), wither_skeleton (auto-renders stone sword),
// iron_golem (auto-renders poppy), piglin (auto-renders golden
// sword), witch (no held item), enderman (no carried block in
// v1), zombie / husk / zombie_villager (no held item), spider
// / cave_spider (no held item), etc.
func TestM4_MobEntityMetadata_DefaultIsEmpty(t *testing.T) {
	cases := []string{
		"minecraft:skeleton",
		"minecraft:stray",
		"minecraft:bogged",
		"minecraft:husk",
		"minecraft:zombie_villager",
		"minecraft:wither_skeleton",
		"minecraft:piglin",
		"minecraft:iron_golem",
		"minecraft:enderman",
		"minecraft:spider",
		"minecraft:cave_spider",
		"minecraft:witch",
		"minecraft:phantom",
		"minecraft:blaze",
		"minecraft:ghast",
		"minecraft:creeper",
		"minecraft:pig",
		"minecraft:cow",
		"",
	}
	for _, mt := range cases {
		m := mobs.Mob{Type: mt}
		md := mobEntityMetadata(m)
		// Should be the default metadata: flags=0, flags2=0,
		// player_flags=0. The only thing that would override is
		// the variant key, which must be absent for these.
		if v, has := md[protocol.EntityDataKeyVariant]; has {
			t.Errorf("mob type %q: variant metadata set to %v; want absent", mt, v)
		}
	}
}

// TestM4_AddMobActor_CarriesMetadata verifies the AddActor
// packet for a slime carries the EntityMetadata map (so the
// Bedrock client picks up the size variant). Other mob types
// get the default (empty) metadata.
func TestM4_AddMobActor_CarriesMetadata(t *testing.T) {
	// Slime size 2 — must carry EntityMetadata with variant=1
	// (0-based index).
	slime := mobs.Mob{
		EntityID: 1, Type: "minecraft:slime",
		Size: 2, X: 0, Y: 0, Z: 0, Yaw: 0,
	}
	pkt := addMobActor(slime)
	if v, ok := pkt.EntityMetadata[protocol.EntityDataKeyVariant]; !ok {
		t.Errorf("slime: AddActor missing variant metadata")
	} else if v.(int32) != 1 {
		t.Errorf("slime size 2: variant=%d; want 1", v.(int32))
	}
	// Slime size 4 — variant 3 (huge).
	huge := mobs.Mob{EntityID: 11, Type: "minecraft:slime", Size: 4}
	pkt3 := addMobActor(huge)
	if v, ok := pkt3.EntityMetadata[protocol.EntityDataKeyVariant]; !ok {
		t.Errorf("huge slime: AddActor missing variant metadata")
	} else if v.(int32) != 3 {
		t.Errorf("huge slime: variant=%d; want 3", v.(int32))
	}
	// Skeleton — no metadata override.
	skel := mobs.Mob{EntityID: 2, Type: "minecraft:skeleton"}
	pkt2 := addMobActor(skel)
	if v, has := pkt2.EntityMetadata[protocol.EntityDataKeyVariant]; has {
		t.Errorf("skeleton: AddActor has variant=%v; want absent", v)
	}
}

// TestRombak_MobEntityMetadata_OnFire verifies the rombak on-fire flag is set
// in the Bedrock entity metadata when the mob is burning, and absent otherwise.
func TestRombak_MobEntityMetadata_OnFire(t *testing.T) {
	burning := mobEntityMetadata(mobs.Mob{Type: "minecraft:zombie", FireTicks: 60})
	flags, ok := burning[protocol.EntityDataKeyFlags]
	if !ok || flags.(int64)&(1<<protocol.EntityDataFlagOnFire) == 0 {
		t.Errorf("burning mob should set the on-fire flag, got flags=%v ok=%v", flags, ok)
	}
	calm := mobEntityMetadata(mobs.Mob{Type: "minecraft:zombie", FireTicks: 0})
	if f, ok := calm[protocol.EntityDataKeyFlags]; ok && f.(int64)&(1<<protocol.EntityDataFlagOnFire) != 0 {
		t.Error("non-burning mob must not set the on-fire flag")
	}
}
