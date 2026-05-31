package server

import (
	"livingworld/internal/java/protocol"
	"livingworld/internal/java/skin"
	"log"
	"time"

	"livingworld/internal/player"
	"livingworld/plugin"

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

	// Restore saved player data (position/health/food) before the initial spawn
	// packets so the client respawns where it logged out.
	saved, hasSaved := j.pm.LoadPlayerData(id)
	if hasSaved {
		session.X, session.Y, session.Z = saved.X, saved.Y, saved.Z
		session.Yaw, session.Pitch = saved.Yaw, saved.Pitch
		if saved.Health > 0 {
			session.Health = saved.Health
		}
		if saved.Food > 0 {
			session.Food = int32(saved.Food)
		}
	}

	go session.ChunkWorker()
	defer close(session.chunkQueue)
	go session.sendLoop()
	defer close(session.sendQueue)

	j.sessions.Add(session)
	defer j.sessions.Remove(id)

	plugin.Manager().Emit(&plugin.PlayerJoinEvent{
		BaseEvent:  plugin.BaseEvent{Type_: plugin.EventPlayerJoin},
		PlayerName: name,
		UUID:       id.String(),
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

	// In offline mode, the client doesn't send textures. Fall back to the
	// configured skin source by username so the player's skin can still be shown
	// (to themselves on authlib-injector launchers, and to Bedrock viewers).
	var value, signature string
	if skinURL == "" {
		switch j.cfg.Java.SkinSource {
		case "mojang":
			log.Printf("[Java] No client textures for %s, trying Mojang API", name)
			skinURL, model, value, signature = skin.FetchMojangSkin(name)
		case "ely", "elyby", "ely.by":
			log.Printf("[Java] No client textures for %s, fetching skin from Ely.by", name)
			skinURL, model, value, signature = skin.FetchElySkin(name)
		case "none", "off", "disabled":
			log.Printf("[Java] No client textures for %s, skin source disabled", name)
		default: // "auto" or unset — try every source (Ely.by then Mojang)
			log.Printf("[Java] No client textures for %s, auto-resolving skin", name)
			skinURL, model, value, signature = skin.FetchAnySkin(name)
		}
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
	pl.Op = j.cfg.IsOp(name)
	pl.EntityRuntimeID = uint64(session.EntityID())
	pl.Position.X, pl.Position.Y, pl.Position.Z = session.X, session.Y, session.Z
	pl.Rotation.Pitch, pl.Rotation.Yaw = session.Pitch, session.Yaw
	pl.OnGround = true
	if hasSaved {
		pl.ApplyPersisted(saved) // restore inventory/gamemode/health
	}
	pl.ProfileProperties = javaProfileProperties(properties)
	if len(pl.ProfileProperties) == 0 && value != "" {
		pl.ProfileProperties = []player.ProfileProperty{{
			Name:      "textures",
			Value:     value,
			Signature: signature,
		}}
	}
	if len(skinData) > 0 {
		pl.Skin = player.NewSkinData("java_"+id.String(), model, skinData)
	}
	j.pm.AddPlayer(pl)
	defer j.pm.RemovePlayer(id)

	// Register this session so server/plugin code can message or kick the player.
	j.pm.SetController(id, session)
	defer j.pm.RemoveController(id)

	// Send the player info to themselves so they see their own skin in inventory/third-person.
	_ = session.sendPlayerInfoAdd(pl.Snapshot())

	// Send initial metadata to themselves (including skin parts)
	_ = session.version.UpdateForeignMetadata(session, pl.Snapshot())

	// Mark Ready BEFORE the catch-up spawn: spawnForeignAvatar (and the async
	// event handlers) early-return while !Ready. If we spawn first, every
	// already-online player — e.g. a Bedrock player who joined earlier — is
	// skipped and stays invisible to this client.
	session.Ready = true
	session.spawnExistingForeignPlayers()
	session.spawnExistingMobs()
	session.sendWeather()

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
				if err := session.SendPacket(pk.Marshal(packetid.ClientboundGameKeepAlive, pk.Long(time.Now().UnixMilli()))); err != nil {
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
}
