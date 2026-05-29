package registry

import (
	mcregistry "github.com/Tnze/go-mc/registry"
)

func Build() (mcregistry.Registries, map[string]int) {
	r := mcregistry.NewNetworkCodec()
	sizes := map[string]int{}

	registerDimension(&r, sizes)
	registerBiomes(&r, sizes)
	registerDamageTypes(&r, sizes)
	registerChatType(&r, sizes)
	registerEntityVariants(&r, sizes)
	registerSoundVariants(&r, sizes)
	registerTrimData(&r, sizes)
	registerEnchantments(&r, sizes)
	registerJukeboxSongs(&r, sizes)
	registerInstruments(&r, sizes)

	return r, sizes
}
