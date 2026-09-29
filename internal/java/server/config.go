package server

import (
	"bytes"
	"io"
	"log"

	"github.com/Tnze/go-mc/chat"
	"github.com/Tnze/go-mc/data/packetid"
	gmnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	gmserver "github.com/Tnze/go-mc/server"
	gmcommand "github.com/Tnze/go-mc/server/command"
)

type javaConfig struct {
	commands *gmcommand.Graph
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
	// Announce the vanilla "core" data pack before anything else. This is the
	// known-packs handshake: a genuine 26.3 client matches the pack by exact
	// id+version, replies with the same pack, and builds its data-less-entry
	// resource manager from its own jar. Every registry element below is then
	// sent data-less and the client decodes it with the real vanilla codecs
	// straight from its own data — byte-for-byte identical to a vanilla
	// server↔client join, and immune to NBT/codec encoding drift on our side.
	//
	// Verified from the 26.3 client classes: ClientConfigurationPacketListenerImpl
	// .handleSelectKnownPacks → KnownPacksManager.trySelectingPacks (pack
	// repository keyed by KnownPack "minecraft"/"core"/version), and
	// NetworkRegistryLoadTask, which resolves entries without data through the
	// resource manager created in runWithResources.
	{
		var buf bytes.Buffer
		_, _ = pk.VarInt(1).WriteTo(&buf)           // known pack count
		_, _ = pk.String("minecraft").WriteTo(&buf) // namespace
		_, _ = pk.String("core").WriteTo(&buf)      // id
		_, _ = pk.String("26.3").WriteTo(&buf)      // version
		if err := conn.WritePacket(pk.Packet{
			ID:   int32(packetid.ClientboundConfigSelectKnownPacks),
			Data: buf.Bytes(),
		}); err != nil {
			return err
		}
	}

	// Send every synchronized registry data-less, in vanilla order (see
	// vanillasync.go). This covers all 32 registries a vanilla server syncs,
	// including the ones the 26.3 client's item components hard-require
	// (block_transformer for shovel/axe/hoe, banner_pattern, enchantment,
	// decorated_pot_pattern, dialog, ...) — omitting any of them leaves the
	// client with an empty registry and "Missing element" crashes at
	// finish_configuration.
	if err := writeVanillaRegistryData(conn); err != nil {
		return err
	}

	// The complete vanilla tag set. Registry elements arrive data-less, so ALL
	// HolderSet tag references inside the client-decoded JSON resolve against
	// this payload alone: dimension_type infiniburn, enchantment enchantable /
	// exclusive_set, and the timeline tag the overworld dimension_type uses.
	if err := conn.WritePacket(vanillaUpdateTagsPacket()); err != nil {
		log.Printf("[Java] AcceptConfig: error sending UpdateTags: %v", err)
		return err
	}

	// NOTE: do NOT send ClientboundGameCommands from the configuration phase.
	// In the play-state packet id table that packet is 0x10, but in the
	// configuration-state table 0x10 is ClientboundConfigServerLinks, whose
	// payload is a non-null NBT compound tag. Sending the command graph here
	// causes the 26.1 client to try to decode it as a compound tag and crash
	// with "Failed to decode packet 'clientbound/minecraft:server_links'
	//  Caused by: Expected non-null compound tag". The Commands packet is a
	// play-state packet and is sent by SendInitialPlayPackets instead.

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
			// The client's reply lists the packs it can resolve data-less entries
			// from. If minecraft/core/26.3 is missing, every registry element we
			// send data-less will fail to load on the client, so surface it loudly
			// instead of leaving it as an opaque "Failed to parse value" crash.
			if !replyHasVanillaCorePack(p) {
				log.Printf("[Java] AcceptConfig: client did NOT select minecraft/core/26.3; " +
					"data-less registry sync will fail on this client")
			}
		case packetid.ServerboundConfigAcceptCodeOfConduct:
			// Code of conduct acceptance - ignored
		default:
			// Unknown packet, log and continue waiting for FinishConfiguration
			log.Printf("[Java] Unknown config packet: raw ID=%d (0x%x)", p.ID, p.ID)
			_ = p.Data // use p.Data to avoid compile error
		}
	}
}

// replyHasVanillaCorePack decodes a ServerboundSelectKnownPacks reply
// (VarInt count, then per pack: String namespace, String id, String version)
// and reports whether the client selected minecraft/core/26.3.
func replyHasVanillaCorePack(p pk.Packet) bool {
	r := bytes.NewReader(p.Data)
	var count pk.VarInt
	if _, err := count.ReadFrom(r); err != nil {
		return false
	}
	for i := int32(0); i < int32(count); i++ {
		var ns, id, version pk.String
		if _, err := ns.ReadFrom(r); err != nil {
			return false
		}
		if _, err := id.ReadFrom(r); err != nil {
			return false
		}
		if _, err := version.ReadFrom(r); err != nil {
			return false
		}
		if ns == "minecraft" && id == "core" && version == "26.3" {
			return true
		}
	}
	return false
}
