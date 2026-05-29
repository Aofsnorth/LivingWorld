package server

import (
	"livingworld/internal/java/protocol"
	"livingworld/internal/java/skin"
	"log"
	"time"

	"livingworld/internal/player"
	"livingworld/internal/plugin"

	"github.com/Tnze/go-mc/data/packetid"
	gmnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/Tnze/go-mc/yggdrasil/user"
	"github.com/google/uuid"
)

func (j *javaBridge) AcceptPlayer(name string, id uuid.UUID, _ *user.PublicKey, properties []user.Property, clientProtocol int32, conn *gmnet.Conn) {
	defer conn.Close()

	log.Printf("[Java] Player joining: %s (UUID: %s), protocol=%d", name, id, clientProtocol)

	session := NewPlayerSession(name, id, conn, j)
	versionHandler, ok := protocol.GetVersionHandler(int(clientProtocol))
	if !ok {
		log.Printf("[Java] Unsupported client protocol %d joining. Rejecting connection.", clientProtocol)
		return
	}
	session.version = versionHandler

	go session.ChunkWorker()
	defer close(session.chunkQueue)

	j.sessions.Add(session)
	defer j.sessions.Remove(id)

	plugin.Manager().Emit(&plugin.PlayerJoinEvent{
		BaseEvent:  plugin.BaseEvent{Type_: plugin.EventPlayerJoin},
		PlayerName: name,
	})

	log.Printf("[Java] Calling sendInitialPlayPackets for %s", name)
	if err := session.sendInitialPlayPackets(); err != nil {
		log.Printf("[Java] Initial play packet error: %v", err)
		return
	}
	log.Printf("[Java] Player %s joined (entityID=%d)", name, session.EntityID())

	var skinURL string
	var model string = "wide"
	for _, prop := range properties {
		if prop.Name == "textures" {
			skinURL, model = skin.ParseJavaSkinProperty(prop.Value)
			break
		}
	}

	// In offline mode, the client doesn't send textures. Fall back to Mojang
	// API lookup by username so the player's official skin can still be shown
	// to Bedrock viewers.
	if skinURL == "" {
		log.Printf("[Java] No textures property from client for %s, trying Mojang API", name)
		skinURL, model = skin.FetchMojangSkin(name)
	}

	var skinData []byte
	if skinURL != "" {
		log.Printf("[Java] Downloading skin for %s: url=%s model=%s", name, skinURL, model)
		if rgba, err := skin.DownloadAndDecodeSkin(skinURL); err == nil {
			skinData = rgba
			log.Printf("[Java] Skin downloaded for %s: %d bytes (%dx pixels)", name, len(rgba), len(rgba)/4)
		} else {
			log.Printf("[Java] Failed to download skin for %s: %v", name, err)
		}
	} else {
		log.Printf("[Java] No skin texture URL found for %s", name)
	}

	pl := player.NewPlayer(id, name, player.EditionJava)
	pl.EntityRuntimeID = uint64(session.EntityID())
	pl.Position.X, pl.Position.Y, pl.Position.Z = session.X, session.Y, session.Z
	pl.Rotation.Pitch, pl.Rotation.Yaw = session.Pitch, session.Yaw
	pl.OnGround = true
	pl.ProfileProperties = javaProfileProperties(properties)
	if len(skinData) > 0 {
		pl.Skin = player.NewSkinData("java_"+id.String(), model, skinData)
	}
	j.pm.AddPlayer(pl)
	defer j.pm.RemovePlayer(id)
	session.spawnExistingForeignPlayers()

	done := make(chan struct{})
	go func() {
		// The vanilla client disconnects if it gets no KeepAlive for 15s, so we
		// must send well inside that window. 10s matches the vanilla server.
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := conn.WritePacket(pk.Marshal(packetid.ClientboundGameKeepAlive, pk.Long(time.Now().UnixMilli()))); err != nil {
					log.Printf("[Java] KeepAlive send error: %v", err)
					return
				}
			}
		}
	}()

	for {
		var p pk.Packet
		if err := conn.ReadPacket(&p); err != nil {
			log.Printf("[Java] Player %s disconnected: %v", name, err)
			break
		}
		session.HandlePacket(p)
	}
	close(done)

	j.sessions.Broadcast(pk.Marshal(
		packetid.ClientboundGameSystemChat,
		pk.NBT(chatText{Text: name + " left the game"}),
		pk.Boolean(false),
	))
}
