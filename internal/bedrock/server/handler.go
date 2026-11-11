package server

import (
	"log"
	"net"
	"strings"

	"livingworld/internal/bedrock/inventory"
	"livingworld/internal/bedrock/skin"
	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/player"
	"livingworld/internal/shared/constants/system"
	"livingworld/internal/skinbridge"
	"livingworld/internal/world"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const (
	bedrockGroundY      = int16(world.SuperflatGroundY)
	bedrockSpawnFeetY   = float32(world.SuperflatSpawnY)
	bedrockLocalRuntime = system.BedrockLocalRuntime
)

// playerPerms maps the server-side operator flag to Bedrock's PlayerPermissions
// game-rule value. Bedrock uses 0=visitor, 1=member, 2=operator. 2 is the value
// that flips the in-game "Cheats are not enabled on this world" banner OFF and
// unlocks the gamemode/time/weather selectors the server is willing to honour.
// Vanilla Bedrock servers (and Realms) send 2 to ops and 1 to everyone else;
// the previous hardcoded 1 silently downgraded ops to member and the client
// refused to show the cheat controls.
func playerPerms(isOp bool) byte {
	if isOp {
		return 2
	}
	return 1
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	mcConn := conn.(*minecraft.Conn)
	addr := conn.RemoteAddr().String()

	identity := mcConn.IdentityData()
	playerName := identity.DisplayName
	playerID, err := uuid.Parse(identity.Identity)
	if err != nil {
		playerID = uuid.New()
	}

	// Resolve op status BEFORE StartGame: PlayerPermissions is part of the
	// initial GameData and cannot be changed post-join without a re-handshake.
	// The Player struct is built later, so do the lookup now and reuse it.
	isOp := s.cfg.IsOp(playerName)

	spawn := s.cfg.World.Spawn
	spawnBlockY := int32(spawn.Y)
	// cfg.World.Spawn is already center-padded (X/Z +0.5) and surface-snapped
	// by server.New (server/new.go:44-52), so use it verbatim. The previous
	// double +0.5 shifted the feet column one block diagonally from the
	// column whose surface was computed, which could embed the player in
	// terrain on an uphill diagonal.
	feetX, feetY, feetZ := spawn.X, spawn.Y, spawn.Z
	savedBedrock, hasSavedBedrock := s.pm.LoadPlayerData(playerID)
	if hasSavedBedrock {
		// Validate before restoring: the save may predate world changes and sit
		// inside solid terrain now (see safeRestoredFeet).
		feetX, feetY, feetZ = safeRestoredFeet(s.wm.GetDefaultWorld(), savedBedrock.X, savedBedrock.Y, savedBedrock.Z)
	}
	spawnFeet := mgl32.Vec3{float32(feetX), float32(feetY), float32(feetZ)}
	spawnClientPos := bedrockLocalClientPosFromFeet(feetX, feetY, feetZ)
	doDaylight := s.cfg.World.DayNightCycle
	if s.cfg.World.AlwaysDay {
		doDaylight = false
	}

	gameData := minecraft.GameData{
		WorldName:        s.cfg.ServerName,
		WorldSeed:        s.cfg.World.Seed,
		Difficulty:       int32(s.cfg.World.DifficultyByte()),
		EntityUniqueID:   int64(bedrockLocalRuntime),
		EntityRuntimeID:  bedrockLocalRuntime,
		PlayerGameMode:   javaModeToBedrock(s.cfg.DefaultGamemode),
		PlayerPosition:   spawnClientPos,
		Pitch:            spawn.Pitch,
		Yaw:              spawn.Yaw,
		Dimension:        0,
		WorldSpawn:       protocol.BlockPos{int32(spawn.X), spawnBlockY, int32(spawn.Z)},
		WorldGameMode:    javaModeToBedrock(s.cfg.DefaultGamemode),
		Hardcore:         false,
		XBLBroadcastMode: 0,
		Time:             s.wm.GetDefaultWorld().GetDayTime(),
		GameRules: []protocol.GameRule{
			{Name: "doDaylightCycle", Value: doDaylight},
			{Name: "showcoordinates", Value: true},
			{Name: "locatorbar", Value: s.cfg.World.LocatorBar},
			{Name: "locatorBar", Value: s.cfg.World.LocatorBar},
		},
		ServerBlockStateChecksum: 0,
		PlayerMovementSettings: protocol.PlayerMovementSettings{
			RewindHistorySize:                0,
			ServerAuthoritativeBlockBreaking: true,
		},
		ChunkRadius: int32(s.cfg.Bedrock.ViewDistance),
		// PlayerPermissions drives Bedrock's "Cheats are not enabled on this
		// world" UI banner AND the in-game cheats toggle. Hardcoding 1 (member)
		// here is what kept ops from opening the gamemode selector, time controls,
		// etc. — the server was saying "this player can run /gamemode" (via
		// SetCommandsEnabled below) but the client thought "cheats are off,
		// don't show the selector". Mirror the actual op status so ops see
		// the controls the server is willing to honour.
		PlayerPermissions:   playerPerms(isOp),
		BaseGameVersion:     protocol.CurrentVersion,
		CustomBlocks:        bedrockworld.VanillaBlockEntries(),
		Items:               inventory.VanillaItemEntries(),
		PersonaDisabled:     false,
		CustomSkinsDisabled: false,
		EmoteChatMuted:      false,
		// Modern Bedrock clients (1.16.100+) require the server-authoritative
		// inventory system; with this false the client refuses to open the
		// inventory UI. gophertunnel's StartGame already sends an (empty)
		// CreativeContent, so the inventory initializes. Matches dragonfly.
		ServerAuthoritativeInventory: true,
	}

	if err := mcConn.StartGame(gameData); err != nil {
		log.Printf("[Bedrock] StartGame failed for %s: %v", addr, err)
		return
	}

	bs := newBedrockSession(playerID, playerName, system.BedrockPlayerRuntimeIDOffset+uint64(s.pm.PlayerCount()), mcConn, s.pm)
	s.addSession(bs)
	defer s.removeSession(playerID)
	pl := player.NewPlayer(playerID, playerName, player.EditionBedrock)
	pl.World = s.wm.GetDefaultWorld()
	pl.EntityRuntimeID = bs.runtimeID
	pl.Op = isOp // resolved before StartGame (PlayerPermissions needs it)
	pl.Position.X = float64(spawnFeet[0])
	pl.Position.Y = float64(spawnFeet[1])
	pl.Position.Z = float64(spawnFeet[2])
	pl.Rotation.Pitch = spawn.Pitch
	pl.Rotation.Yaw = spawn.Yaw
	pl.OnGround = true
	if hasSavedBedrock {
		pl.ApplyPersisted(savedBedrock) // restore inventory/health/gamemode
	} else {
		// First-time Bedrock join: apply the configured default gamemode.
		// ApplyPersisted would have set pl.Gamemode for returning players,
		// but brand-new ones keep the zero-value (survival) without this.
		if d := s.cfg.DefaultGamemode; d >= 0 && d <= 3 {
			pl.Gamemode = d
			pl.Creative = d == 1
		}
	}
	if s.skins != nil {
		sk := skin.SkinFromClientData(bs.clientData)
		model := "wide"
		if sk.ArmSize == protocol.ArmSizeSlim {
			model = "slim"
		}
		// Compress and normalize the Bedrock skin (64x32, 64x64, 128x128, 256x256)
		// into a perceptually faithful 64x64 PNG and RGBA buffer immediately so both
		// the local skinbridge HTTP server and MineSkin receive a valid 64x64 texture
		// with sanitized base-layer alpha.
		skinURL, normRGBA, pngData := s.skins.RegisterSkin(
			playerID,
			int(sk.SkinImageWidth),
			int(sk.SkinImageHeight),
			sk.SkinData,
			bs.clientData.SkinColour,
		)
		pl.BedrockSkinURL = skinURL
		if len(normRGBA) == skinbridge.JavaSkinRGBABytes {
			pl.Skin = player.NewSkinData(skinURL, model, normRGBA)
		} else if len(sk.SkinData) >= int(sk.SkinImageWidth)*int(sk.SkinImageHeight)*4 {
			pl.Skin = player.NewSkinData(skinURL, model, sk.SkinData)
		}

		// Upload compressed 64x64 PNG to MineSkin asynchronously (works both with and
		// without an API key, with SHA-256 caching) to obtain a Mojang-signed texture
		// property on textures.minecraft.net for vanilla Java clients.
		if len(pngData) > 0 {
			go s.uploadBedrockSkinPNGToMineSkin(playerID, playerName, pngData, model)
		}
	}
	s.pm.AddPlayer(pl)
	defer s.pm.RemovePlayer(playerID)

	// Register this session so server/plugin code can message, kick, or push the
	// player (cross-edition player pushing is server-driven).
	s.pm.SetController(playerID, bs)
	defer s.pm.RemoveController(playerID)

	// gophertunnel's StartGame hardcodes CommandsEnabled=true (which shows the
	// gamemode/cheat selector). Disable it for non-ops so they can't self-change
	// gamemode; ops keep it. Then advertise our commands so the client autocompletes.
	_ = mcConn.WritePacket(&packet.SetCommandsEnabled{Enabled: pl.Op})
	s.sendAvailableCommands(mcConn)

	s.bootstrapWorld(mcConn, s.cfg.Bedrock.ViewDistance, bs)

	teleportPlayer(mcConn, spawnClientPos, spawn.Pitch, spawn.Yaw)
	// Re-assert the configured default gamemode (with abilities + movement
	// attribute) AFTER StartGame so the client can't be stuck on a stale
	// survival mode if DefaultGamemode is creative/adventure.
	s.sendBedrockGameMode(mcConn, bedrockLocalRuntime, s.cfg.DefaultGamemode)
	s.sendLocalPlayerActorData(mcConn)
	sendInitialInventories(mcConn)
	_ = bedrockworld.SendSetTime(mcConn, int32(s.wm.GetDefaultWorld().GetDayTime()))
	s.spawnExistingForeignPlayers(bs)
	s.spawnExistingMobs(bs)
	s.sendWeatherTo(bs)
	s.spawnExistingDropsFor(bs)

	// Last step of the join sequence. Everything the client needs to build
	// the first frame — spawn chunks, time of day, weather, actors — has now
	// been queued, so release the input lock. See releaseClientInput for why
	// gophertunnel does not do this for us.
	releaseClientInput(mcConn)

	for {
		pk, err := mcConn.ReadPacket()
		if err != nil {
			if !strings.Contains(err.Error(), "context canceled") {
				log.Printf("[Bedrock] Client disconnected %s: %v", addr, err)
			}
			return
		}
		s.handlePacket(bs, pk, bs.chunkCache)
	}
}
