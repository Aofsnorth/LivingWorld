package world

import (
	"math"
	"math/rand"

	"livingworld/internal/shared/constants/gameplay"
)

// PROBLEM #6 — Mob spawning director.
//
// Nothing ever called mobs.Store.Spawn, so the world stayed empty (StartMobAI
// only moved already-existing mobs). spawnTick runs inside the StartMobAI tick
// loop: it picks candidate columns in a shell around players, checks per-category
// caps, the surface block, and an APPROXIMATE light level, then spawns via
// mobs.Store.Spawn. Cross-edition visibility is free — both bridges already
// subscribe to the mob store's OnSpawn — and the AOI system (#9) re-spawns mobs
// as viewers approach.
//
// LIGHT IS APPROXIMATE (tech debt): there is no light-propagation engine, so we
// use heightmap coverage + day/night only. Block light (torches/lava) is NOT
// modeled, so lit areas and shallow caves can still spawn hostiles at night, and
// deep caves won't spawn them in daytime. Caps are flat globals, not vanilla's
// per-loaded-chunk scaling. Acceptable for v1; tune the constants below.

type mobCategory int

const (
	catPassive mobCategory = iota
	catHostile
)

type mobDef struct {
	Type     string
	Category mobCategory
}

// spawnTable lists the spawnable mobs. Every type here is in javaMobTypeIDs (so
// Java doesn't fall back to a pig) and is also a valid Bedrock entity identifier,
// so the same table renders on both editions with no extra mapping.
var spawnTable = []mobDef{
	{"minecraft:cow", catPassive},
	{"minecraft:pig", catPassive},
	{"minecraft:chicken", catPassive},
	{"minecraft:sheep", catPassive},
	{"minecraft:zombie", catHostile},
	{"minecraft:skeleton", catHostile},
	{"minecraft:creeper", catHostile},
}

// Flat global caps per category (tech debt: vanilla scales caps by loaded-chunk
// count; these are starting guesses to tune).
const (
	capPassive = 10
	capHostile = 15
)

// Candidate-column shell radii (blocks) around a player: outside the immediate
// no-spawn ring, inside simulation distance.
const (
	spawnMinRadius = 24
	spawnMaxRadius = 44
)

func mobCategoryOf(mobType string) (mobCategory, bool) {
	for _, d := range spawnTable {
		if d.Type == mobType {
			return d.Category, true
		}
	}
	return 0, false
}

// categoryCounts tallies the current mob population per category (for caps).
func (m *Manager) categoryCounts() map[mobCategory]int {
	counts := map[mobCategory]int{}
	for _, mob := range m.mobs.All() {
		if cat, ok := mobCategoryOf(mob.Type); ok {
			counts[cat]++
		}
	}
	return counts
}

// isNight reports whether dayTime is in the hostile-spawn window (dusk→dawn).
func isNight(dayTime int64) bool {
	t := dayTime % 24000
	return t >= 13000 && t < 23000
}

// hasHeadroom reports a 2-block air gap at (x,y,z) so a mob fits.
func hasHeadroom(w *World, x, y, z int) bool {
	return w.GetBlock(x, y, z).ID() == AirID && w.GetBlock(x, y+1, z).ID() == AirID
}

// pickSpawnColumn chooses a random column in the shell around a random player
// anchor. ok=false when there are no anchors (nobody online → no spawning).
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

// randomOfCategory returns a random mobDef of the given category.
func randomOfCategory(rng *rand.Rand, cat mobCategory) (mobDef, bool) {
	candidates := make([]mobDef, 0, len(spawnTable))
	for _, d := range spawnTable {
		if d.Category == cat {
			candidates = append(candidates, d)
		}
	}
	if len(candidates) == 0 {
		return mobDef{}, false
	}
	return candidates[rng.Intn(len(candidates))], true
}

// spawnTick is the director, called at 5 Hz from StartMobAI with a few candidate
// attempts per call. Peaceful difficulty suppresses hostiles. No anchors → no
// spawning (also avoids generating chunks with nobody online).
func (m *Manager) spawnTick(rng *rand.Rand) {
	anchors := m.playerAnchors()
	if len(anchors) == 0 {
		return
	}
	m.mu.RLock()
	difficulty := m.difficulty
	m.mu.RUnlock()

	w := m.GetDefaultWorld()
	counts := m.categoryCounts()
	night := isNight(w.GetDayTime())

	const attempts = 3
	for i := 0; i < attempts; i++ {
		x, z, ok := m.pickSpawnColumn(rng, anchors)
		if !ok {
			return
		}
		y := w.HighestSolidY(x, z) // feet Y on the surface
		if !hasHeadroom(w, x, y, z) {
			continue
		}
		surface := StateName(w.GetBlock(x, y-1, z).ID())

		switch {
		case night && difficulty != gameplay.DifficultyPeaceful && counts[catHostile] < capHostile:
			if def, ok := randomOfCategory(rng, catHostile); ok {
				m.mobs.Spawn(def.Type, float64(x)+0.5, float64(y), float64(z)+0.5)
				counts[catHostile]++
			}
		case !night && surface == "minecraft:grass_block" && counts[catPassive] < capPassive:
			if def, ok := randomOfCategory(rng, catPassive); ok {
				m.mobs.Spawn(def.Type, float64(x)+0.5, float64(y), float64(z)+0.5)
				counts[catPassive]++
			}
		}
	}
}
