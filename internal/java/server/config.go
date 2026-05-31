package server

import (
	"bytes"
	"io"
	"log"
	"sort"
	"strings"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	gmnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/registry"
	gmserver "github.com/Tnze/go-mc/server"
)

type javaConfig struct {
	registries    registry.Registries
	registrySizes map[string]int
}

type javaListPing struct {
	ping       *gmserver.PingInfo
	playerList *gmserver.PlayerList
	sessions   *SessionManager
}

func (j *javaListPing) Name() string                           { return j.ping.Name() }
func (j *javaListPing) Protocol(v int32) int                   { return j.ping.Protocol(v) }
func (j *javaListPing) MaxPlayer() int                         { return j.playerList.MaxPlayer() }
func (j *javaListPing) OnlinePlayer() int                      { return j.sessions.Count() }
func (j *javaListPing) PlayerSamples() []gmserver.PlayerSample { return j.playerList.PlayerSamples() }
func (j *javaListPing) Description() *chat.Message             { return j.ping.Description() }
func (j *javaListPing) FavIcon() string                        { return j.ping.FavIcon() }

type rawBytes []byte

func (r rawBytes) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(r)
	return int64(n), err
}

func (c *javaConfig) AcceptConfig(conn *gmnet.Conn) error {
	// Announce the vanilla "core" data pack BEFORE sending registries. This keeps
	// the 26.1 client's resource provider initialised the same way it was when
	// 26.1 already worked. NOTE: the client matches a known pack by exact
	// id+version, so this announced version only matches one patch build; we
	// therefore no longer depend on it for timeline (see the timeline block below,
	// which now sends full data). The packet stays as a harmless no-op for the
	// patch builds whose version differs.
	// Verified from 26.1 client: ClientConfigurationPacketListenerImpl.runWithResources.
	{
		var buf bytes.Buffer
		_, _ = pk.VarInt(1).WriteTo(&buf)           // known pack count
		_, _ = pk.String("minecraft").WriteTo(&buf) // namespace
		_, _ = pk.String("core").WriteTo(&buf)      // id
		_, _ = pk.String("26.1").WriteTo(&buf)      // version
		if err := conn.WritePacket(pk.Packet{
			ID:   int32(packetid.ClientboundConfigSelectKnownPacks),
			Data: buf.Bytes(),
		}); err != nil {
			return err
		}
	}

	keys := make([]string, 0, len(c.registrySizes))
	for id, count := range c.registrySizes {
		if count > 0 {
			keys = append(keys, id)
		}
	}
	sort.Strings(keys)
	for _, id := range keys {
		reg := c.registries.Registry(id)
		if err := conn.WritePacket(pk.Marshal(
			packetid.ClientboundConfigRegistryData,
			pk.Identifier(id),
			reg,
		)); err != nil {
			return err
		}
	}

	// Send world_clock registry for 26.1 (1.21.4)
	{
		var buf bytes.Buffer
		_, _ = pk.VarInt(1).WriteTo(&buf)                         // count
		_, _ = pk.Identifier("minecraft:overworld").WriteTo(&buf) // entry id
		_, _ = pk.Boolean(true).WriteTo(&buf)                     // has data
		_, _ = buf.Write([]byte{0x0a, 0x00})                      // empty NBT compound

		if err := conn.WritePacket(pk.Marshal(
			packetid.ClientboundConfigRegistryData,
			pk.Identifier("minecraft:world_clock"),
			rawBytes(buf.Bytes()),
		)); err != nil {
			return err
		}
	}

	// Send the minecraft:timeline registry for 26.1. The timeline elements carry
	// the sun_angle / moon_angle / star_angle keyframe tracks that actually move
	// the sun; without this registry the dimension_type's "timelines" reference is
	// unbound and the client rejects the join. We send each element WITH its full
	// NBT data (extracted from 26.1.2.jar) rather than relying on the client's
	// built-in "core" pack: protocol 775 spans 26.1/26.1.1/26.1.2 and the client
	// matches known packs by exact version, which the server cannot know — so the
	// data-less approach only worked for whichever single version we announced and
	// crashed the rest ("Unbound values in registry minecraft:timeline" on 26.1.2).
	// See timeline.go. Order matches the dimension_type.timelines id list.
	{
		timelineIDs := []string{
			"minecraft:day", "minecraft:moon",
			"minecraft:early_game", "minecraft:villager_schedule",
		}
		var buf bytes.Buffer
		_, _ = pk.VarInt(len(timelineIDs)).WriteTo(&buf) // entry count
		for _, id := range timelineIDs {
			data, err := timelineNBT(strings.TrimPrefix(id, "minecraft:"))
			if err != nil {
				return err
			}
			_, _ = pk.Identifier(id).WriteTo(&buf)
			_, _ = pk.Boolean(true).WriteTo(&buf) // hasData=true
			_, _ = buf.Write(data)                // network NBT (nameless compound)
		}
		if err := conn.WritePacket(pk.Marshal(
			packetid.ClientboundConfigRegistryData,
			pk.Identifier("minecraft:timeline"),
			rawBytes(buf.Bytes()),
		)); err != nil {
			return err
		}
	}

	var tagsBuf bytes.Buffer
	var tagsRegCount pk.VarInt = 2
	_, _ = tagsRegCount.WriteTo(&tagsBuf)

	var damageTypeRegID pk.Identifier = "minecraft:damage_type"
	_, _ = damageTypeRegID.WriteTo(&tagsBuf)

	damageTypeTags := []string{
		"minecraft:is_fire", "minecraft:bypasses_armor", "minecraft:bypasses_shield",
		"minecraft:bypasses_invulnerability", "minecraft:bypasses_cooldown",
		"minecraft:bypasses_effects", "minecraft:bypasses_enchantments",
		"minecraft:bypasses_resistance", "minecraft:is_projectile", "minecraft:is_explosion",
		"minecraft:is_fall", "minecraft:is_drowning", "minecraft:is_freezing",
		"minecraft:is_lightning", "minecraft:no_anger", "minecraft:no_impact",
		"minecraft:always_kills_armor_stands", "minecraft:can_break_armor_stands",
		"minecraft:avoid_vibration", "minecraft:ignites_armor_stands",
		"minecraft:burns_armor_stands", "minecraft:wither_summon_killer",
		"minecraft:panic_causes", "minecraft:panic_environmental_causes",
	}

	var tagCount pk.VarInt = pk.VarInt(len(damageTypeTags))
	_, _ = tagCount.WriteTo(&tagsBuf)

	for _, tagName := range damageTypeTags {
		var tagID pk.Identifier = pk.Identifier(tagName)
		_, _ = tagID.WriteTo(&tagsBuf)
		var entryCount pk.VarInt = 0
		_, _ = entryCount.WriteTo(&tagsBuf)
	}

	var bannerPatternRegID pk.Identifier = "minecraft:banner_pattern"
	_, _ = bannerPatternRegID.WriteTo(&tagsBuf)

	bannerPatternTags := []string{
		"minecraft:pattern_item/flower", "minecraft:pattern_item/creeper",
		"minecraft:pattern_item/skull", "minecraft:pattern_item/mojang",
		"minecraft:pattern_item/globe", "minecraft:pattern_item/piglin",
		"minecraft:pattern_item/flow", "minecraft:pattern_item/guster",
		"minecraft:pattern_item/field_masoned", "minecraft:pattern_item/bordure_indented",
	}

	var bpTagCount pk.VarInt = pk.VarInt(len(bannerPatternTags))
	_, _ = bpTagCount.WriteTo(&tagsBuf)

	for _, tagName := range bannerPatternTags {
		var tagID pk.Identifier = pk.Identifier(tagName)
		_, _ = tagID.WriteTo(&tagsBuf)
		var entryCount pk.VarInt = 0
		_, _ = entryCount.WriteTo(&tagsBuf)
	}

	if err := conn.WritePacket(pk.Packet{
		ID:   int32(packetid.ClientboundConfigUpdateTags),
		Data: tagsBuf.Bytes(),
	}); err != nil {
		log.Printf("[Java] AcceptConfig: error sending UpdateTags: %v", err)
		return err
	}

	// Send FinishConfiguration to transition client to play state
	if err := conn.WritePacket(pk.Marshal(packetid.ClientboundConfigFinishConfiguration)); err != nil {
		log.Printf("[Java] AcceptConfig: error sending FinishConfiguration: %v", err)
		return err
	}

	for {
		var p pk.Packet
		if err := conn.ReadPacket(&p); err != nil {
			log.Printf("[Java] AcceptConfig: error reading packet: %v", err)
			return err
		}
		switch packetid.ServerboundPacketID(p.ID) {
		case packetid.ServerboundConfigFinishConfiguration:
			return nil // Config complete, proceed to Play state
		case packetid.ServerboundConfigKeepAlive:
			// Respond with same ID to keep connection alive
			var keepAliveID pk.VarLong
			if err := p.Scan(&keepAliveID); err == nil {
				_ = conn.WritePacket(pk.Marshal(packetid.ClientboundConfigPing, pk.Int(int64(keepAliveID))))
			}
		case packetid.ServerboundConfigClientInformation:
			// Client information (locale, view distance, etc.) - can be ignored for now
		case packetid.ServerboundConfigCustomPayload:
			// Custom plugin messages - ignored
		case packetid.ServerboundConfigCustomClickAction:
			// Click actions during configuration - ignored
		case packetid.ServerboundConfigCookieResponse:
			// Cookie responses - ignored
		case packetid.ServerboundConfigPong:
			// Pong response to server ping - ignored
		case packetid.ServerboundConfigResourcePack:
			// Resource pack response - ignored
		case packetid.ServerboundConfigSelectKnownPacks:
			// Pack selection - ignored
		case packetid.ServerboundConfigAcceptCodeOfConduct:
			// Code of conduct acceptance - ignored
		default:
			// Unknown packet, log and continue waiting for FinishConfiguration
			log.Printf("[Java] Unknown config packet: raw ID=%d (0x%x)", p.ID, p.ID)
			_ = p.Data // use p.Data to avoid compile error
		}
	}
}
