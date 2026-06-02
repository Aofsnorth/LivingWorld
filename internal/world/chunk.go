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

// GetBlock/SetBlock address the chunk by CANONICAL world-Y in
// [MinWorldHeight, MinWorldHeight+SectionsPerChunk*16) = [-64, 320), i.e. top
// placeable Y is 319. Section index and local-Y are both derived from
// (y - MinWorldHeight) so sub-0 Y maps correctly: section 0 holds world-Y
// -64..-49, section 4 holds 0..15, etc. (The previous `y>>4` rejected y<0 and was
// off by 64 against the protocol converters — DESIGN §canonical-Y.)
func (c *Chunk) GetBlock(x, y, z int) Block {
	sectionIndex := (y - MinWorldHeight) >> 4
	if sectionIndex < 0 || sectionIndex >= SectionsPerChunk {
		return BlockAir{}
	}
	section := c.sections[sectionIndex]
	if section.IsEmpty() {
		return BlockAir{}
	}
	return section.GetBlock(x, (y-MinWorldHeight)&15, z)
}

func (c *Chunk) SetBlock(x, y, z int, block Block) {
	sectionIndex := (y - MinWorldHeight) >> 4
	if sectionIndex < 0 || sectionIndex >= SectionsPerChunk {
		return
	}
	c.sections[sectionIndex].SetBlock(x, (y-MinWorldHeight)&15, z, block)
	c.dirty.Store(true)
}

// GetSkyLight returns the sky light level (0-15) at the given chunk-local coordinates.
// y is world Y (canonical).
func (c *Chunk) GetSkyLight(x, y, z int) uint8 {
	sectionIndex := (y - MinWorldHeight) >> 4
	if sectionIndex < 0 || sectionIndex >= SectionsPerChunk {
		return 0
	}
	return c.sections[sectionIndex].GetSkyLight(x, (y-MinWorldHeight)&15, z)
}

// SetSkyLight sets the sky light level (0-15) at the given chunk-local coordinates.
// y is world Y (canonical).
func (c *Chunk) SetSkyLight(x, y, z int, val uint8) {
	sectionIndex := (y - MinWorldHeight) >> 4
	if sectionIndex < 0 || sectionIndex >= SectionsPerChunk {
		return
	}
	c.sections[sectionIndex].SetSkyLight(x, (y-MinWorldHeight)&15, z, val)
}

// GetBlockLight returns the block light level (0-15) at the given chunk-local coordinates.
// y is world Y (canonical).
func (c *Chunk) GetBlockLight(x, y, z int) uint8 {
	sectionIndex := (y - MinWorldHeight) >> 4
	if sectionIndex < 0 || sectionIndex >= SectionsPerChunk {
		return 0
	}
	return c.sections[sectionIndex].GetBlockLight(x, (y-MinWorldHeight)&15, z)
}

// SetBlockLight sets the block light level (0-15) at the given chunk-local coordinates.
// y is world Y (canonical).
func (c *Chunk) SetBlockLight(x, y, z int, val uint8) {
	sectionIndex := (y - MinWorldHeight) >> 4
	if sectionIndex < 0 || sectionIndex >= SectionsPerChunk {
		return
	}
	c.sections[sectionIndex].SetBlockLight(x, (y-MinWorldHeight)&15, z, val)
}

// Heightmap returns the chunk's heightmap array (256 entries, indexed by x*16+z).
// Each entry is the world Y of the highest non-air block in that column.
func (c *Chunk) Heightmap() []int32 {
	return c.heightMap
}

// SetHeightmap sets the heightmap value at column (x, z) to the given world Y.
func (c *Chunk) SetHeightmap(x, z int, y int32) {
	if x < 0 || x >= 16 || z < 0 || z >= 16 {
		return
	}
	c.heightMap[x*16+z] = y
}

// GetHeightmap returns the heightmap value at column (x, z).
func (c *Chunk) GetHeightmap(x, z int) int32 {
	if x < 0 || x >= 16 || z < 0 || z >= 16 {
		return 0
	}
	return c.heightMap[x*16+z]
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

// GetSkyLight returns the sky light level (0-15) at the given section-local coordinates.
func (s *ChunkSection) GetSkyLight(x, y, z int) uint8 {
	idx := (y << 8) | (z << 4) | x
	if s.skyLight == nil {
		return 0
	}
	if idx&1 == 0 {
		return s.skyLight[idx>>1] & 0x0F
	}
	return s.skyLight[idx>>1] >> 4
}

// SetSkyLight sets the sky light level (0-15) at the given section-local coordinates.
func (s *ChunkSection) SetSkyLight(x, y, z int, val uint8) {
	idx := (y << 8) | (z << 4) | x
	if s.skyLight == nil {
		s.skyLight = make([]byte, 2048)
	}
	byteIdx := idx >> 1
	if idx&1 == 0 {
		s.skyLight[byteIdx] = (s.skyLight[byteIdx] & 0xF0) | (val & 0x0F)
	} else {
		s.skyLight[byteIdx] = (s.skyLight[byteIdx] & 0x0F) | ((val & 0x0F) << 4)
	}
}

// GetBlockLight returns the block light level (0-15) at the given section-local coordinates.
func (s *ChunkSection) GetBlockLight(x, y, z int) uint8 {
	idx := (y << 8) | (z << 4) | x
	if s.blockLight == nil {
		return 0
	}
	if idx&1 == 0 {
		return s.blockLight[idx>>1] & 0x0F
	}
	return s.blockLight[idx>>1] >> 4
}

// SetBlockLight sets the block light level (0-15) at the given section-local coordinates.
func (s *ChunkSection) SetBlockLight(x, y, z int, val uint8) {
	idx := (y << 8) | (z << 4) | x
	if s.blockLight == nil {
		s.blockLight = make([]byte, 2048)
	}
	byteIdx := idx >> 1
	if idx&1 == 0 {
		s.blockLight[byteIdx] = (s.blockLight[byteIdx] & 0xF0) | (val & 0x0F)
	} else {
		s.blockLight[byteIdx] = (s.blockLight[byteIdx] & 0x0F) | ((val & 0x0F) << 4)
	}
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
