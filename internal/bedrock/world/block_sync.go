package world

import (
	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
)

func LivingWorldBlockIDToBedrockRID(id int32) uint32 {
	switch id {
	case 0:
		return BlockRID("minecraft:air")
	case 1:
		return BlockRID("minecraft:bedrock")
	case 2:
		return BlockRID("minecraft:dirt")
	case 3:
		return BlockRID("minecraft:grass_block", map[string]any{"minecraft:snowy_bit": false})
	case 4:
		return BlockRID("minecraft:stone")
	default:
		return BlockRID("minecraft:air")
	}
}

func BedrockRIDToLivingWorldBlockID(rid uint32) int32 {
	name, _, ok := dfchunk.RuntimeIDToState(rid)
	if !ok {
		return 0
	}
	switch name {
	case "minecraft:air":
		return 0
	case "minecraft:bedrock":
		return 1
	case "minecraft:dirt":
		return 2
	case "minecraft:grass_block":
		return 3
	case "minecraft:stone":
		return 4
	default:
		return 4
	}
}
