package mobs

import "testing"

// TestM7_XPRewardFor_HostileAreFive drives the hostile mob XP
// table (5 for the standard 1.20 set). Iron golem is the only
// mob that drops zero.
func TestM7_XPRewardFor_HostileAreFive(t *testing.T) {
	fiveMobs := []string{
		"minecraft:zombie", "minecraft:skeleton", "minecraft:creeper",
		"minecraft:husk", "minecraft:zombie_villager", "minecraft:drowned",
		"minecraft:stray", "minecraft:bogged", "minecraft:cave_spider",
		"minecraft:spider", "minecraft:enderman", "minecraft:witch",
		"minecraft:piglin", "minecraft:phantom", "minecraft:wither_skeleton",
	}
	for _, name := range fiveMobs {
		if got := XPRewardFor(name); got != 5 {
			t.Errorf("XPRewardFor(%q) = %d, want 5", name, got)
		}
	}
}

// TestM7_XPRewardFor_BlazeGhastTen asserts the heavy hitters
// (blaze, ghast) yield 10 XP — the 1.20 vanilla reward.
func TestM7_XPRewardFor_BlazeGhastTen(t *testing.T) {
	if got := XPRewardFor("minecraft:blaze"); got != 10 {
		t.Errorf("blaze XP = %d, want 10", got)
	}
	if got := XPRewardFor("minecraft:ghast"); got != 10 {
		t.Errorf("ghast XP = %d, want 10", got)
	}
}

// TestM7_XPRewardFor_IronGolemZero asserts the iron golem
// (player-spawned or village spawn) drops no XP.
func TestM7_XPRewardFor_IronGolemZero(t *testing.T) {
	if got := XPRewardFor("minecraft:iron_golem"); got != 0 {
		t.Errorf("iron_golem XP = %d, want 0", got)
	}
}

// TestM7_XPRewardFor_UnknownIsZero asserts unknown mob types
// return zero (no orb drops) so a typo in a future mob
// definition doesn't spawn orbs the player can't earn.
func TestM7_XPRewardFor_UnknownIsZero(t *testing.T) {
	for _, name := range []string{"", "minecraft:nonexistent", "minecraft:zombified_piglin"} {
		if got := XPRewardFor(name); got != 0 {
			t.Errorf("XPRewardFor(%q) = %d, want 0", name, got)
		}
	}
}
