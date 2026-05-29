package bedrock

import (
	"log"
	"net"

	"livingworld/internal/player"
	"livingworld/internal/world"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	mcConn := conn.(*minecraft.Conn)
	addr := conn.RemoteAddr().String()

	log.Printf("[Bedrock] Client connected from %s", addr)

	identity := mcConn.IdentityData()
	playerName := identity.DisplayName
	playerID, err := uuid.Parse(identity.Identity)
	if err != nil {
		playerID = uuid.New()
	}
	log.Printf("[Bedrock] Player joining: %s", playerName)

	spawn := s.cfg.World.Spawn
	// The flat terrain has grass at Y=3, so the player's feet belong at Y=4.
	// Bedrock's StartGame/MovePlayer positions are camera/eye positions for the
	// local player, matching dragonfly's +1.62 offset.
	groundY := int16(3)
	spawnFeetY := float32(groundY + 1)
	spawnEyeY := spawnFeetY + 1.62
	spawnBlockY := int32(groundY + 1)

	gameData := minecraft.GameData{
		WorldName:        s.cfg.ServerName,
		WorldSeed:        s.cfg.World.Seed,
		Difficulty:       0,
		EntityUniqueID:   1,
		EntityRuntimeID:  1,
		PlayerGameMode:   0,
		PlayerPosition:   mgl32.Vec3{float32(spawn.X) + 0.5, spawnEyeY, float32(spawn.Z) + 0.5},
		Pitch:            spawn.Pitch,
		Yaw:              spawn.Yaw,
		Dimension:        0,
		WorldSpawn:       protocol.BlockPos{int32(spawn.X), spawnBlockY, int32(spawn.Z)},
		WorldGameMode:    0,
		Hardcore:         false,
		XBLBroadcastMode: 0,
		Time:             6000, // noon — also enforced via SetTime after bootstrap
		GameRules: []protocol.GameRule{
			{Name: "doDaylightCycle", Value: false},
		},
		ServerBlockStateChecksum: 0,
		PlayerMovementSettings: protocol.PlayerMovementSettings{
			RewindHistorySize:                0,
			ServerAuthoritativeBlockBreaking: false,
		},
		ChunkRadius:         int32(s.cfg.Bedrock.ViewDistance),
		PlayerPermissions:   1,
		BaseGameVersion:     protocol.CurrentVersion,
		Items:               vanillaItemEntries(),
		PersonaDisabled:     true,
		CustomSkinsDisabled: true,
		EmoteChatMuted:      false,
	}

	if err := mcConn.StartGame(gameData); err != nil {
		log.Printf("[Bedrock] StartGame failed for %s: %v", addr, err)
		return
	}

	log.Printf("[Bedrock] Client %s spawned successfully", addr)

	bs := newBedrockSession(playerID, playerName, uint64(100000+s.pm.PlayerCount()), mcConn)
	s.addSession(bs)
	defer s.removeSession(playerID)
	pl := player.NewPlayer(playerID, playerName, player.EditionBedrock)
	pl.EntityRuntimeID = bs.runtimeID
	pl.Position.X = float64(spawn.X) + 0.5
	pl.Position.Y = float64(spawnFeetY)
	pl.Position.Z = float64(spawn.Z) + 0.5
	pl.Rotation.Pitch = spawn.Pitch
	pl.Rotation.Yaw = spawn.Yaw
	pl.OnGround = true
	s.pm.AddPlayer(pl)
	defer s.pm.RemovePlayer(playerID)

	// Per-connection chunk cache (populated during bootstrap, used by SubChunkRequest).
	chunkCache := make(map[protocol.ChunkPos]*dfchunk.Chunk)
	s.bootstrapWorld(mcConn, s.cfg.Bedrock.ViewDistance, chunkCache)

	// Force the local player above the loaded terrain after chunks are sent.
	teleportPlayer(mcConn, mgl32.Vec3{float32(spawn.X) + 0.5, spawnEyeY, float32(spawn.Z) + 0.5}, spawn.Pitch, spawn.Yaw)

	// Force noon (sun directly overhead) — StartGame.Time alone isn't enough,
	// the Bedrock client resets to 0 and auto-advances. Send SetTime after
	// bootstrap to lock the sun at noon.
	_ = sendSetTime(mcConn, 6000)
	s.spawnExistingForeignPlayers(bs)

	for {
		pk, err := mcConn.ReadPacket()
		if err != nil {
			log.Printf("[Bedrock] Client disconnected %s: %v", addr, err)
			return
		}
		s.handlePacket(bs, pk, chunkCache)
	}
}

func teleportPlayer(conn *minecraft.Conn, pos mgl32.Vec3, pitch, yaw float32) {
	_ = conn.WritePacket(&packet.MovePlayer{
		EntityRuntimeID: 1,
		Position:        pos,
		Pitch:           pitch,
		Yaw:             yaw,
		HeadYaw:         yaw,
		Mode:            packet.MoveModeTeleport,
		OnGround:        true,
		TeleportCause:   packet.TeleportCauseCommand,
	})
}

func (s *Server) handlePacket(bs *bedrockSession, pk packet.Packet, chunkCache map[protocol.ChunkPos]*dfchunk.Chunk) {
	conn := bs.conn
	switch p := pk.(type) {
	case *packet.RequestChunkRadius:
		r := int(p.ChunkRadius)
		if r <= 0 || r > s.cfg.Bedrock.ViewDistance {
			r = s.cfg.Bedrock.ViewDistance
		}
		s.bootstrapWorld(conn, r, chunkCache)

	case *packet.SubChunkRequest:
		s.converter.handleSubChunkRequest(conn, p, chunkCache)

	case *packet.MovePlayer:
		// Bedrock reports the local player's camera/eye position. Store feet Y in
		// the shared model so Java-side avatars stand on the ground.
		s.pm.UpdatePosition(bs.id, float64(p.Position[0]), float64(p.Position[1]-1.62), float64(p.Position[2]), p.Pitch, p.Yaw, p.OnGround)

	case *packet.PlayerAuthInput:
		s.pm.UpdatePosition(bs.id, float64(p.Position[0]), float64(p.Position[1]-1.62), float64(p.Position[2]), p.Pitch, p.Yaw, true)

	case *packet.InventoryTransaction:
		if p.TransactionData != nil {
			switch data := p.TransactionData.(type) {
			case *protocol.UseItemTransactionData:
				switch data.ActionType {
				case protocol.UseItemActionClickBlock:
					blockPos := data.BlockPosition
					face := data.BlockFace
					x, y, z := blockPos[0], blockPos[1], blockPos[2]
					switch face {
					case 0:
						y--
					case 1:
						y++
					case 2:
						z--
					case 3:
						z++
					case 4:
						x--
					case 5:
						x++
					}
					placeRID, _ := dfchunk.StateToRuntimeID("minecraft:stone", nil)
					_ = conn.WritePacket(&packet.UpdateBlock{
						Position:          protocol.BlockPos{x, y, z},
						NewBlockRuntimeID: placeRID,
						Layer:             0,
					})
					s.wm.GetDefaultWorld().SetBlock(int(x), int(y), int(z), world.PlaceholderBlock{IDValue: 1})
				}
			}
		}

	case *packet.PlayerAction:
		if p.ActionType == 1 {
			blockPos := p.BlockPosition
			airRID, _ := dfchunk.StateToRuntimeID("minecraft:air", nil)
			_ = conn.WritePacket(&packet.UpdateBlock{
				Position:          blockPos,
				NewBlockRuntimeID: airRID,
				Layer:             0,
			})
			s.wm.GetDefaultWorld().SetBlock(int(blockPos[0]), int(blockPos[1]), int(blockPos[2]), world.BlockAir{})
		}

	case *packet.Text:

	case *packet.ResourcePackClientResponse:
		log.Printf("[Bedrock] Resource pack response: %d", p.Response)

	case *packet.PacketViolationWarning:
		log.Printf("[Bedrock] Packet violation: %v", p)
	}
}
