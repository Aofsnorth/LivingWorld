package world

import "sync/atomic"

const (
	ChunkSize        = 16
	SectionsPerChunk = 24
	MaxWorldHeight   = 384
	MinWorldHeight   = -64
)

// ChunkCoord converts a world block/position coordinate to its chunk coordinate
// using true floor division (Minecraft semantics), NOT Go's truncate-toward-zero
// int conversion. `int32(x) >> 4` is WRONG for negative coordinates: a player at
// x=-0.5 truncates to int32 0 → chunk 0, but the real chunk is -1. Since spawn is
// at (0,0), walking into -X/-Z mis-computes the player's chunk, so the view-radius
// is centred on the wrong chunk and far chunks never stream in until the player
// physically walks into a chunk the broken math finally agrees on. Floor division
// fixes both the boundary-cross trigger and the radius centre.
//
// We floor the block coordinate first, then arithmetic-shift by 4. Arithmetic
// right shift already floors toward -∞ for ints, so the only correction needed is
// turning the float→int truncation into a floor.
func ChunkCoord(worldCoord float64) int32 {
	block := int32(worldCoord) // truncates toward zero
	if worldCoord < 0 && float64(block) != worldCoord {
		block-- // floor: a fractional negative block belongs to the lower block
	}
	return block >> 4 // arithmetic shift floors toward -∞ (e.g. -1>>4 = -1)
}

type Chunk struct {
	sections  []ChunkSection
	heightMap []int32
	biomes    []byte
	lightData *LightData
	dirty     atomic.Bool
}

// Dirty reports whether the chunk has unsaved block changes.
func (c *Chunk) Dirty() bool { return c.dirty.Load() }

// MarkDirty flags the chunk as having unsaved changes.
func (c *Chunk) MarkDirty() { c.dirty.Store(true) }

// ClearDirty resets the dirty flag (called after a successful save).
func (c *Chunk) ClearDirty() { c.dirty.Store(false) }

func NewChunk() *Chunk {
	sections := make([]ChunkSection, SectionsPerChunk)
	for i := range sections {
		sections[i] = ChunkSection{
			blocks:     make([]int32, 4096),
			metadata:   make([]byte, 2048),
			blockLight: make([]byte, 2048),
			skyLight:   make([]byte, 2048),
		}
	}
	return &Chunk{
		sections:  sections,
		heightMap: make([]int32, 256),
		biomes:    make([]byte, 256),
		lightData: NewLightData(),
	}
}

func (c *Chunk) GetBlock(x, y, z int) Block {
	chunkY := y >> 4
	if chunkY < 0 || chunkY >= SectionsPerChunk {
		return BlockAir{}
	}
	section := c.sections[chunkY]
	if section.IsEmpty() {
		return BlockAir{}
	}
	return section.GetBlock(x, y&15, z)
}

func (c *Chunk) SetBlock(x, y, z int, block Block) {
	chunkY := y >> 4
	if chunkY < 0 || chunkY >= SectionsPerChunk {
		return
	}
	c.sections[chunkY].SetBlock(x, y&15, z, block)
	c.dirty.Store(true)
}

type ChunkSection struct {
	blocks       []int32
	metadata     []byte
	blockLight   []byte
	skyLight     []byte
	nonAirBlocks int32
	solidBlocks  int32
}

func NewChunkSection() *ChunkSection {
	return &ChunkSection{
		blocks:     make([]int32, 4096),
		metadata:   make([]byte, 2048),
		blockLight: make([]byte, 2048),
		skyLight:   make([]byte, 2048),
	}
}

func (s *ChunkSection) IsEmpty() bool { return s.nonAirBlocks == 0 }

func (s *ChunkSection) GetBlock(x, y, z int) Block {
	idx := (y << 8) | (z << 4) | x
	return BlockByID(s.blocks[idx])
}

func (s *ChunkSection) SetBlock(x, y, z int, block Block) {
	idx := (y << 8) | (z << 4) | x
	oldID := s.blocks[idx]
	newID := block.ID()
	if oldID == 0 && newID != 0 {
		s.nonAirBlocks++
	} else if oldID != 0 && newID == 0 {
		s.nonAirBlocks--
	}
	s.blocks[idx] = newID
}

type LightData struct {
	skyLight   []byte
	blockLight []byte
}

func NewLightData() *LightData {
	return &LightData{
		skyLight:   make([]byte, 2048),
		blockLight: make([]byte, 2048),
	}
}
