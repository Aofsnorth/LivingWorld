package registry

import (
	"github.com/Tnze/go-mc/nbt"
	mcregistry "github.com/Tnze/go-mc/registry"
)

type wolfVariant struct {
	BabyAssets map[string]string `nbt:"baby_assets"`
	Assets     map[string]string `nbt:"assets"`
}

type zombieNautilusVariant struct {
	AssetID string `nbt:"asset_id"`
}

type paintingVariant struct {
	Width   int32  `nbt:"width"`
	Height  int32  `nbt:"height"`
	AssetID string `nbt:"asset_id"`
}

type catVariant struct {
	AssetID     string `nbt:"asset_id"`
	BabyAssetID string `nbt:"baby_asset_id"`
}

type frogVariant struct {
	AssetID     string `nbt:"asset_id"`
	BabyAssetID string `nbt:"baby_asset_id"`
}

type animalVariant struct {
	Model       string `nbt:"model"`
	AssetID     string `nbt:"asset_id"`
	BabyAssetID string `nbt:"baby_asset_id"`
}

func registerEntityVariants(r *mcregistry.Registries, sizes map[string]int) {
	registerWolfVariants(r, sizes)
	registerPaintingVariants(r, sizes)
	registerZombieNautilusVariants(r, sizes)
	registerCatVariants(r, sizes)
	registerFrogVariants(r, sizes)
	registerCowVariants(r, sizes)
	registerChickenVariants(r, sizes)
	registerPigVariants(r, sizes)
}

func registerWolfVariants(r *mcregistry.Registries, sizes map[string]int) {
	wolfData := wolfVariant{
		BabyAssets: map[string]string{
			"wild":  "minecraft:entity/wolf/wolf",
			"tame":  "minecraft:entity/wolf/wolf_tame",
			"angry": "minecraft:entity/wolf/wolf_angry",
		},
		Assets: map[string]string{
			"wild":  "minecraft:entity/wolf/wolf",
			"tame":  "minecraft:entity/wolf/wolf_tame",
			"angry": "minecraft:entity/wolf/wolf_angry",
		},
	}
	wolfVariants := []string{
		"minecraft:pale", "minecraft:spotted", "minecraft:black",
		"minecraft:chestnut", "minecraft:rusty", "minecraft:striped",
		"minecraft:snowy", "minecraft:woods", "minecraft:ashen",
	}
	for _, wv := range wolfVariants {
		r.Registry("minecraft:wolf_variant").(*mcregistry.Registry[nbt.RawMessage]).Put(wv, marshalNBT(wolfData))
	}
	sizes["minecraft:wolf_variant"] = len(wolfVariants)
}

func registerPaintingVariants(r *mcregistry.Registries, sizes map[string]int) {
	paintingData := paintingVariant{
		Width:   1,
		Height:  1,
		AssetID: "minecraft:kebab",
	}
	r.Registry("minecraft:painting_variant").(*mcregistry.Registry[nbt.RawMessage]).Put("minecraft:kebab", marshalNBT(paintingData))
	sizes["minecraft:painting_variant"] = 1
}

func registerZombieNautilusVariants(r *mcregistry.Registries, sizes map[string]int) {
	variants := []struct {
		key   string
		asset string
	}{
		{"minecraft:temperate", "minecraft:default"},
		{"minecraft:warm", "minecraft:warm"},
	}
	for _, v := range variants {
		r.Registry("minecraft:zombie_nautilus_variant").(*mcregistry.Registry[nbt.RawMessage]).Put(v.key, marshalNBT(zombieNautilusVariant{
			AssetID: v.asset,
		}))
	}
	sizes["minecraft:zombie_nautilus_variant"] = len(variants)
}

func registerCatVariants(r *mcregistry.Registries, sizes map[string]int) {
	variants := []string{
		"minecraft:tabby", "minecraft:black", "minecraft:red", "minecraft:siamese",
		"minecraft:british_shorthair", "minecraft:calico", "minecraft:persian",
		"minecraft:ragdoll", "minecraft:white", "minecraft:jellie", "minecraft:tuxedo",
	}
	for _, cv := range variants {
		r.Registry("minecraft:cat_variant").(*mcregistry.Registry[nbt.RawMessage]).Put(cv, marshalNBT(catVariant{
			AssetID:     "minecraft:cat/" + cv[10:],
			BabyAssetID: "minecraft:cat/" + cv[10:],
		}))
	}
	sizes["minecraft:cat_variant"] = len(variants)
}

func registerFrogVariants(r *mcregistry.Registries, sizes map[string]int) {
	variants := []string{
		"minecraft:temperate", "minecraft:warm", "minecraft:cold",
	}
	for _, fv := range variants {
		r.Registry("minecraft:frog_variant").(*mcregistry.Registry[nbt.RawMessage]).Put(fv, marshalNBT(frogVariant{
			AssetID:     "minecraft:frog/" + fv[10:],
			BabyAssetID: "minecraft:frog/" + fv[10:],
		}))
	}
	sizes["minecraft:frog_variant"] = len(variants)
}

func registerCowVariants(r *mcregistry.Registries, sizes map[string]int) {
	// asset_id/baby_asset_id MUST match the vanilla texture paths exactly (verified
	// from 26.1 datagen cow_variant/*.json). The old short form "minecraft:cow/default"
	// resolved to textures/cow/default.png — which does not exist — so the client fell
	// back to the black/magenta missing texture for every cow. The correct path is
	// "minecraft:entity/cow/cow_temperate" → textures/entity/cow/cow_temperate.png.
	// The client defaults a freshly-spawned cow to the "minecraft:temperate" variant
	// (looked up by key), so fixing the registry alone renders cows correctly without
	// sending per-entity variant metadata.
	variants := []struct {
		key, model, asset string
	}{
		{"minecraft:temperate", "normal", "minecraft:entity/cow/cow_temperate"},
		{"minecraft:warm", "warm", "minecraft:entity/cow/cow_warm"},
		{"minecraft:cold", "cold", "minecraft:entity/cow/cow_cold"},
	}
	for _, v := range variants {
		r.Registry("minecraft:cow_variant").(*mcregistry.Registry[nbt.RawMessage]).Put(v.key, marshalNBT(animalVariant{
			Model:       v.model,
			AssetID:     v.asset,
			BabyAssetID: v.asset + "_baby",
		}))
	}
	sizes["minecraft:cow_variant"] = len(variants)
}

func registerChickenVariants(r *mcregistry.Registries, sizes map[string]int) {
	// Vanilla texture paths (26.1 datagen chicken_variant/*.json). See the cow note
	// above: only "cold" carries a non-normal model; warm reuses the normal model.
	variants := []struct {
		key, model, asset string
	}{
		{"minecraft:temperate", "normal", "minecraft:entity/chicken/chicken_temperate"},
		{"minecraft:warm", "normal", "minecraft:entity/chicken/chicken_warm"},
		{"minecraft:cold", "cold", "minecraft:entity/chicken/chicken_cold"},
	}
	for _, v := range variants {
		r.Registry("minecraft:chicken_variant").(*mcregistry.Registry[nbt.RawMessage]).Put(v.key, marshalNBT(animalVariant{
			Model:       v.model,
			AssetID:     v.asset,
			BabyAssetID: v.asset + "_baby",
		}))
	}
	sizes["minecraft:chicken_variant"] = len(variants)
}

func registerPigVariants(r *mcregistry.Registries, sizes map[string]int) {
	// Vanilla texture paths (26.1 datagen pig_variant/*.json). See the cow note above;
	// pig's warm variant reuses the normal model, only cold has its own model.
	variants := []struct {
		key, model, asset string
	}{
		{"minecraft:temperate", "normal", "minecraft:entity/pig/pig_temperate"},
		{"minecraft:warm", "normal", "minecraft:entity/pig/pig_warm"},
		{"minecraft:cold", "cold", "minecraft:entity/pig/pig_cold"},
	}
	for _, v := range variants {
		r.Registry("minecraft:pig_variant").(*mcregistry.Registry[nbt.RawMessage]).Put(v.key, marshalNBT(animalVariant{
			Model:       v.model,
			AssetID:     v.asset,
			BabyAssetID: v.asset + "_baby",
		}))
	}
	sizes["minecraft:pig_variant"] = len(variants)
}
