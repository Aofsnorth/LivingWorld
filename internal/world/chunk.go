package world

const (
	ChunkSize        = 16
	SectionsPerChunk = 24
	MaxWorldHeight   = 384
	MinWorldHeight   = -64
)

type Chunk struct {
	sections  []ChunkSection
	heightMap []int32
	biomes    []byte
	lightData *LightData
}

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
	return GlobalBlockRegistry.Get(s.blocks[idx])
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
