package server

import (
	"bytes"

	"github.com/Tnze/go-mc/data/packetid"
	gmnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
)

// vanillaSyncRegistry is one synchronized registry: its protocol name and its
// element ids in the vanilla server registry's raw-id order (see
// vanillasync.go for the provenance and why the order matters).
type vanillaSyncRegistry struct {
	Name     string
	Elements []string
}

// vanillaTag is one network tag with its members as raw registry ids.
type vanillaTag struct {
	Name    string
	Members []int32
}

// vanillaTagSection is the per-registry section of ClientboundUpdateTags.
type vanillaTagSection struct {
	Registry string
	Tags     []vanillaTag
}

// vanillaRegistryDataPacket builds the ClientboundRegistryData packet for one
// synchronized registry, exactly like a vanilla 26.3 server talking to a
// client that agreed to the same known pack: every entry is data-less
// (hasData=false), so the client decodes each element from its own jar via
// the resource manager it built from the announced minecraft/core/26.3 pack.
//
// Wire shape (ClientboundRegistryDataPacket.STREAM_CODEC):
//
//	Identifier registry
//	VarInt entry count
//	per entry: Identifier id, Boolean hasData, [network NBT if hasData]
func vanillaRegistryDataPacket(reg vanillaSyncRegistry) pk.Packet {
	var buf bytes.Buffer
	_, _ = pk.VarInt(len(reg.Elements)).WriteTo(&buf)
	for _, el := range reg.Elements {
		_, _ = pk.Identifier(el).WriteTo(&buf)
		_, _ = pk.Boolean(false).WriteTo(&buf)
	}
	return pk.Marshal(
		packetid.ClientboundConfigRegistryData,
		pk.Identifier(reg.Name),
		rawBytes(buf.Bytes()),
	)
}

func writeVanillaRegistryData(conn *gmnet.Conn) error {
	for _, reg := range vanillaSyncRegistries {
		if err := conn.WritePacket(vanillaRegistryDataPacket(reg)); err != nil {
			return err
		}
	}
	return nil
}

// vanillaUpdateTagsPacket assembles the full ClientboundUpdateTags from the
// embedded vanilla tag dump. The 26.3 client replaces its whole tag view with
// this payload and resolves every HolderSet tag reference against it —
// dimension_type's "#minecraft:infiniburn_overworld", the enchantments'
// "#minecraft:enchantable/*" and "#minecraft:exclusive_set/*", and the
// timeline tag the vanilla overworld dimension_type references.
func vanillaUpdateTagsPacket() pk.Packet {
	return pk.Packet{
		ID:   int32(packetid.ClientboundConfigUpdateTags),
		Data: buildVanillaUpdateTagsPayload(),
	}
}

// buildVanillaUpdateTagsPayload assembles the ClientboundUpdateTags body.
//
// Wire shape (TagNetworkSerialization.NetworkPayload):
//
//	VarInt registry count
//	per registry: Identifier registry, VarInt tag count
//	per tag: Identifier tag, VarInt member count, member raw ids as VarInt
func buildVanillaUpdateTagsPayload() []byte {
	var buf bytes.Buffer
	_, _ = pk.VarInt(len(vanillaTagSections)).WriteTo(&buf)
	for _, sec := range vanillaTagSections {
		_, _ = pk.Identifier(sec.Registry).WriteTo(&buf)
		_, _ = pk.VarInt(len(sec.Tags)).WriteTo(&buf)
		for _, tag := range sec.Tags {
			_, _ = pk.Identifier(tag.Name).WriteTo(&buf)
			_, _ = pk.VarInt(len(tag.Members)).WriteTo(&buf)
			for _, m := range tag.Members {
				_, _ = pk.VarInt(m).WriteTo(&buf)
			}
		}
	}
	return buf.Bytes()
}
