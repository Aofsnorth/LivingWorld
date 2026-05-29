package registry

import (
	"github.com/Tnze/go-mc/nbt"
	mcregistry "github.com/Tnze/go-mc/registry"
)

type animalSoundSet struct {
	AmbientSound string `nbt:"ambient_sound"`
	HurtSound    string `nbt:"hurt_sound"`
	DeathSound   string `nbt:"death_sound"`
	StepSound    string `nbt:"step_sound"`
}

type chickenSoundVariant struct {
	AdultSounds animalSoundSet `nbt:"adult_sounds"`
	BabySounds  animalSoundSet `nbt:"baby_sounds"`
}

type pigSoundSet struct {
	AmbientSound string `nbt:"ambient_sound"`
	HurtSound    string `nbt:"hurt_sound"`
	DeathSound   string `nbt:"death_sound"`
	StepSound    string `nbt:"step_sound"`
	EatSound     string `nbt:"eat_sound"`
}

type pigSoundVariant struct {
	AdultSounds pigSoundSet `nbt:"adult_sounds"`
	BabySounds  pigSoundSet `nbt:"baby_sounds"`
}

type cowSoundVariant struct {
	AmbientSound string `nbt:"ambient_sound"`
	HurtSound    string `nbt:"hurt_sound"`
	DeathSound   string `nbt:"death_sound"`
	StepSound    string `nbt:"step_sound"`
}

type wolfSoundSet struct {
	AmbientSound string `nbt:"ambient_sound"`
	HurtSound    string `nbt:"hurt_sound"`
	DeathSound   string `nbt:"death_sound"`
	StepSound    string `nbt:"step_sound"`
	GrowlSound   string `nbt:"growl_sound"`
	PantSound    string `nbt:"pant_sound"`
	WhineSound   string `nbt:"whine_sound"`
}

type wolfSoundVariant struct {
	AdultSounds wolfSoundSet `nbt:"adult_sounds"`
	BabySounds  wolfSoundSet `nbt:"baby_sounds"`
}

type catSoundSet struct {
	AmbientSound      string `nbt:"ambient_sound"`
	StrayAmbientSound string `nbt:"stray_ambient_sound"`
	HissSound         string `nbt:"hiss_sound"`
	HurtSound         string `nbt:"hurt_sound"`
	DeathSound        string `nbt:"death_sound"`
	EatSound          string `nbt:"eat_sound"`
	BegForFoodSound   string `nbt:"beg_for_food_sound"`
	PurrSound         string `nbt:"purr_sound"`
	PurreowSound      string `nbt:"purreow_sound"`
}

type catSoundVariant struct {
	AdultSounds catSoundSet `nbt:"adult_sounds"`
	BabySounds  catSoundSet `nbt:"baby_sounds"`
}

func registerSoundVariants(r *mcregistry.Registries, sizes map[string]int) {
	r.Registry("minecraft:wolf_sound_variant").(*mcregistry.Registry[nbt.RawMessage]).Put("minecraft:default", marshalNBT(wolfSoundVariant{
		AdultSounds: wolfSoundSet{
			AmbientSound: "minecraft:entity.wolf.ambient", HurtSound: "minecraft:entity.wolf.hurt",
			DeathSound: "minecraft:entity.wolf.death", StepSound: "minecraft:entity.wolf.step",
			GrowlSound: "minecraft:entity.wolf.growl", PantSound: "minecraft:entity.wolf.pant",
			WhineSound: "minecraft:entity.wolf.whine",
		},
		BabySounds: wolfSoundSet{
			AmbientSound: "minecraft:entity.wolf.ambient", HurtSound: "minecraft:entity.wolf.hurt",
			DeathSound: "minecraft:entity.wolf.death", StepSound: "minecraft:entity.wolf.step",
			GrowlSound: "minecraft:entity.wolf.growl", PantSound: "minecraft:entity.wolf.pant",
			WhineSound: "minecraft:entity.wolf.whine",
		},
	}))
	sizes["minecraft:wolf_sound_variant"] = 1

	r.Registry("minecraft:cow_sound_variant").(*mcregistry.Registry[nbt.RawMessage]).Put("minecraft:default", marshalNBT(cowSoundVariant{
		AmbientSound: "minecraft:entity.cow.ambient", HurtSound: "minecraft:entity.cow.hurt",
		DeathSound: "minecraft:entity.cow.death", StepSound: "minecraft:entity.cow.step",
	}))
	sizes["minecraft:cow_sound_variant"] = 1

	r.Registry("minecraft:chicken_sound_variant").(*mcregistry.Registry[nbt.RawMessage]).Put("minecraft:default", marshalNBT(chickenSoundVariant{
		AdultSounds: animalSoundSet{
			AmbientSound: "minecraft:entity.chicken.ambient", HurtSound: "minecraft:entity.chicken.hurt",
			DeathSound: "minecraft:entity.chicken.death", StepSound: "minecraft:entity.chicken.step",
		},
		BabySounds: animalSoundSet{
			AmbientSound: "minecraft:entity.chicken.ambient", HurtSound: "minecraft:entity.chicken.hurt",
			DeathSound: "minecraft:entity.chicken.death", StepSound: "minecraft:entity.chicken.step",
		},
	}))
	sizes["minecraft:chicken_sound_variant"] = 1

	r.Registry("minecraft:pig_sound_variant").(*mcregistry.Registry[nbt.RawMessage]).Put("minecraft:default", marshalNBT(pigSoundVariant{
		AdultSounds: pigSoundSet{
			AmbientSound: "minecraft:entity.pig.ambient", HurtSound: "minecraft:entity.pig.hurt",
			DeathSound: "minecraft:entity.pig.death", StepSound: "minecraft:entity.pig.step",
			EatSound: "minecraft:entity.pig.eat",
		},
		BabySounds: pigSoundSet{
			AmbientSound: "minecraft:entity.pig.ambient", HurtSound: "minecraft:entity.pig.hurt",
			DeathSound: "minecraft:entity.pig.death", StepSound: "minecraft:entity.pig.step",
			EatSound: "minecraft:entity.pig.eat",
		},
	}))
	sizes["minecraft:pig_sound_variant"] = 1

	r.Registry("minecraft:cat_sound_variant").(*mcregistry.Registry[nbt.RawMessage]).Put("minecraft:default", marshalNBT(catSoundVariant{
		AdultSounds: catSoundSet{
			AmbientSound: "minecraft:entity.cat.ambient", StrayAmbientSound: "minecraft:entity.cat.stray_ambient",
			HissSound: "minecraft:entity.cat.hiss", HurtSound: "minecraft:entity.cat.hurt",
			DeathSound: "minecraft:entity.cat.death", EatSound: "minecraft:entity.cat.eat",
			BegForFoodSound: "minecraft:entity.cat.beg_for_food", PurrSound: "minecraft:entity.cat.purr",
			PurreowSound: "minecraft:entity.cat.purreow",
		},
		BabySounds: catSoundSet{
			AmbientSound: "minecraft:entity.cat.ambient", StrayAmbientSound: "minecraft:entity.cat.stray_ambient",
			HissSound: "minecraft:entity.cat.hiss", HurtSound: "minecraft:entity.cat.hurt",
			DeathSound: "minecraft:entity.cat.death", EatSound: "minecraft:entity.cat.eat",
			BegForFoodSound: "minecraft:entity.cat.beg_for_food", PurrSound: "minecraft:entity.cat.purr",
			PurreowSound: "minecraft:entity.cat.purreow",
		},
	}))
	sizes["minecraft:cat_sound_variant"] = 1
}
