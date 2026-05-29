package registry

import (
	mcregistry "github.com/Tnze/go-mc/registry"
)

func registerDamageTypes(r *mcregistry.Registries, sizes map[string]int) {
	damageTypes := []string{
		"arrow", "dragon_breath", "ender_pearl", "fireball", "fireworks", "mace_smash", "mob_attack",
		"mob_attack_no_aggro", "mob_projectile", "player_attack", "player_explosion", "sonic_boom",
		"spear", "spit", "thrown", "trident", "unattributed_fireball", "wither_skull", "wind_charge",
		"cactus", "campfire", "cramming", "drown", "dry_out", "explosion", "fall", "falling_anvil",
		"falling_block", "falling_stalactite", "fly_into_wall", "freeze", "hot_floor", "in_fire",
		"in_wall", "lava", "lightning_bolt", "on_fire", "stalagmite", "sweet_berry_bush",
		"bad_respawn_point", "generic", "generic_kill", "indirect_magic", "magic", "out_of_world",
		"outside_border", "starve", "sting", "thorns", "wither",
	}
	for _, dt := range damageTypes {
		r.DamageType.Put("minecraft:"+dt, marshalNBT(mcregistry.DamageType{
			MessageID:  dt,
			Scaling:    "when_caused_by_living_non_player",
			Exhaustion: 0.1,
		}))
	}
	sizes["minecraft:damage_type"] = len(damageTypes)
}

func registerChatType(r *mcregistry.Registries, sizes map[string]int) {
	chatType := mcregistry.ChatType{}
	chatType.Chat.TranslationKey = "chat.type.text"
	chatType.Chat.Parameters = []string{"sender", "content"}
	chatType.Narration.TranslationKey = "chat.type.text.narrate"
	chatType.Narration.Parameters = []string{"sender", "content"}
	r.ChatType.Put("minecraft:chat", chatType)
	sizes["minecraft:chat_type"] = 1
}
