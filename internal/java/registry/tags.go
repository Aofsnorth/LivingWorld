package registry

// Block-tag data the configuration phase must send in UpdateTags.
//
// The 26.3 client resolves a dimension type's infiniburn field — a
// HolderSet<Block> tag reference like "#minecraft:infiniburn_overworld" —
// against the block tags delivered in ClientboundUpdateTags. UpdateTags
// replaces the client's tag view wholesale: when the minecraft:block section
// was omitted, EVERY join attempt failed with
//
//	Missing tag: 'minecraft:infiniburn_overworld' in 'minecraft:block'
//
// wrapped in "Failed to parse value {...} for key minecraft:overworld" during
// registry sync. This is the only block tag our registry data references.
//
// Members come from 26.3-server.jar/data/minecraft/tags/block/infiniburn_overworld.json
// (exactly two blocks), and the ids are minecraft:block registry ids from the
// 26.3 registries dump (reports/registries.json protocol_id): netherrack=334,
// magma_block=729. Tag entries are written as registry ids on the wire
// (TagNetworkSerialization.NetworkPayload).

// BlockTagNameInfiniburnOverworld is the tag referenced by the overworld
// dimension type's infiniburn field.
const BlockTagNameInfiniburnOverworld = "minecraft:infiniburn_overworld"

// BlockTagInfiniburnOverworld lists the member registry ids of
// #minecraft:infiniburn_overworld: netherrack (334), magma_block (729).
func BlockTagInfiniburnOverworld() []int32 {
	return []int32{334, 729}
}
