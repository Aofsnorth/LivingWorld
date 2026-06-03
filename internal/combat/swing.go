// M7: swing damage math. Vanilla attack damage = base weapon damage +
// (1 + enchantment/strength levels) + critical multiplier. For v1 we
// only model the base weapon damage (no enchantment / strength yet —
// the inventory side is in Phase 4e). Sword item ids match the
// canonical Java 1.21 set used by the bridges' held-item packets
// (internal/java/server/mobdata.go).

package combat

// Vanilla sword item ids. Match the M4 wired ids in
// third_party/go-mc/data/item/item.go (WoodenSword=912, StoneSword=922,
// GoldenSword=927, IronSword=932, DiamondSword=937, NetheriteSword=942).
const (
	ItemWoodenSword    int32 = 912
	ItemStoneSword     int32 = 922
	ItemGoldenSword    int32 = 927
	ItemIronSword      int32 = 932
	ItemDiamondSword   int32 = 937
	ItemNetheriteSword int32 = 942
	ItemShield         int32 = 1296
)

// SwordDamage returns the base attack damage (in HP) for a sword item
// id, or 1.0 for a bare hand / unknown item. Values are vanilla 1.20
// AttackDamage attribute + 1 (the +1 is the base player-fist damage
// that every weapon swings alongside its material bonus).
func SwordDamage(itemID int32) float64 {
	switch itemID {
	case ItemWoodenSword:
		return 5 // wood: 4 + 1
	case ItemStoneSword:
		return 6 // stone: 5 + 1
	case ItemGoldenSword:
		return 5 // gold: 4 + 1
	case ItemIronSword:
		return 7 // iron: 6 + 1
	case ItemDiamondSword:
		return 8 // diamond: 7 + 1
	case ItemNetheriteSword:
		return 9 // netherite: 8 + 1
	}
	return 1 // bare hand / unknown
}

// IsSwordItem reports whether the item id is one of the 6 sword
// types. Used by the routeAttack path to gate the sweep attack
// (only swords sweep in vanilla — the Sweeping Edge enchantment
// applies to swords only).
func IsSwordItem(itemID int32) bool {
	switch itemID {
	case ItemWoodenSword, ItemStoneSword, ItemGoldenSword,
		ItemIronSword, ItemDiamondSword, ItemNetheriteSword:
		return true
	}
	return false
}

// IsShieldItem reports whether the item id is a shield. The
// shield is held in the off-hand and blocks incoming melee damage
// when the defender is sneaking (vanilla: blocking flag).
// v1 only checks the item id; the off-hand slot and sneaking
// flag are not yet tracked — any active shield blocks 100% of
// the swing. A future v2 will gate on OffhandItem + Sneaking.
func IsShieldItem(itemID int32) bool {
	return itemID == ItemShield
}

// AttackSwing is the result of resolving one player melee swing
// before the damage is applied to the target. The bridge builds
// this from the attacker's held item + the target's armor, then
// passes it to the mob store (or to the player damage path).
type AttackSwing struct {
	// BaseDamage is the pre-armor damage. SwordDamage(held) for
	// sword swings, 1.0 for bare hand.
	BaseDamage float64
	// IsSweep is true when the attacker is holding a sword AND
	// the swing lands at full charge (no cooldown in effect).
	// A true value asks the caller to fan out sweep damage to
	// nearby mobs.
	IsSweep bool
	// IsCritical is true when the swing meets the vanilla
	// critical conditions (attacker falling, not on ground, not
	// sprinting). v1: never set — M5.x reserved the 1.5× math
	// but does not detect fall/sprint state. The field is here
	// so future M-phase work can flip it without changing the
	// AttackSwing surface.
	IsCritical bool
	// Sprinting doubles knockback (vanilla). v1: not read.
	Sprinting bool
}

// FinalDamage applies armor / resistance / critical multipliers and
// returns the HP to subtract from the target. isPlayer=true
// enables the armor formula (AfterArmor); false skips armor for
// the mob path (mobs don't wear armor in v1).
func (s AttackSwing) FinalDamage(armorPoints, armorToughness float64, resistanceLevel int, isPlayer bool) float64 {
	dmg := s.BaseDamage
	if s.IsCritical {
		dmg = Critical(dmg)
	}
	if isPlayer {
		dmg = AfterArmor(dmg, armorPoints, armorToughness)
	}
	if resistanceLevel > 0 {
		dmg = AfterResistance(dmg, resistanceLevel)
	}
	return dmg
}

// SweepDamage is the per-target damage a sweep attack applies.
// Vanilla: 50% of the base sword damage × 1 + Sweeping Edge level.
// For v1 (no enchantment data) we use 50% of the base. Each
// nearby mob (within 1 block horizontal, vanilla's sweep radius)
// takes this much.
func SweepDamage(base float64) float64 {
	return base * 0.5
}

// IFramesTicks is the duration of the invulnerability window after
// a hit, in 20 Hz ticks. Vanilla: 20 ticks (1 second). Used by the
// player-side damage gate; per-mob I-frames are M5.x-deferred.
const IFramesTicks = 20
