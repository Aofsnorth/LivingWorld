package registry

// Sentinels resolve unmapped translations instead of dropping packets
// (DESIGN §4, §12): a lookup that misses returns ok=false plus the sentinel so
// the edge substitutes it and logs, never silently drops.
const (
	AirState    BlockState = 0 // Java global state id 0 = minecraft:air
	UnknownItem            = "minecraft:air"
)

// Registry holds the Java<->Bedrock id maps for blocks, items and entities.
// The canonical key is always the Java-side id/name; Bedrock runtime ids are
// the translated edge values. Tables are populated from bundled go-mc/dragonfly
// data via `go generate` later; this is the lookup surface lanes build on.
type Registry struct {
	blockToBedrock map[BlockState]uint32
	blockToJava    map[uint32]BlockState
	itemRuntime    map[string]int32
	itemName       map[int32]string
	entityNetID    map[string]int32
}

// New returns an empty registry ready for population.
func New() *Registry {
	return &Registry{
		blockToBedrock: map[BlockState]uint32{},
		blockToJava:    map[uint32]BlockState{},
		itemRuntime:    map[string]int32{},
		itemName:       map[int32]string{},
		entityNetID:    map[string]int32{},
	}
}

// RegisterBlock maps a canonical Java block state to a Bedrock runtime id.
func (r *Registry) RegisterBlock(java BlockState, bedrock uint32) {
	r.blockToBedrock[java] = bedrock
	r.blockToJava[bedrock] = java
}

// BlockToBedrock returns the Bedrock runtime id for a canonical state.
func (r *Registry) BlockToBedrock(java BlockState) (uint32, bool) {
	id, ok := r.blockToBedrock[java]
	return id, ok
}

// BlockToJava returns the canonical state for a Bedrock runtime id, falling
// back to the air sentinel (ok=false) when unmapped.
func (r *Registry) BlockToJava(bedrock uint32) (BlockState, bool) {
	if s, ok := r.blockToJava[bedrock]; ok {
		return s, true
	}
	return AirState, false
}

// RegisterItem maps a namespaced item id to an edition runtime id.
func (r *Registry) RegisterItem(name string, runtime int32) {
	r.itemRuntime[name] = runtime
	r.itemName[runtime] = name
}

// ItemRuntime returns the runtime id for a namespaced item name.
func (r *Registry) ItemRuntime(name string) (int32, bool) {
	id, ok := r.itemRuntime[name]
	return id, ok
}

// ItemName returns the namespaced name for a runtime id, or the unknown
// sentinel when unmapped.
func (r *Registry) ItemName(runtime int32) string {
	if n, ok := r.itemName[runtime]; ok {
		return n
	}
	return UnknownItem
}

// RegisterEntity maps a canonical entity type to its network id.
func (r *Registry) RegisterEntity(typ string, netID int32) { r.entityNetID[typ] = netID }

// EntityNetID returns the network id for a canonical entity type.
func (r *Registry) EntityNetID(typ string) (int32, bool) {
	id, ok := r.entityNetID[typ]
	return id, ok
}
