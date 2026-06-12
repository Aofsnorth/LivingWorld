// Package ore is the ore-vein pass. Vanilla's ore pipeline runs two
// distinct passes:
//
//   1. "Ore veins" — copper and iron veins laid down in the
//      underground before the surface pass.
//   2. "Ore blobs" — diamond, coal, redstone, lapis, gold, emerald —
//      the "replace stone with ore" post-pass that runs AFTER carvers
//      but BEFORE decoration.
//
// This package models both. The vein pass (vein.go) is the
// vein_toggle / vein_ridged / vein_gap density field interpretation;
// the blob pass (blob.go) is the per-ore "N tries per chunk, replace
// stone in a small ellipsoid" loop.
package ore

// Config describes a single ore type. VeinTries is the number of
// candidate vein / blob anchors per chunk; VeinSize is the radius of
// the ellipsoid. ThresholdFloat is the per-block probability — higher
// means rarer.
type Config struct {
	BlockName      string
	MinY, MaxY     int
	VeinTries      int
	VeinSize       int
	ThresholdFloat float64
	MountainOnly   bool
}

// AllOverworldOres returns the canonical ore list for the Overworld.
// Order matters: the pipeline walks the list in order, and diamond /
// emerald must be last so they don't get overwritten by coal blobs in
// the same chunk.
func AllOverworldOres() []Config {
	return []Config{
		{BlockName: "minecraft:coal_ore", MinY: -64, MaxY: 320, VeinTries: 20, VeinSize: 8, ThresholdFloat: 0.40},
		{BlockName: "minecraft:iron_ore", MinY: -64, MaxY: 320, VeinTries: 12, VeinSize: 6, ThresholdFloat: 0.45},
		{BlockName: "minecraft:gold_ore", MinY: -64, MaxY: 32, VeinTries: 6, VeinSize: 6, ThresholdFloat: 0.55},
		{BlockName: "minecraft:redstone_ore", MinY: -64, MaxY: 16, VeinTries: 6, VeinSize: 6, ThresholdFloat: 0.55},
		{BlockName: "minecraft:diamond_ore", MinY: -64, MaxY: 16, VeinTries: 4, VeinSize: 5, ThresholdFloat: 0.65},
		{BlockName: "minecraft:lapis_ore", MinY: -64, MaxY: 64, VeinTries: 3, VeinSize: 5, ThresholdFloat: 0.60},
		{BlockName: "minecraft:emerald_ore", MinY: -16, MaxY: 320, VeinTries: 2, VeinSize: 1, ThresholdFloat: 0.75, MountainOnly: true},
	}
}
