package server

import (
	"math"

	"livingworld/internal/player"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

func bedrockMetadata(name string, sneaking bool) protocol.EntityMetadata {
	return bedrockPlayerMetadata(player.PlayerSnapshot{
		Username:  name,
		Sneaking:  sneaking,
		Breathing: true,
		AirSupply: 300,
	})
}

func bedrockPlayerMetadata(p player.PlayerSnapshot) protocol.EntityMetadata {
	meta := protocol.NewEntityMetadata()
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagHasGravity)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagHasCollision)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagShowName)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagAlwaysShowName)

	breathing := p.Breathing || (p.AirSupply == 0 && p.UUID == [16]byte{})
	if breathing {
		meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagBreathing)
	}
	if p.OnFire {
		meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagOnFire)
	}
	if p.Sprinting {
		meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagSprinting)
	}
	if p.Swimming {
		meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagSwimming)
	}
	if p.Sneaking {
		meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagSneaking)
	}

	meta[protocol.EntityDataKeyName] = p.Username
	meta[protocol.EntityDataKeyAlwaysShowNameTag] = uint8(1)
	meta[protocol.EntityDataKeyScale] = float32(1)
	meta[protocol.EntityDataKeyWidth] = float32(0.6)

	switch {
	case p.Swimming:
		meta[protocol.EntityDataKeyHeight] = float32(0.6)
	case p.Sneaking:
		meta[protocol.EntityDataKeyHeight] = float32(1.5)
	default:
		meta[protocol.EntityDataKeyHeight] = float32(1.8)
	}

	air := p.AirSupply
	if air == 0 && breathing {
		air = 300
	}
	if air < 0 {
		air = 0
	}
	meta[protocol.EntityDataKeyAirSupply] = air
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
