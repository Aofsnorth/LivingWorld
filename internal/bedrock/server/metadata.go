package server

import (
	"math"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

func bedrockMetadata(name string, sneaking bool) protocol.EntityMetadata {
	meta := protocol.NewEntityMetadata()
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagHasGravity)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagHasCollision)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagShowName)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagAlwaysShowName)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagBreathing)

	meta[protocol.EntityDataKeyName] = name
	// The dedicated AlwaysShowNameTag byte (not just the flag) is what makes the
	// nametag render at any distance/angle; without it Bedrock fades it like a mob
	// nametag (only when close and looked at). Matches dragonfly's reference.
	meta[protocol.EntityDataKeyAlwaysShowNameTag] = uint8(1)
	meta[protocol.EntityDataKeyScale] = float32(1)
	meta[protocol.EntityDataKeyWidth] = float32(0.6)

	if sneaking {
		meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagSneaking)
		meta[protocol.EntityDataKeyPoseIndex] = int32(5)  // Crouching pose index
		meta[protocol.EntityDataKeyHeight] = float32(1.5) // Reduced height
	} else {
		meta[protocol.EntityDataKeyPoseIndex] = int32(0)  // Standing pose index
		meta[protocol.EntityDataKeyHeight] = float32(1.8) // Normal height
	}

	// Full air so remote player entities never render drowning bubble particles.
	meta[protocol.EntityDataKeyAirSupply] = int16(300)
	meta[protocol.EntityDataKeyAirSupplyMax] = int16(300)
	return meta
}

func bedrockSurvivalAbilityData(runtimeID uint64) protocol.AbilityData {
	return protocol.AbilityData{
		EntityUniqueID:     int64(runtimeID),
		PlayerPermissions:  0,
		CommandPermissions: 0,
		Layers: []protocol.AbilityLayer{{
			Type:      protocol.AbilityLayerTypeBase,
			Abilities: protocol.AbilityCount - 1,
			Values: protocol.AbilityBuild |
				protocol.AbilityMine |
				protocol.AbilityDoorsAndSwitches |
				protocol.AbilityOpenContainers |
				protocol.AbilityAttackPlayers |
				protocol.AbilityAttackMobs,
			FlySpeed:         protocol.AbilityBaseFlySpeed,
			VerticalFlySpeed: protocol.AbilityBaseVerticalFlySpeed,
			WalkSpeed:        protocol.AbilityBaseWalkSpeed,
		}},
	}
}

// bedrockHealthAttribute builds a minecraft:health attribute for UpdateAttributes.
func bedrockHealthAttribute(hp float32) protocol.Attribute {
	return protocol.Attribute{
		AttributeValue: protocol.AttributeValue{Name: "minecraft:health", Value: hp, Max: 20, Min: 0},
		DefaultMin:     0,
		DefaultMax:     20,
		Default:        20,
	}
}

func bedrockMovementAttribute() protocol.Attribute {
	return protocol.Attribute{
		AttributeValue: protocol.AttributeValue{
			Name:  "minecraft:movement",
			Value: protocol.AbilityBaseWalkSpeed,
			Max:   math.MaxFloat32,
			Min:   0,
		},
		DefaultMin: 0,
		DefaultMax: math.MaxFloat32,
		Default:    protocol.AbilityBaseWalkSpeed,
	}
}
