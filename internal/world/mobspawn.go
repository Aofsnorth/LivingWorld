package world

import (
	"math"
	"math/rand"

	"livingworld/internal/mobs"
	"livingworld/internal/shared/constants/gameplay"
)

// M2 — Natural spawning director.
//
// The director runs at ~4 Hz (every 5 ticks) inside the unified
// tick loop. It picks a candidate column in a 24..44 block shell
// around each online player, then evaluates every mob's
// SpawnRule against the column's block / light / sky /
// dimension values. Mobs whose rule matches are pooled;
// a random pick is then spawned (subject to the global cap and
// the per-mob cap).
//
// Reference: vanilla Minecraft §M.SPAWN. The director
// simplifies the rules to:
//   - Dimension whitelist (one of: overworld, nether, end).
//   - Time-of-day window (NightOnly / DayOnly).
//   - Light level (sky + block light) bounded by MinLight / MaxLight.
//   - Surface block at the candidate's feet (whitelist).
//   - RequireDark (sky light = 0) and RequireOpenSky (8+ cells
//     of open air above) for cave_spider and ghast.
//   - RequireSkyLight15 for phantom.
//
// Difficulty gating:
//   - Peaceful suppresses all hostile spawns (passive + neutral
//     still allowed). The mob's Category drives this.
//   - Easy / Normal / Hard: identical for v1 (vanilla scales
//     pack size and per-mob chance, deferred to M2.2).

// Spawn-mode identifiers. The director replicates either the Java Edition
// model (per-category caps scaled by loaded-chunk count, surface-Y spawning,
// internal light = max(block,sky)) or the Bedrock Edition model (one static
// global cap, 3D shell-Y spawning around the player, hostile light = block
// light only). Selected server-wide via Manager.SetSpawnMode.
const (
	spawnModeJava    = "java"
	spawnModeBedrock = "bedrock"
)

// Global mob caps (Java mode). Vanilla scales these by loaded-chunk count
// (per-category monster density): cap = base × eligibleChunks / 289. We treat
// the constants below as the per-289-chunk base and scale at spawn time.
const (
	capPassive = 10 // vanilla creature cap (base)
	capHostile = 70 // vanilla monster cap (base)
	capNeutral = 8

	// jeChunksPerCapUnit is vanilla's 17×17 = 289 spawn-eligible-chunk
	// denominator the global cap is divided by.
	jeChunksPerCapUnit = 289

	// beStaticCap is the Bedrock Edition single global cap for
	// environment-spawned mobs (vanilla 200).
	beStaticCap = 200

	// beShellMin/beShellMax bound the Bedrock vertical shell (blocks above
	// and below the player) sampled when picking a spawn Y. Vanilla spawns
	// in a 24..128 shell; we sample ±beShellMax around the player's feet.
	beShellMin = 24
	beShellMax = 64
)

// Candidate-column shell radii (blocks) around a player: outside
// the immediate no-spawn ring, inside the 128-block despawn radius.
// Vanilla: hostile mobs spawn in 24..128 block shell.
const (
	spawnMinRadius = 24
	spawnMaxRadius = 128
	// Max attempts per director call. Vanilla runs one spawn cycle
	// per chunk per tick; we approximate with per-tick attempts.
	spawnAttemptsPerTick = 10
	// openSkyCheckCells is how many cells above the spawn
	// column the director checks for "open sky" (ghast).
	openSkyCheckCells = 8
	// packMaxSize is the maximum mobs per pack spawn. Vanilla: 4
	// for most mobs, up to 8 for wolves/cod/tropical fish.
	packMaxSize = 4
	// packSpreadBlocks is the ±5 block triangular distribution
	// radius for pack member offsets (vanilla).
	packSpreadBlocks = 5
)

// pickSpawnColumn chooses a random column in the shell around a
// random player anchor. ok=false when there are no anchors
// (nobody online → no spawning, also avoids generating
// chunks with nobody online).
func (m *Manager) pickSpawnColumn(rng *rand.Rand, anchors []Position) (x, z int, ok bool) {
	if len(anchors) == 0 {
		return 0, 0, false
	}
	p := anchors[rng.Intn(len(anchors))]
	d := spawnMinRadius + rng.Float64()*(spawnMaxRadius-spawnMinRadius)
	theta := rng.Float64() * 2 * math.Pi
	x = int(p.X) + int(d*math.Cos(theta))
	z = int(p.Z) + int(d*math.Sin(theta))
	return x, z, true
}

// hasHeadroom reports a 2-block air gap at (x,y,z) so a mob fits.
func hasHeadroom(w *World, x, y, z int) bool {
	return w.GetBlock(x, y, z).ID() == AirID && w.GetBlock(x, y+1, z).ID() == AirID
}

// hasOpenSky returns true if the cell column above (x, z) is
// open air for at least openSkyCheckCells blocks. Used by the
// ghast rule.
func hasOpenSky(w *World, x, y, z int) bool {
	for i := 1; i <= openSkyCheckCells; i++ {
		if w.GetBlock(x, y+i, z).ID() != AirID {
			return false
		}
	}
	return true
}

// lightLevel returns the max of sky and block light at (x, y, z).
// 0..15.
func lightLevel(w *World, x, y, z int) uint8 {
	sky := w.GetSkyLight(x, y, z)
	block := w.GetBlockLight(x, y, z)
	if sky > block {
		return sky
	}
	return block
}

// isNight reports whether dayTime is in the hostile-spawn
// window (dusk→dawn).
func isNight(dayTime int64) bool {
	t := dayTime % 24000
	return t >= 13000 && t < 23000
}

// isDay reports whether dayTime is in the passive-spawn
// window (sunrise→dusk).
func isDay(dayTime int64) bool {
	return !isNight(dayTime)
}

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

// surfaceName is the namespaced block at (x, y-1, z) — the
// block the mob will stand on. We compare against the rule's
// Surfaces whitelist.
func surfaceName(w *World, x, y, z int) string {
	return StateName(w.GetBlock(x, y-1, z).ID())
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

// spawnTick is the director, called at ~4 Hz from the unified
// tick loop. For each attempt:
//  1. pick a candidate column
//  2. check headroom + light bounds (early reject)
//  3. build a pool of mobs whose rule matches the column AND
//     the global cap AND the per-mob cap
//  4. roll a weighted pick from the pool (rule.Chance weights
//     the entry; default 1.0)
//  5. spawn via mobs.Store.Spawn
//
// The director is cheap (a few rule checks per attempt) so
// running it every 5 ticks is well within budget.
func (m *Manager) spawnTick(rng *rand.Rand) {
	anchors := m.playerAnchors()
	if len(anchors) == 0 {
		return
	}
	m.mu.RLock()
	difficulty := m.difficulty
	mode := m.spawnMode
	m.mu.RUnlock()
	if mode != spawnModeBedrock {
		mode = spawnModeJava
	}
	w := m.GetDefaultWorld()
	dayTime := w.GetDayTime()
	counts := m.mobCategoryCounts()
	defs := mobs.SpawnDefList()

	// Edition-specific cap model. Java scales per-category caps by the number
	// of loaded chunks (cap = base × chunks / 289). Bedrock uses a single
	// static 200 cap shared across all environment-spawned mobs.
	caps := m.effectiveCaps(w, mode)
	beTotal := counts[mobs.SpawnPassive] + counts[mobs.SpawnHostile] + counts[mobs.SpawnNeutral]

	for i := 0; i < spawnAttemptsPerTick; i++ {
		if mode == spawnModeBedrock && beTotal >= beStaticCap {
			return // BE global cap reached
		}
		x, z, ok := m.pickSpawnColumn(rng, anchors)
		if !ok {
			return
		}
		// Y sampling differs by edition: Java spawns on the surface column;
		// Bedrock samples a Y inside the vertical shell around the player so
		// mobs can appear in caves above/below the player, not just on top.
		y := w.HighestSolidY(x, z)
		if mode == spawnModeBedrock {
			if sy, ok := m.pickBedrockShellY(w, rng, anchors, x, z); ok {
				y = sy
			}
		}
		if !hasHeadroom(w, x, y, z) {
			continue
		}
		// 24-block player exclusion: vanilla does not allow spawning
		// within 24 blocks of any player (spherical distance).
		tooClose := false
		for _, a := range anchors {
			dx, dz := float64(x)-a.X+0.5, float64(z)-a.Z+0.5
			dy := float64(y) - a.Y
			if dx*dx+dy*dy+dz*dz < 24*24 {
				tooClose = true
				break
			}
		}
		if tooClose {
			continue
		}
		// Build the candidate pool for this column.
		type cand struct {
			mobType string
			rule    *mobs.SpawnRule
			weight  float32
		}
		pool := make([]cand, 0, len(defs))
		for _, d := range defs {
			rule := d.Spawn
			if !evaluateSpawnRuleMode(w, x, y, z, rule, dayTime, mode) {
				continue
			}
			if !difficultyAllowsCategory(difficulty, rule.Category) {
				continue
			}
			if counts[rule.Category] >= caps[rule.Category] {
				continue
			}
			if rule.Cap > 0 && m.mobTypeCount(d.Type) >= rule.Cap {
				continue
			}
			weight := rule.Chance
			if weight <= 0 {
				weight = 1.0
			}
			pool = append(pool, cand{mobType: d.Type, rule: rule, weight: weight})
		}
		if len(pool) == 0 {
			continue
		}
		var total float32
		for _, c := range pool {
			total += c.weight
		}
		roll := rng.Float32() * total
		var pick string
		var pickedCat mobs.SpawnCategory
		var acc float32
		for _, c := range pool {
			acc += c.weight
			if roll <= acc {
				pick = c.mobType
				pickedCat = c.rule.Category
				break
			}
		}
		if pick == "" {
			continue
		}
		// Pack spawning: spawn 1..packMaxSize mobs of the same type
		// near the initial position (vanilla: ±5 blocks triangular).
		packSize := 1 + rng.Intn(packMaxSize)
		for j := 0; j < packSize; j++ {
			px, pz := x, z
			if j > 0 {
				// Offset pack members with triangular distribution
				// within ±packSpreadBlocks of the initial position.
				px = x + int(math.Round((rng.Float64()-rng.Float64())*packSpreadBlocks))
				pz = z + int(math.Round((rng.Float64()-rng.Float64())*packSpreadBlocks))
			}
			py := w.HighestSolidY(px, pz)
			if mode == spawnModeBedrock {
				py = y // keep pack members at the sampled shell Y band
			}
			if !hasHeadroom(w, px, py, pz) {
				continue
			}
			// Check cap again for each pack member (per-category for JE, the
			// shared static total for BE).
			if mode == spawnModeBedrock {
				if beTotal >= beStaticCap {
					break
				}
			} else if counts[pickedCat] >= caps[pickedCat] {
				break
			}
			m.mobs.Spawn(pick, float64(px)+0.5, float64(py), float64(pz)+0.5)
			counts[pickedCat]++
			beTotal++
		}
	}
}

// effectiveCaps returns the per-category mob caps for the given spawn mode.
// Java scales the vanilla base caps by the loaded-chunk count (cap = base ×
// chunks / 289, floored at the base so a tiny world still spawns something).
// Bedrock returns the single static 200 cap for every category — the shared
// total is enforced separately in spawnTick via beTotal.
func (m *Manager) effectiveCaps(w *World, mode string) map[mobs.SpawnCategory]int {
	if mode == spawnModeBedrock {
		return map[mobs.SpawnCategory]int{
			mobs.SpawnPassive: beStaticCap,
			mobs.SpawnHostile: beStaticCap,
			mobs.SpawnNeutral: beStaticCap,
		}
	}
	chunks := len(w.loadedChunkPositions())
	scale := func(base int) int {
		c := base * chunks / jeChunksPerCapUnit
		if c < base {
			return base // floor so small worlds still populate
		}
		return c
	}
	return map[mobs.SpawnCategory]int{
		mobs.SpawnPassive: scale(capPassive),
		mobs.SpawnHostile: scale(capHostile),
		mobs.SpawnNeutral: scale(capNeutral),
	}
}

// pickBedrockShellY samples a spawn Y inside the Bedrock vertical shell around
// the nearest player anchor to (x, z): a random offset in ±beShellMax that is
// at least beShellMin blocks from the player vertically, clamped to a solid
// floor with headroom. ok=false when no valid Y is found in a few tries.
func (m *Manager) pickBedrockShellY(w *World, rng *rand.Rand, anchors []Position, x, z int) (int, bool) {
	// Nearest anchor by horizontal distance gives the shell centre.
	var anchorY float64
	best := math.MaxFloat64
	for _, a := range anchors {
		dx, dz := a.X-float64(x), a.Z-float64(z)
		if d := dx*dx + dz*dz; d < best {
			best, anchorY = d, a.Y
		}
	}
	for try := 0; try < 6; try++ {
		off := beShellMin + rng.Intn(beShellMax-beShellMin+1)
		if rng.Intn(2) == 0 {
			off = -off
		}
		y := int(anchorY) + off
		if y <= MinWorldHeight || y >= MaxWorldHeight-2 {
			continue
		}
		// Need a solid block below and 2-air headroom (a standable ledge).
		if w.GetBlock(x, y-1, z).ID() != AirID && hasHeadroom(w, x, y, z) {
			return y, true
		}
	}
	return 0, false
}

// capForCategory returns the global cap for the given spawn category.
func capForCategory(cat mobs.SpawnCategory) int {
	switch cat {
	case mobs.SpawnPassive:
		return capPassive
	case mobs.SpawnHostile:
		return capHostile
	case mobs.SpawnNeutral:
		return capNeutral
	default:
		return 0
	}
}
