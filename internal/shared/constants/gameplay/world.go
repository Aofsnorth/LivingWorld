// Package gameplay menyediakan konstanta terkait gameplay mechanics.
package gameplay

// Chunk constants
const (
	// ChunkSizeXZ adalah ukuran chunk dalam blocks (X dan Z axis)
	ChunkSizeXZ = 16

	// ChunkSectionBlocks adalah jumlah blocks dalam satu chunk section
	ChunkSectionBlocks = 4096 // 16 * 16 * 16

	// ChunkBitShift adalah bit shift untuk konversi world coord ke chunk coord
	ChunkBitShift = 4 // x >> 4 = x / 16

	// ChunkMask adalah mask untuk mendapatkan local coord dalam chunk
	ChunkMask = 15 // x & 15 = x % 16
)

// World generation constants
const (
	// SuperflatGroundY adalah Y coordinate untuk ground level di superflat world
	SuperflatGroundY = 3

	// SuperflatSpawnY adalah Y coordinate untuk player spawn di superflat world
	SuperflatSpawnY = 4

	// ClimateScale adalah scale factor untuk climate noise generation
	ClimateScale = 1.0 / 512.0

	// TerrainScale adalah scale factor untuk terrain noise generation
	TerrainScale = 1.0 / 128.0
)

// Dimension names
const (
	// DimensionOverworld adalah identifier untuk Overworld dimension
	DimensionOverworld = "overworld"

	// DimensionNether adalah identifier untuk Nether dimension
	DimensionNether = "nether"

	// DimensionEnd adalah identifier untuk End dimension
	DimensionEnd = "end"
)

// View distance constants
const (
	// DefaultJavaViewDistance adalah view distance default untuk Java clients
	DefaultJavaViewDistance = 10

	// DefaultBedrockViewDistance adalah view distance default untuk Bedrock clients
	DefaultBedrockViewDistance = 8

	// DefaultSimulationDistance adalah simulation distance default
	DefaultSimulationDistance = 10
)
