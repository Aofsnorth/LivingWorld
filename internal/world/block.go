package world

// Block is a placed block. Its ID is the canonical world block ID, which equals
// the vanilla Java global block-state ID (see registry.go). Meta is retained for
// interface compatibility with older call sites and is unused for state blocks.
type Block interface {
	ID() int32
	Meta() int16
}

// BlockAir is the empty block (global state 0).
type BlockAir struct{}

func (BlockAir) ID() int32   { return AirID }
func (BlockAir) Meta() int16 { return 0 }

// StateBlock is any non-air block, identified by its global block-state ID.
type StateBlock struct{ State int32 }

func (b StateBlock) ID() int32   { return b.State }
func (b StateBlock) Meta() int16 { return 0 }

// BlockByID returns the Block for a canonical world block ID.
func BlockByID(id int32) Block {
	if id == AirID {
		return BlockAir{}
	}
	return StateBlock{State: id}
}

// BlockByName returns the Block for a namespaced name (e.g. "minecraft:stone").
// Unknown names yield air.
func BlockByName(name string) Block {
	return BlockByID(StateID(name))
}
