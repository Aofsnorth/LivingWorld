// Package biome defines the climate-classified biomes the worldgen surface
// stage uses to choose terrain height and the blocks laid on top.
//
// Surface/Filler are namespaced block names (e.g. "minecraft:grass_block"),
// not state ids, so this package stays independent of the world/registry
// packages; the chunk stage resolves names to state ids when it builds blocks.
package biome

import "math"

// Biome is a terrain region selected by climate (DESIGN §10: Biomes stage).
type Biome struct {
	Name        string  // namespaced biome id, e.g. "minecraft:plains"
	Temperature float64 // climate axis, 0 (cold) .. 1 (hot)
	Humidity    float64 // climate axis, 0 (dry) .. 1 (wet)
	BaseHeight  float64 // surface Y baseline
	Variation   float64 // vertical roughness applied to terrain noise
	Surface     string  // top block name
	Filler      string  // block name directly below the surface
}

// Core biomes. Kept small and deterministic; extended as the pipeline grows.
var (
	Ocean     = Biome{"minecraft:ocean", 0.5, 0.5, 45, 3, "minecraft:gravel", "minecraft:dirt"}
	Plains    = Biome{"minecraft:plains", 0.8, 0.4, 68, 4, "minecraft:grass_block", "minecraft:dirt"}
	Forest    = Biome{"minecraft:forest", 0.7, 0.8, 70, 6, "minecraft:grass_block", "minecraft:dirt"}
	Desert    = Biome{"minecraft:desert", 1.0, 0.0, 68, 3, "minecraft:sand", "minecraft:sandstone"}
	Mountains = Biome{"minecraft:windswept_hills", 0.25, 0.3, 96, 24, "minecraft:stone", "minecraft:stone"}
)

var registry = []Biome{Ocean, Plains, Forest, Desert, Mountains}

// All returns the registered biomes.
func All() []Biome { return registry }

// Select returns the biome whose climate is nearest to (temperature, humidity)
// by squared Euclidean distance. Deterministic: ties resolve to the earlier
// entry in the registry.
func Select(temperature, humidity float64) Biome {
	best := registry[0]
	bestDist := math.Inf(1)
	for _, b := range registry {
		dt := b.Temperature - temperature
		dh := b.Humidity - humidity
		if d := dt*dt + dh*dh; d < bestDist {
			bestDist, best = d, b
		}
	}
	return best
}
