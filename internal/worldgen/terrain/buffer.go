// Package terrain builds a chunk's block layout from noise + biomes into a
// height-indexed buffer of namespaced block-NAME strings.
//
// It imports only the worldgen noise + biome packages (never world/registry),
// so it stays foundation-free and unit-testable while internal/** is locked.
// The worldgen→world glue translates names to Java state ids (via world.StateID)
// when materializing a *world.Chunk; that step is deferred until the lock lifts.
package terrain

// Chunk dimensions and reference levels (canonical -64..319 column, DESIGN §1.4).
const (
	Size     = 16
	MinY     = -64
	MaxY     = 319
	Height   = MaxY - MinY + 1 // 384
	SeaLevel = 63
)

// Air is the empty cell. Carved cells use CaveAir so the glue can tell carved
// air from never-set air; both resolve to air when materialized.
const (
	Air     = ""
	CaveAir = "minecraft:cave_air"
)

// Buffer holds one chunk's blocks as namespaced names. A cell's zero value is
// Air. Coordinates are local x,z in [0,Size) and world y in [MinY,MaxY].
type Buffer struct{ blocks []string }

// NewBuffer returns an all-air buffer for one chunk.
func NewBuffer() *Buffer { return &Buffer{blocks: make([]string, Size*Height*Size)} }

func inBounds(x, y, z int) bool {
	return x >= 0 && x < Size && z >= 0 && z < Size && y >= MinY && y <= MaxY
}

func index(x, y, z int) int { return ((y-MinY)*Size+z)*Size + x }

// Set writes a block name at (x,y,z); out-of-range writes are ignored.
func (b *Buffer) Set(x, y, z int, name string) {
	if inBounds(x, y, z) {
		b.blocks[index(x, y, z)] = name
	}
}

// Get returns the block name at (x,y,z), or Air if out of range.
func (b *Buffer) Get(x, y, z int) string {
	if !inBounds(x, y, z) {
		return Air
	}
	return b.blocks[index(x, y, z)]
}

// Blocks exposes the raw name slice (row-major y,z,x) for the materialization
// glue. Normal callers should use Get/Set.
func (b *Buffer) Blocks() []string { return b.blocks }
