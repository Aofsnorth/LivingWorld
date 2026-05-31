// Package gameplay menyediakan konstanta terkait gameplay mechanics.
package gameplay

// Combat constants
const (
	// CriticalHitMultiplier adalah multiplier untuk critical hit damage
	CriticalHitMultiplier = 1.5

	// ResistanceReductionPerLevel adalah pengurangan damage per level Resistance effect
	ResistanceReductionPerLevel = 0.2

	// MaxArmorReduction adalah maksimum armor reduction points
	MaxArmorReduction = 20.0

	// ArmorReductionDivisor adalah divisor untuk armor reduction calculation
	ArmorReductionDivisor = 25.0

	// ArmorPointsDivisor adalah divisor untuk armor points calculation
	ArmorPointsDivisor = 5.0

	// ArmorToughnessDivisor adalah divisor untuk armor toughness calculation
	ArmorToughnessDivisor = 4.0

	// ArmorToughnessBase adalah base value untuk armor toughness calculation
	ArmorToughnessBase = 2.0
)

// Player health constants
const (
	// MaxHealth adalah health maksimum player
	MaxHealth = 20.0

	// MaxFood adalah food level maksimum
	MaxFood = 20

	// DefaultSaturation adalah saturation default saat spawn
	DefaultSaturation = 5.0
)
