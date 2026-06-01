package server

import (
	"fmt"
	"log"
	"strings"
	"time"

	"livingworld/internal/bedrock/inventory"
	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/command"
	"livingworld/internal/item"
	"livingworld/internal/world"
	"livingworld/plugin"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

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

		// Dynamically load new chunks if the player crossed a chunk boundary.
		// Use floor division (world.ChunkCoord) not int32(x)>>4, which is wrong
		// for negative coords and left far -X/-Z chunks unloaded until entered.
		chunkX := world.ChunkCoord(x)
		chunkZ := world.ChunkCoord(z)
		if chunkX != bs.lastChunkX || chunkZ != bs.lastChunkZ {
			bs.setChunkCenter(chunkX, chunkZ) // under bs.mu (AOI reads it concurrently)
			s.updateBedrockChunks(bs, chunkX, chunkZ)
			// AOI: this viewer moved — re-evaluate which foreign players are now in /
			// out of range (reads the just-updated lastChunkX/Z).
			s.reconcileViewers(bs)
		}
	}
}

func (s *Server) handlePacket(bs *bedrockSession, pk packet.Packet, chunkCache *bedrockworld.ChunkCache) {
	conn := bs.conn
	switch p := pk.(type) {
	case *packet.RequestChunkRadius:
		r := int(p.ChunkRadius)
		if r <= 0 || r > s.cfg.Bedrock.ViewDistance {
			r = s.cfg.Bedrock.ViewDistance
		}
		s.bootstrapWorld(conn, r, bs)
		s.sendBedrockSurvivalState(conn, bedrockLocalRuntime)

	case *packet.SubChunkRequest:
		s.converter.HandleSubChunkRequest(conn, p, s.wm.GetDefaultWorld(), chunkCache)

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
			switch action.Action {
			case protocol.PlayerActionStartBreak:
				s.crackSwitch(bs, action.BlockPos) // clears crack on a previously-targeted block
				s.broadcastBlockCracking(action.BlockPos, packet.LevelEventStartBlockCracking)
				s.publishCrack(bs, action.BlockPos, 0) // start the overlay on Java viewers too
			case protocol.PlayerActionContinueDestroyBlock:
				// Holding break while the crosshair moves to a new block sends a
				// continue (not a fresh start); detect that switch and move the
				// crack overlay so it doesn't stick on the old block.
				if s.crackSwitch(bs, action.BlockPos) {
					s.broadcastBlockCracking(action.BlockPos, packet.LevelEventStartBlockCracking)
					s.publishCrack(bs, action.BlockPos, 0) // overlay moved to the new block on Java
				} else {
					s.broadcastBlockCracking(action.BlockPos, packet.LevelEventUpdateBlockCracking)
				}
			case protocol.PlayerActionAbortBreak:
				s.wm.CrackManager().StopBreaking(bs.id)
				s.broadcastBlockCracking(action.BlockPos, packet.LevelEventStopBlockCracking)
				s.publishCrack(bs, action.BlockPos, -1) // clear the overlay on Java too
			case protocol.PlayerActionStopBreak, protocol.PlayerActionPredictDestroyBlock, protocol.PlayerActionCreativePlayerDestroyBlock:
				s.wm.CrackManager().StopBreaking(bs.id)
				s.breakBedrockBlock(bs, action.BlockPos)
			}
		}

	case *packet.InventoryTransaction:
		if p.TransactionData != nil {
			switch data := p.TransactionData.(type) {
			case *protocol.UseItemTransactionData:
				switch data.ActionType {
				case protocol.UseItemActionClickBlock:
					// Bedrock block placement: resolve held item â†’ block state â†’ place
					if data.HeldItem.Stack.ItemType.NetworkID != 0 {
						itemName, ok := inventory.NameByRuntimeID(data.HeldItem.Stack.ItemType.NetworkID)
						if ok {
							stateID, placeable := item.BlockStateID(itemName)
							if placeable {
								targetPos := adjacentBlockPos(data.BlockPosition, data.BlockFace)
								s.wm.SetBlockAndPublish(world.BlockUpdateSourceBedrock, int(targetPos[0]), int(targetPos[1]), int(targetPos[2]), world.BlockByID(stateID))

								// Decrement held item count (survival item consumption)
								pl := s.pm.GetPlayer(bs.id)
								if pl != nil && pl.Inventory != nil && pl.Inventory.HeldSlot >= 0 && pl.Inventory.HeldSlot < len(pl.Inventory.Items) {
									if pl.Inventory.Items[pl.Inventory.HeldSlot].Count > 0 {
										pl.Inventory.Items[pl.Inventory.HeldSlot].Count--
										s.syncBedrockInventory(bs, pl)
										// Held stack shrank: re-render the hand for others.
										s.pm.PublishEquipmentChange(bs.id)
									}
								}
								return
							}
						}
					}
					// Fallback: resync jika item tidak placeable atau tidak ditemukan
					s.resyncBedrockBlock(conn, data.BlockPosition)
					s.resyncBedrockBlock(conn, adjacentBlockPos(data.BlockPosition, data.BlockFace))
				case protocol.UseItemActionBreakBlock:
					s.breakBedrockBlock(bs, data.BlockPosition)
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
		case protocol.PlayerActionStartBreak:
			s.crackSwitch(bs, p.BlockPosition)
			s.broadcastBlockCracking(p.BlockPosition, packet.LevelEventStartBlockCracking)
			s.publishCrack(bs, p.BlockPosition, 0)
		case protocol.PlayerActionAbortBreak:
			s.wm.CrackManager().StopBreaking(bs.id)
			s.broadcastBlockCracking(p.BlockPosition, packet.LevelEventStopBlockCracking)
			s.publishCrack(bs, p.BlockPosition, -1)
		case protocol.PlayerActionStopBreak, protocol.PlayerActionPredictDestroyBlock:
			s.breakBedrockBlock(bs, p.BlockPosition)
		case protocol.PlayerActionStartFlying, protocol.PlayerActionStopFlying:
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
		// Reject client self-gamemode changes unless the player is OP. The client
		// applies its picked mode locally, so re-asserting only the abilities left
		// the mode (and its creative inventory/flight UI) changed. We must echo a
		// SetPlayerGameType back to snap the client's actual game mode to survival,
		// then re-assert survival abilities. Ops change mode via /gamemode instead.
		if pl := s.pm.GetPlayer(bs.id); pl == nil || !pl.Op {
			_ = conn.WritePacket(&packet.SetPlayerGameType{GameType: packet.GameTypeSurvival})
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

	case *packet.MobEquipment:
		// Player switched their held hotbar slot; publish so other clients
		// (both editions) re-render this player's hand item.
		s.pm.UpdateHeldSlot(bs.id, int(p.HotBarSlot))

	case *packet.ResourcePackClientResponse:
		log.Printf("[Bedrock] Resource pack response: %d", p.Response)

	case *packet.PacketViolationWarning:
		log.Printf("[Bedrock] Packet violation: %v", p)
	}
}
