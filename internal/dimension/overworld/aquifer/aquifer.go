// Package aquifer is LivingWorld's aquifer stage. Vanilla's aquifer
// (net.minecraft.world.level.levelgen.Aquifer) reads four density
// fields per cell to decide whether that cell should be water, lava, or
// air:
//
//   - Barrier              : is this cell inside a "barrier" of solid rock?
//   - FluidLevelFloodedness: should this cell flood if its surrounding
//     cells are removed?
//   - FluidLevelSpread     : should the fluid spread INTO this cell from
//     a flooded neighbour?
//   - Lava                 : if flooded, is the fluid lava?
//
// The implementation here models the same per-cell decision process with
// the same four density fields. The chunk pipeline calls Aquifer.Apply
// AFTER carvers (so the cells the carvers emptied are eligible to flood)
// and BEFORE surface rules (so the surface rule sees the final column).
package aquifer

import (
	"livingworld/internal/dimension/overworld/biome"
	"livingworld/internal/dimension/overworld/density"
	"math"
)

// Aquifer is the per-chunk aquifer instance. It carries the density
// fields the chunk pipeline queries per cell.
type Aquifer struct {
	SeaLevel int
	Fields   density.AquiferFields
}

// New builds an aquifer for the given density context.
func New(c density.Context, cr density.ClimateReader, seaLevel int) *Aquifer {
	return &Aquifer{
		SeaLevel: seaLevel,
		Fields:   density.BuildAquiferFields(c, cr),
	}
}

// Apply inspects each cell of the chunk. If the cell is currently air
// (carved out) and the aquifer decides it should be flooded, the cell
// becomes water (or lava if the lava field is set). Returns the list of
// per-cell decisions; the chunk pipeline materialises them.
func (a *Aquifer) Apply(c density.Context, x, y, z int, currentBlock string, b biome.Parameters) string {
	if currentBlock != "minecraft:air" && currentBlock != "minecraft:cave_air" {
		return currentBlock // solid — not a flooding candidate
	}
	barrier := a.Fields.Barrier.Eval(c, x, y, z)
	if barrier > 0 {
		return currentBlock
	}
	floodedness := a.Fields.FluidLevelFloodedness.Eval(c, x, y, z)
	if math.Abs(floodedness) > 0.5 {
		// Flooded cell.
		lava := a.Fields.Lava.Eval(c, x, y, z) > 0
		if lava {
			return "minecraft:lava"
		}
		return "minecraft:water"
	}
	// Below sea level and no flooding? Sea-floor cells get water.
	if y < a.SeaLevel {
		return "minecraft:water"
	}
	return currentBlock
}
