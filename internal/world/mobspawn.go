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

// Global mob caps. Vanilla scales these by loaded-chunk count
// (per-category monster density). For v1 we use flat numbers
// that fit a small playtest; M2.2 should re-derive from
// the number of loaded chunks.
const (
	capPassive = 12
	capHostile = 18
	capNeutral = 8
)

// Candidate-column shell radii (blocks) around a player: outside
// the immediate no-spawn ring, inside simulation distance.
const (
	spawnMinRadius = 24
	spawnMaxRadius = 44
	// Max attempts per director call. The director picks up
	// to this many candidate columns per tick.
	spawnAttemptsPerTick = 4
	// openSkyCheckCells is how many cells above the spawn
	// column the director checks for "open sky" (ghast).
	openSkyCheckCells = 8
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
	m.mu.RUnlock()
	w := m.GetDefaultWorld()
	dayTime := w.GetDayTime()
	counts := m.mobCategoryCounts()
	defs := mobs.SpawnDefList()
	for i := 0; i < spawnAttemptsPerTick; i++ {
		x, z, ok := m.pickSpawnColumn(rng, anchors)
		if !ok {
			return
		}
		y := w.HighestSolidY(x, z)
		if !hasHeadroom(w, x, y, z) {
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
			if !evaluateSpawnRule(w, x, y, z, rule, dayTime) {
				continue
			}
			if !difficultyAllowsCategory(difficulty, rule.Category) {
				continue
			}
			// Global cap (per category).
			switch rule.Category {
			case mobs.SpawnPassive:
				if counts[mobs.SpawnPassive] >= capPassive {
					continue
				}
			case mobs.SpawnHostile:
				if counts[mobs.SpawnHostile] >= capHostile {
					continue
				}
			case mobs.SpawnNeutral:
				if counts[mobs.SpawnNeutral] >= capNeutral {
					continue
				}
			}
			// Per-mob cap.
			if rule.Cap > 0 && m.mobTypeCount(d.Type) >= rule.Cap {
				continue
			}
			// Weight (default 1.0).
			weight := rule.Chance
			if weight <= 0 {
				weight = 1.0
			}
			pool = append(pool, cand{mobType: d.Type, rule: rule, weight: weight})
		}
		if len(pool) == 0 {
			continue
		}
		// Weighted pick.
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
		m.mobs.Spawn(pick, float64(x)+0.5, float64(y), float64(z)+0.5)
		counts[pickedCat]++
	}
}
