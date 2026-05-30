package world

import (
	"sync"

	lwworld "livingworld/internal/world"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
)

// ridCache memoizes world state ID -> Bedrock runtime ID. Chunk serialization
// resolves the RID for every non-air block, so caching avoids repeated palette
// hash lookups.
var (
	ridCacheMu sync.RWMutex
	ridCache   = map[int32]uint32{}
)

// bedrockPropOverrides supplies the Bedrock block-state properties required for
// blocks that will not resolve from their name alone (Bedrock hashes name+props).
// Most blocks resolve by name; only the property-sensitive ones need an entry.
// Java->Bedrock property fidelity for stateful blocks (stairs, slabs, logs...) is
// a known limitation and can be expanded here as needed.
var bedrockPropOverrides = map[string]map[string]any{
	"minecraft:grass_block": {"minecraft:snowy_bit": false},
}

// LivingWorldBlockIDToBedrockRID maps a canonical world block ID (= Java global
// state ID) to a Bedrock runtime ID via the block's namespaced name.
func LivingWorldBlockIDToBedrockRID(id int32) uint32 {
	ridCacheMu.RLock()
	rid, ok := ridCache[id]
	ridCacheMu.RUnlock()
	if ok {
		return rid
	}

	name := lwworld.StateName(id)
	if props, ok := bedrockPropOverrides[name]; ok {
		rid = BlockRID(name, props)
	} else {
		rid = BlockRID(name)
	}

	ridCacheMu.Lock()
	ridCache[id] = rid
	ridCacheMu.Unlock()
	return rid
}

// BedrockRIDToLivingWorldBlockID maps a Bedrock runtime ID back to a canonical
// world block ID by resolving its namespaced name in the global palette.
func BedrockRIDToLivingWorldBlockID(rid uint32) int32 {
	name, _, ok := dfchunk.RuntimeIDToState(rid)
	if !ok {
		return lwworld.AirID
	}
	return lwworld.StateID(name)
}
