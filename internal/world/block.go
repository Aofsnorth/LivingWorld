package world

type Block interface {
	ID() int32
	Meta() int16
}

type BlockAir struct{}

func (BlockAir) ID() int32   { return 0 }
func (BlockAir) Meta() int16 { return 0 }

type PlaceholderBlock struct {
	IDValue int32
}

func (b PlaceholderBlock) ID() int32   { return b.IDValue }
func (b PlaceholderBlock) Meta() int16 { return 0 }

func BlockByID(id int32) Block {
	if blk := GlobalBlockRegistry.Get(id); blk.ID() != 0 || id == 0 {
		return blk
	}
	return PlaceholderBlock{IDValue: id}
}

type BlockRegistry struct {
	blocks map[int32]Block
}

func NewBlockRegistry() *BlockRegistry {
	return &BlockRegistry{
		blocks: make(map[int32]Block),
	}
}

func (r *BlockRegistry) Register(id int32, block Block) {
	r.blocks[id] = block
}

func (r *BlockRegistry) Get(id int32) Block {
	if b, ok := r.blocks[id]; ok {
		return b
	}
	return BlockAir{}
}

var GlobalBlockRegistry = NewBlockRegistry()

func init() {
	GlobalBlockRegistry.Register(0, BlockAir{})
	GlobalBlockRegistry.Register(1, PlaceholderBlock{IDValue: 1})
	GlobalBlockRegistry.Register(2, PlaceholderBlock{IDValue: 2})
	GlobalBlockRegistry.Register(3, PlaceholderBlock{IDValue: 3})
	GlobalBlockRegistry.Register(4, PlaceholderBlock{IDValue: 4})
}
