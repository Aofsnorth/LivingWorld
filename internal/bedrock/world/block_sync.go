package world

import (
	lwworld "livingworld/internal/world"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
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
	name := lwworld.StateName(id)
	if props, ok := bedrockPropOverrides[name]; ok {
		return BlockRID(name, props)
	}
	return BlockRID(name)
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
