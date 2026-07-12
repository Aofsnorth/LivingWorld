package world

import (
	"livingworld/internal/mobs"
	"livingworld/internal/shared/constants/gameplay"
)

// mobCategoryCounts tallies the current mob population per
// spawn category (passive / hostile / neutral).
func (m *Manager) mobCategoryCounts() map[mobs.SpawnCategory]int {
	counts := map[mobs.SpawnCategory]int{}
	for _, mob := range m.mobs.All() {
		def := mobs.DefFor(mob.Type)
		if def.Spawn == nil {
			continue
		}
		counts[def.Spawn.Category]++
	}
	return counts
}

// mobTypeCount returns how many of a given mob type are
// currently alive. Used to enforce per-mob Cap.
func (m *Manager) mobTypeCount(mobType string) int {
	n := 0
	for _, mob := range m.mobs.All() {
		if mob.Type == mobType {
			n++
		}
	}
	return n
}

// evaluateSpawnRule checks a single mob's SpawnRule against
// the candidate column. Returns true if the mob can spawn
// here.
func evaluateSpawnRule(w *World, x, y, z int, rule *mobs.SpawnRule, dayTime int64) bool {
	if rule == nil {
		return false
	}
	// Dimension check.
	if rule.Dimension != "" && rule.Dimension != string(w.Dimension()) {
		return false
	}
	// Time of day.
	if rule.NightOnly && !isNight(dayTime) {
		return false
	}
	if rule.DayOnly && !isDay(dayTime) {
		return false
	}
	// Special sky checks. These run BEFORE the light-bounds
	// check so that phantom (sky=15) can still pass even if
	// MaxLight would otherwise bound it.
	if rule.RequireDark {
		// Cave spider rule: sky light = 0 (deep cave).
		sky := w.GetSkyLight(x, y, z)
		if sky != 0 {
			return false
		}
	}
	if rule.RequireSkyLight15 {
		// Phantom rule: open night sky above the head.
		sky := w.GetSkyLight(x, y, z)
		if sky != 15 {
			return false
		}
	}
	if rule.RequireOpenSky && !hasOpenSky(w, x, y, z) {
		return false
	}
	// Light bounds. lightLevel = max(sky, block).
	ll := int(lightLevel(w, x, y, z))
	if rule.MinLight >= 0 && ll < rule.MinLight {
		return false
	}
	if rule.MaxLight >= 0 && ll > rule.MaxLight {
		return false
	}
	// Surface block whitelist.
	if len(rule.Surfaces) > 0 {
		s := surfaceName(w, x, y, z)
		match := false
		for _, allowed := range rule.Surfaces {
			if s == allowed {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}

// evaluateSpawnRuleMode is the edition-aware wrapper around evaluateSpawnRule.
// Java mode (and any unknown mode) uses the internal-light = max(block,sky)
// rule baked into evaluateSpawnRule. Bedrock mode applies the BE hostile-light
// rule instead: a hostile mob needs block light == 0 AND sky light below the
// rule's MaxLight bound (torchlight blocks BE spawns more aggressively than
// JE, which folds block + sky into one value). Non-hostile rules are identical
// across editions, so they always defer to evaluateSpawnRule.
func evaluateSpawnRuleMode(w *World, x, y, z int, rule *mobs.SpawnRule, dayTime int64, mode string) bool {
	if mode != spawnModeBedrock || rule == nil || rule.Category != mobs.SpawnHostile {
		return evaluateSpawnRule(w, x, y, z, rule, dayTime)
	}
	// Run the full JE check first for dimension/time/surface/sky-special, but
	// override the light test with BE's block-light-only rule. The simplest
	// faithful approach: temporarily evaluate everything except light by
	// reusing evaluateSpawnRule with a copy whose light bounds are relaxed,
	// then apply the BE light test ourselves.
	relaxed := *rule
	relaxed.MinLight = mobs.LightAny
	relaxed.MaxLight = mobs.LightAny
	if !evaluateSpawnRule(w, x, y, z, &relaxed, dayTime) {
		return false
	}
	block := int(w.GetBlockLight(x, y, z))
	sky := int(w.GetSkyLight(x, y, z))
	if block > 0 {
		return false // BE hostiles never spawn under any block light
	}
	if rule.MaxLight >= 0 && sky > rule.MaxLight {
		return false
	}
	return true
}

// difficultyAllowsCategory returns true if the mob's category
// can spawn at the current difficulty. Peaceful suppresses
// hostiles; easy/normal/hard are all the same for v1.
func difficultyAllowsCategory(difficulty string, cat mobs.SpawnCategory) bool {
	if cat == mobs.SpawnHostile && difficulty == gameplay.DifficultyPeaceful {
		return false
	}
	return true
}
