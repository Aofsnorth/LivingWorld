package bedrock

import (
	"log"
	"net"
	"time"

	"livingworld/internal/player"
	lwworld "livingworld/internal/world"

	dfchunk "github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const (
	bedrockGroundY      = int16(lwworld.SuperflatGroundY)
	bedrockSpawnFeetY   = float32(lwworld.SuperflatSpawnY)
	bedrockLocalRuntime = uint64(1)
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
	spawnBlockY := int32(lwworld.SuperflatSpawnY)
	spawnFeet := mgl32.Vec3{float32(spawn.X) + 0.5, bedrockSpawnFeetY, float32(spawn.Z) + 0.5}
	spawnClientPos := bedrockLocalClientPosFromFeet(float64(spawnFeet[0]), float64(spawnFeet[1]), float64(spawnFeet[2]))

	gameData := minecraft.GameData{
		WorldName:        s.cfg.ServerName,
		WorldSeed:        s.cfg.World.Seed,
		Difficulty:       0,
		EntityUniqueID:   int64(bedrockLocalRuntime),
		EntityRuntimeID:  bedrockLocalRuntime,
		PlayerGameMode:   packet.GameTypeSurvival,
		PlayerPosition:   spawnClientPos,
		Pitch:            spawn.Pitch,
		Yaw:              spawn.Yaw,
		Dimension:        0,
		WorldSpawn:       protocol.BlockPos{int32(spawn.X), spawnBlockY, int32(spawn.Z)},
		WorldGameMode:    packet.GameTypeSurvival,
		Hardcore:         false,
		XBLBroadcastMode: 0,
		Time:             6000,
		GameRules: []protocol.GameRule{
			{Name: "doDaylightCycle", Value: false},
			{Name: "showcoordinates", Value: true},
		},
		ServerBlockStateChecksum:     0,
		PlayerMovementSettings:       protocol.PlayerMovementSettings{},
		ChunkRadius:                  int32(s.cfg.Bedrock.ViewDistance),
		PlayerPermissions:            1, // member, not operator: prevents settings/commands gamemode changes.
		BaseGameVersion:              protocol.CurrentVersion,
		Items:                        vanillaItemEntries(),
		PersonaDisabled:              false,
		CustomSkinsDisabled:          false,
		EmoteChatMuted:               false,
		ServerAuthoritativeInventory: false,
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
	pl.Position.X = float64(spawnFeet[0])
	pl.Position.Y = float64(spawnFeet[1])
	pl.Position.Z = float64(spawnFeet[2])
	pl.Rotation.Pitch = spawn.Pitch
	pl.Rotation.Yaw = spawn.Yaw
	pl.OnGround = true
	if s.skins != nil {
		skin := skinFromClientData(bs.clientData)
		pl.BedrockSkinURL = s.skins.RegisterRGBA(playerID, int(skin.SkinImageWidth), int(skin.SkinImageHeight), skin.SkinData)
	}
	s.pm.AddPlayer(pl)
	defer s.pm.RemovePlayer(playerID)

	chunkCache := make(map[protocol.ChunkPos]*dfchunk.Chunk)
	s.bootstrapWorld(mcConn, s.cfg.Bedrock.ViewDistance, chunkCache)

	teleportPlayer(mcConn, spawnClientPos, spawn.Pitch, spawn.Yaw)
	s.sendBedrockSurvivalState(mcConn, bedrockLocalRuntime, playerName)
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
		EntityRuntimeID: bedrockLocalRuntime,
		Position:        pos,
		Pitch:           pitch,
		Yaw:             yaw,
		HeadYaw:         yaw,
		Mode:            packet.MoveModeTeleport,
		OnGround:        true,
		TeleportCause:   packet.TeleportCauseCommand,
	})
}

func (s *Server) sendBedrockSurvivalState(conn *minecraft.Conn, runtimeID uint64, name string) {
	// Keep this deliberately minimal. Sending custom UpdateAbilities layers with
	// an incomplete/incorrect bitset made the Bedrock client enter a broken
	// low-gravity/fly-like movement state and showed underwater bubbles. StartGame
	// already establishes survival; this packet reasserts the gamemode without
	// touching physics abilities.
	_ = conn.WritePacket(&packet.SetPlayerGameType{GameType: packet.GameTypeSurvival})
}

func adjacentBlockPos(pos protocol.BlockPos, face int32) protocol.BlockPos {
	x, y, z := pos[0], pos[1], pos[2]
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
	return protocol.BlockPos{x, y, z}
}

func (s *Server) resyncBedrockBlock(conn *minecraft.Conn, pos protocol.BlockPos) {
	blockID := s.wm.GetDefaultWorld().GetBlock(int(pos[0]), int(pos[1]), int(pos[2])).ID()
	_ = conn.WritePacket(&packet.UpdateBlock{
		Position:          pos,
		NewBlockRuntimeID: livingWorldBlockIDToBedrockRID(blockID),
		Flags:             packet.BlockUpdateNetwork | packet.BlockUpdateNeighbours,
		Layer:             0,
	})
}

func (s *Server) breakBedrockBlock(pos protocol.BlockPos) {
	// Do not allow breaking bedrock or air. This is still a minimal survival
	// placeholder; real hardness/drop logic belongs in a block service.
	current := s.wm.GetDefaultWorld().GetBlock(int(pos[0]), int(pos[1]), int(pos[2]))
	if current.ID() == 0 || current.ID() == 1 {
		return
	}
	s.wm.SetBlockAndPublish(lwworld.BlockUpdateSourceBedrock, int(pos[0]), int(pos[1]), int(pos[2]), lwworld.BlockAir{})
}

func isBedrockBreakAction(action int32) bool {
	switch action {
	case protocol.PlayerActionStopBreak, protocol.PlayerActionPredictDestroyBlock, protocol.PlayerActionCreativePlayerDestroyBlock:
		return true
	default:
		return false
	}
}

func (s *Server) publishBedrockMove(bs *bedrockSession, clientPos mgl32.Vec3, pitch, yaw float32, onGround bool) {
	now := time.Now()
	x, y, z := bedrockSharedFeetFromLocalClient(clientPos)
	publish, correct := bs.movementUpdate(now, x, y, z)
	if correct {
		// Do not hard-freeze the local Bedrock player for now; just ignore the
		// impossible sample and reassert survival abilities. Hard correction here
		// previously made movement feel broken/floaty.
		s.sendBedrockSurvivalState(bs.conn, bedrockLocalRuntime, bs.username)
		return
	}
	if publish {
		s.pm.UpdatePosition(bs.id, x, y, z, pitch, yaw, onGround)
	}
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
		s.sendBedrockSurvivalState(conn, bedrockLocalRuntime, bs.username)

	case *packet.SubChunkRequest:
		s.converter.handleSubChunkRequest(conn, p, chunkCache)

	case *packet.MovePlayer:
		// If PlayerAuthInput is active, don't process a second movement source in
		// the same tick. Double-processing was one source of ultra-fast movement.
		if time.Since(bs.lastAuthInputAt) > 100*time.Millisecond {
			s.publishBedrockMove(bs, p.Position, p.Pitch, p.Yaw, p.OnGround)
		}

	case *packet.PlayerAuthInput:
		// Modern Bedrock clients commonly use PlayerAuthInput for movement. Use
		// it as the primary movement source and ignore duplicate MovePlayer bursts.
		bs.lastAuthInputAt = time.Now()
		s.publishBedrockMove(bs, p.Position, p.Pitch, p.HeadYaw, p.InputData.Load(packet.InputFlagVerticalCollision))
		for _, action := range p.BlockActions {
			if isBedrockBreakAction(action.Action) {
				s.breakBedrockBlock(action.BlockPos)
			}
		}

	case *packet.InventoryTransaction:
		if p.TransactionData != nil {
			switch data := p.TransactionData.(type) {
			case *protocol.UseItemTransactionData:
				switch data.ActionType {
				case protocol.UseItemActionClickBlock:
					s.resyncBedrockBlock(conn, data.BlockPosition)
					s.resyncBedrockBlock(conn, adjacentBlockPos(data.BlockPosition, data.BlockFace))
				case protocol.UseItemActionBreakBlock:
					s.breakBedrockBlock(data.BlockPosition)
				}
			}
		}

	case *packet.RequestAbility:
		// The settings UI may let the client request ability/gamemode-like
		// changes. This server is authoritative survival, so always reject by
		// re-sending survival abilities.
		s.sendBedrockSurvivalState(conn, bedrockLocalRuntime, bs.username)

	case *packet.PlayerAction:
		switch p.ActionType {
		case protocol.PlayerActionStopBreak, protocol.PlayerActionPredictDestroyBlock:
			s.breakBedrockBlock(p.BlockPosition)
		case protocol.PlayerActionStartFlying, protocol.PlayerActionStopFlying:
			// Client attempted to toggle flight through settings/controls. Re-assert survival.
			s.sendBedrockSurvivalState(conn, bedrockLocalRuntime, bs.username)
		}

	case *packet.ItemStackRequest:
		responses := make([]protocol.ItemStackResponse, 0, len(p.Requests))
		for _, req := range p.Requests {
			responses = append(responses, rejectItemStackRequest(req.RequestID))
		}
		_ = conn.WritePacket(&packet.ItemStackResponse{Responses: responses})

	case *packet.Text:

	case *packet.ResourcePackClientResponse:
		log.Printf("[Bedrock] Resource pack response: %d", p.Response)

	case *packet.PacketViolationWarning:
		log.Printf("[Bedrock] Packet violation: %v", p)
	}
}
