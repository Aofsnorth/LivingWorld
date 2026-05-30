package server

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"livingworld/internal/bedrock/inventory"
	"livingworld/internal/bedrock/skin"
	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/command"
	"livingworld/internal/player"
	lwworld "livingworld/internal/world"
	"livingworld/plugin"

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
		Difficulty:       int32(s.cfg.World.DifficultyByte()),
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
		Time:             s.wm.GetDefaultWorld().GetDayTime(),
		GameRules: []protocol.GameRule{
			{Name: "doDaylightCycle", Value: false},
			{Name: "showcoordinates", Value: true},
		},
		ServerBlockStateChecksum: 0,
		PlayerMovementSettings: protocol.PlayerMovementSettings{
			RewindHistorySize:                0,
			ServerAuthoritativeBlockBreaking: true,
		},
		ChunkRadius:         int32(s.cfg.Bedrock.ViewDistance),
		PlayerPermissions:   1, // member, not operator: prevents settings/commands gamemode changes.
		BaseGameVersion:     protocol.CurrentVersion,
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

	log.Printf("[Bedrock] Client %s spawned successfully", addr)

	bs := newBedrockSession(playerID, playerName, uint64(100000+s.pm.PlayerCount()), mcConn, s.pm)
	s.addSession(bs)
	defer s.removeSession(playerID)
	pl := player.NewPlayer(playerID, playerName, player.EditionBedrock)
	pl.EntityRuntimeID = bs.runtimeID
	pl.Op = s.cfg.IsOp(playerName)
	pl.Position.X = float64(spawnFeet[0])
	pl.Position.Y = float64(spawnFeet[1])
	pl.Position.Z = float64(spawnFeet[2])
	pl.Rotation.Pitch = spawn.Pitch
	pl.Rotation.Yaw = spawn.Yaw
	pl.OnGround = true
	if s.skins != nil {
		sk := skin.SkinFromClientData(bs.clientData)
		pl.BedrockSkinURL = s.skins.RegisterRGBA(playerID, int(sk.SkinImageWidth), int(sk.SkinImageHeight), sk.SkinData)
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

	chunkCache := make(map[protocol.ChunkPos]*dfchunk.Chunk)
	s.bootstrapWorld(mcConn, s.cfg.Bedrock.ViewDistance, chunkCache)

	teleportPlayer(mcConn, spawnClientPos, spawn.Pitch, spawn.Yaw)
	s.sendBedrockSurvivalState(mcConn, bedrockLocalRuntime)
	s.sendLocalPlayerActorData(mcConn)
	sendInitialInventories(mcConn)
	_ = bedrockworld.SendSetTime(mcConn, int32(s.wm.GetDefaultWorld().GetDayTime()))
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

// sendInitialInventories initializes the player's inventory windows so the
// Bedrock client will actually render the inventory UI when opened. With the
// server-authoritative inventory system the client keeps the screen closed
// (player just freezes) until these windows have been given content, even if
// empty. Sizes match dragonfly: main 36, armour 4, off-hand 1, UI 54.
func sendInitialInventories(conn *minecraft.Conn) {
	send := func(windowID uint32, size int) {
		_ = conn.WritePacket(&packet.InventoryContent{
			WindowID: windowID,
			Content:  make([]protocol.ItemInstance, size),
		})
	}
	send(protocol.WindowIDInventory, 36)
	send(protocol.WindowIDArmour, 4)
	send(protocol.WindowIDOffHand, 1)
	send(protocol.WindowIDUI, 54)
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

func (s *Server) sendBedrockSurvivalState(conn *minecraft.Conn, runtimeID uint64) {
	// Reassert survival gamemode.
	_ = conn.WritePacket(&packet.SetPlayerGameType{GameType: packet.GameTypeSurvival})

	// Send UpdateAbilities so the Bedrock client always uses the correct
	// survival walk/fly speeds.  Without this the client may drift into a
	// faster default speed after a gamemode/ability request cycle.
	_ = conn.WritePacket(&packet.UpdateAbilities{AbilityData: bedrockSurvivalAbilityData(runtimeID)})

	// Bedrock's actual walking speed is driven by the movement attribute. If it
	// is omitted, some clients keep a stale/non-survival value and move far too
	// fast even though the gamemode is survival.
	_ = conn.WritePacket(&packet.UpdateAttributes{
		EntityRuntimeID: runtimeID,
		Attributes:      []protocol.Attribute{bedrockMovementAttribute()},
	})
}

// sendLocalPlayerActorData initializes the local player's actor data so the
// client renders a correct HUD. Without it the air-supply component defaults to
// 0 and the client shows the drowning (air-bubble) bar on dry land. 300 ticks
// (15s) = full air, plus the breathing flag, matching dragonfly's reference.
func (s *Server) sendLocalPlayerActorData(conn *minecraft.Conn) {
	meta := protocol.NewEntityMetadata()
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagHasGravity)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagHasCollision)
	meta.SetFlag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagBreathing)
	meta[protocol.EntityDataKeyAirSupply] = int16(300)
	meta[protocol.EntityDataKeyAirSupplyMax] = int16(300)
	_ = conn.WritePacket(&packet.SetActorData{
		EntityRuntimeID: bedrockLocalRuntime,
		EntityMetadata:  meta,
	})
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
		NewBlockRuntimeID: bedrockworld.LivingWorldBlockIDToBedrockRID(blockID),
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
		s.sendBedrockSurvivalState(bs.conn, bedrockLocalRuntime)
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
		s.sendBedrockSurvivalState(conn, bedrockLocalRuntime)

	case *packet.SubChunkRequest:
		s.converter.HandleSubChunkRequest(conn, p, chunkCache)

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

		// Update sneaking state
		sneaking := p.InputData.Load(packet.InputFlagSneaking)
		s.pm.UpdateSneak(bs.id, sneaking)

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
		s.sendBedrockSurvivalState(conn, bedrockLocalRuntime)

	case *packet.PlayerAction:
		switch p.ActionType {
		case protocol.PlayerActionStartSneak:
			s.pm.UpdateSneak(bs.id, true)
		case protocol.PlayerActionStopSneak:
			s.pm.UpdateSneak(bs.id, false)
		case protocol.PlayerActionStopBreak, protocol.PlayerActionPredictDestroyBlock:
			s.breakBedrockBlock(p.BlockPosition)
		case protocol.PlayerActionStartFlying, protocol.PlayerActionStopFlying:
			// Client attempted to toggle flight through settings/controls. Re-assert survival.
			s.sendBedrockSurvivalState(conn, bedrockLocalRuntime)
		}

	case *packet.Interact:
		// Pressing the inventory key sends Interact{OpenInventory}; the server must
		// reply with ContainerOpen for the client to actually open the inventory UI.
		// Sending it twice while open crashes the client, so guard with invOpened.
		if p.ActionType == packet.InteractActionOpenInventory && !bs.invOpened {
			bs.invOpened = true
			x, y, z := int32(0), int32(bedrockGroundY), int32(0)
			if pl := s.pm.GetPlayer(bs.id); pl != nil {
				x, y, z = int32(pl.Position.X), int32(pl.Position.Y), int32(pl.Position.Z)
			}
			_ = conn.WritePacket(&packet.ContainerOpen{
				WindowID:                0,
				ContainerType:           0xff,
				ContainerEntityUniqueID: -1,
				ContainerPosition:       protocol.BlockPos{x, y, z},
			})
		}

	case *packet.ContainerClose:
		bs.invOpened = false
		// Acknowledge the close so the client allows the inventory to be reopened.
		_ = conn.WritePacket(&packet.ContainerClose{WindowID: p.WindowID, ContainerType: p.ContainerType})

	case *packet.ItemStackRequest:
		responses := make([]protocol.ItemStackResponse, 0, len(p.Requests))
		for _, req := range p.Requests {
			responses = append(responses, inventory.RejectItemStackRequest(req.RequestID))
		}
		_ = conn.WritePacket(&packet.ItemStackResponse{Responses: responses})

	case *packet.CommandRequest:
		raw := strings.TrimPrefix(p.CommandLine, "/")
		command.Default().Dispatch(bs, raw)

	case *packet.SetPlayerGameType:
		// Reject client self-gamemode changes unless the player is OP. Re-assert
		// the authoritative survival state so a non-OP client snaps back.
		if pl := s.pm.GetPlayer(bs.id); pl == nil || !pl.Op {
			s.sendBedrockSurvivalState(conn, bedrockLocalRuntime)
		}

	case *packet.SetDefaultGameType:
		// Never client-authoritative.
		s.sendBedrockSurvivalState(conn, bedrockLocalRuntime)

	case *packet.Text:
		if p.TextType == packet.TextTypeChat && p.Message != "" {
			ev := &plugin.PlayerChatEvent{
				BaseEvent:  plugin.BaseEvent{Type_: plugin.EventPlayerChat},
				PlayerName: bs.username,
				Message:    p.Message,
			}
			if !plugin.Manager().EmitCancellable(ev) {
				// Deliver to BOTH editions via the shared player manager.
				s.pm.Broadcast(fmt.Sprintf("<%s> %s", bs.username, ev.Message))
			}
		}

	case *packet.Animate:
		if p.ActionType == packet.AnimateActionSwingArm {
			s.pm.PublishSwing(bs.id)
		}

	case *packet.ResourcePackClientResponse:
		log.Printf("[Bedrock] Resource pack response: %d", p.Response)

	case *packet.PacketViolationWarning:
		log.Printf("[Bedrock] Packet violation: %v", p)
	}
}
