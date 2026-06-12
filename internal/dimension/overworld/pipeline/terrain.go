package pipeline

import (
	"math"

	"livingworld/internal/dimension/overworld/biome"
)

// column is the per-column scratch the pipeline computes once and every
// later stage (surface, caves, ores, features) reads.
type column struct {
	height int     // Y of the highest solid terrain block
	biome  biome.Parameters
	temp   float64 // climate temperature  [-1, 1]
	humid  float64 // climate humidity     [-1, 1]
	cont   float64 // continentalness      [-1, 1]
	eros   float64 // erosion              [-1, 1]
	river  float64 // river strength       [0, 1]
	peaks  float64 // mountain-peak factor [0, 1]
}

// continentSpline maps continentalness to the base terrain height, the
// same role vanilla's "offset" spline plays: deep ocean floors on the
// far negative side, a steep coast around -0.1, and a gentle rise
// inland.
var continentSpline = spline{
	in:  []float64{-1.0, -0.50, -0.30, -0.16, -0.08, -0.02, 0.10, 0.35, 0.60, 1.00},
	out: []float64{36, 40, 49, 57, 62.2, 64.5, 68, 74, 80, 88},
}

// shapeColumn computes the terrain column at world (wx, wz). It is a
// pure function of (seed, wx, wz) — features anchored in neighbouring
// chunks re-evaluate it to find their ground height, so it must stay
// chunk-independent.
func (g *Generator) shapeColumn(wx, wz int) column {
	fx, fz := float64(wx), float64(wz)
	t := g.temperature.at2(fx, fz)
	h := g.humidity.at2(fx, fz)
	c := g.continental.at2(fx, fz)
	e := g.erosion.at2(fx, fz)
	w := g.weirdness.at2(fx, fz)
	d := g.detail.at2(fx, fz)

	base := continentSpline.at(c)

	// Relief amplitude from erosion: low erosion → dramatic terrain,
	// high erosion → flat plains. Linear from (e=-0.7 → 1.0) down to
	// (e=0.7 → 0.15).
	amp := clampF(1.0-(e+0.7)*0.607, 0.15, 1.0)

	// Peaks & valleys from weirdness, folded the way vanilla folds it.
	// pvPos in [0, 1]: 0 = valley floor, 1 = ridge top. Ridges only rise
	// (never dig below base) so inland terrain can't accidentally drop
	// under sea level — rivers do all the cutting.
	pv := 1 - math.Abs(3*math.Abs(w)-2)
	pvPos := (pv + 1) * 0.5
	inland := smooth((c + 0.06) / 0.18)
	ridge := math.Pow(pvPos, 1.5) * 38 * amp * inland

	// Jagged peaks: tall mountains where ridges are strong, erosion is
	// low, and we're solidly inland.
	peakF := 0.0
	if pvPos > 0.72 && e < -0.2 && c > 0.05 {
		peakF = ((pvPos - 0.72) / 0.28) *
			math.Min(1, (-0.2-e)/0.45) *
			math.Min(1, (c-0.05)/0.2)
		j := g.jagged.at2(fx, fz)
		ridge += peakF * (35 + 55*(j+1)*0.5)
	}

	height := base + ridge + d*4.5*amp*(0.35+0.65*inland)

	// River carve: weirdness near zero is the river band (pv ≈ -1 there,
	// i.e. valley floors — same correspondence vanilla uses).
	riv := 0.0
	if c > -0.08 {
		rw := math.Abs(w)
		const riverWidth = 0.05
		if rw < riverWidth {
			riv = smooth(1 - rw/riverWidth)
			bed := 59.0 + 2.5*(1-riv)
			if height > bed {
				height = height*(1-riv) + bed*riv
			}
		}
	}

	hi := clampI(int(math.Round(height)), 1, 280)

	col := column{
		height: hi,
		temp:   t, humid: h, cont: c, eros: e,
		river: riv, peaks: peakF,
	}
	col.biome = biome.ByID(g.pickBiome(col))
	return col
}

// pickBiome is a decision tree over the climate sample — a compact
// stand-in for vanilla's OverworldBiomeBuilder grid. Order matters:
// oceans, rivers, and shores claim columns before the temperature ×
// humidity matrix assigns the inland biomes.
func (g *Generator) pickBiome(col column) biome.ID {
	t, h, c, e := col.temp, col.humid, col.cont, col.eros
	y := col.height

	// Deep oceans.
	if c < -0.45 {
		switch {
		case t < -0.45:
			return "minecraft:deep_frozen_ocean"
		case t < -0.15:
			return "minecraft:deep_cold_ocean"
		case t < 0.3:
			return "minecraft:deep_ocean"
		default:
			return "minecraft:deep_lukewarm_ocean"
		}
	}
	// Shallow oceans.
	if y <= 59 && c < -0.05 {
		switch {
		case t < -0.45:
			return "minecraft:frozen_ocean"
		case t < -0.15:
			return "minecraft:cold_ocean"
		case t < 0.25:
			return "minecraft:ocean"
		case t < 0.55:
			return "minecraft:lukewarm_ocean"
		default:
			return "minecraft:warm_ocean"
		}
	}
	// Rivers.
	if col.river > 0.55 && y <= 62 {
		if t < -0.4 {
			return "minecraft:frozen_river"
		}
		return "minecraft:river"
	}
	// Shores: the coastal band the continent spline holds at 60..64.
	if y >= 59 && y <= 64 && c < 0.0 && c > -0.16 {
		switch {
		case e < -0.35:
			return "minecraft:stony_shore"
		case t < -0.35:
			return "minecraft:snowy_beach"
		default:
			return "minecraft:beach"
		}
	}
	// High mountains.
	if y > 145 {
		switch {
		case t < -0.2:
			if col.peaks > 0.55 {
				return "minecraft:jagged_peaks"
			}
			return "minecraft:frozen_peaks"
		case t > 0.35:
			return "minecraft:stony_peaks"
		default:
			if col.peaks > 0.55 {
				return "minecraft:jagged_peaks"
			}
			return "minecraft:stony_peaks"
		}
	}
	// Mountain slopes / highlands.
	if y > 112 {
		switch {
		case t < -0.25:
			return "minecraft:snowy_slopes"
		case t < 0.05:
			return "minecraft:grove"
		case h < -0.2:
			return "minecraft:windswept_hills"
		case h > 0.3 && t > 0.1:
			return "minecraft:cherry_grove"
		default:
			return "minecraft:meadow"
		}
	}
	// Swamps hug low wet warm ground.
	if y <= 65 && h > 0.35 && t > 0.1 && col.river < 0.4 && c > 0.0 {
		return "minecraft:swamp"
	}
	// Middle biomes: temperature bands × humidity.
	switch {
	case t < -0.45: // frozen
		switch {
		case h < -0.5:
			return "minecraft:ice_spikes"
		case h < 0.2:
			return "minecraft:snowy_plains"
		default:
			return "minecraft:snowy_taiga"
		}
	case t < -0.15: // cold
		switch {
		case h < -0.3:
			return "minecraft:snowy_plains"
		case h < 0.3:
			return "minecraft:taiga"
		default:
			return "minecraft:old_growth_pine_taiga"
		}
	case t < 0.2: // temperate
		switch {
		case h < -0.25:
			return "minecraft:plains"
		case h < 0.05:
			return "minecraft:forest"
		case h < 0.3:
			return "minecraft:birch_forest"
		default:
			return "minecraft:dark_forest"
		}
	case t < 0.55: // warm
		switch {
		case h < -0.35:
			return "minecraft:savanna"
		case h < -0.05:
			return "minecraft:plains"
		case h < 0.2:
			return "minecraft:forest"
		case h < 0.4:
			return "minecraft:sparse_jungle"
		default:
			return "minecraft:jungle"
		}
	default: // hot
		switch {
		case h < -0.3:
			if e < -0.35 && c > 0.3 {
				return "minecraft:badlands"
			}
			return "minecraft:desert"
		case h < 0.0:
			return "minecraft:savanna"
		case h < 0.25:
			return "minecraft:sparse_jungle"
		default:
			return "minecraft:jungle"
		}
	}
}
