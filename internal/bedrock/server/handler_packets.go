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
				breakSecs := bedrockCrackSecondsFor(s.wm.GetDefaultWorld().GetBlock(int(action.BlockPos[0]), int(action.BlockPos[1]), int(action.BlockPos[2])).ID())
				s.crackSwitch(bs, action.BlockPos, breakSecs) // clears crack on a previously-targeted block
				s.broadcastBlockCracking(action.BlockPos, packet.LevelEventStartBlockCracking, breakSecs)
				s.publishCrack(bs, action.BlockPos, 0) // start the overlay on Java viewers too
			case protocol.PlayerActionContinueDestroyBlock:
				// Holding break while the crosshair moves to a new block sends a
				// continue (not a fresh start); detect that switch and move the
				// crack overlay so it doesn't stick on the old block.
				if s.crackSwitch(bs, action.BlockPos, 0) {
					breakSecs := bedrockCrackSecondsFor(s.wm.GetDefaultWorld().GetBlock(int(action.BlockPos[0]), int(action.BlockPos[1]), int(action.BlockPos[2])).ID())
					s.broadcastBlockCracking(action.BlockPos, packet.LevelEventStartBlockCracking, breakSecs)
					s.publishCrack(bs, action.BlockPos, 0) // overlay moved to the new block on Java
				} else {
					s.broadcastBlockCracking(action.BlockPos, packet.LevelEventUpdateBlockCracking, 0)
					// Progressive overlay update for Java viewers: Bedrock self-animates
					// from LevelEventStartBlockCracking, but Java needs explicit stage
					// transitions or the overlay freezes at stage 0. Pass 0 here so
					// AdvanceStage reads the per-block duration captured at break-start.
					if stage, changed := s.wm.CrackManager().AdvanceStage(bs.id, 0); changed {
						s.publishCrack(bs, action.BlockPos, stage)
					}
				}
			case protocol.PlayerActionAbortBreak:
				s.wm.CrackManager().StopBreaking(bs.id)
				s.broadcastBlockCracking(action.BlockPos, packet.LevelEventStopBlockCracking, 0)
				s.publishCrack(bs, action.BlockPos, -1) // clear the overlay on Java too
			case protocol.PlayerActionStopBreak, protocol.PlayerActionPredictDestroyBlock, protocol.PlayerActionCreativePlayerDestroyBlock:
				s.wm.CrackManager().StopBreaking(bs.id)
				s.breakBedrockBlock(bs, action.BlockPos)
			}
		}

	case *packet.InventoryTransaction:
		if p.TransactionData != nil {
			switch data := p.TransactionData.(type) {
			case *protocol.NormalTransactionData, *protocol.MismatchTransactionData:
				// Q / Ctrl+Q drop: the client sends an InventoryTransaction where one
				// action has SourceType=World (2), the hotbar slot, OldItem=the held
				// stack, and NewItem=the held stack minus the dropped count. We do
				// not try to "balance" the transaction (that would require tracking
				// the full book-keeping of all client slots); we just spawn the drop
				// entity and shrink the held slot authoritatively.
				for _, act := range p.Actions {
					if act.SourceType != protocol.InventoryActionSourceWorld {
						continue
					}
					dropped := int(act.OldItem.Stack.Count) - int(act.NewItem.Stack.Count)
					if dropped <= 0 {
						continue
					}
					s.handleBedrockPlayerDrop(bs, act.InventorySlot, dropped)
				}
			case *protocol.UseItemTransactionData:
				switch data.ActionType {
				case protocol.UseItemActionClickBlock:
					// Bedrock block placement: resolve held item → block state → place
					if data.HeldItem.Stack.ItemType.NetworkID != 0 {
						itemName, ok := inventory.NameByRuntimeID(data.HeldItem.Stack.ItemType.NetworkID)
						if ok {
							stateID, placeable := item.BlockStateID(itemName)
							if placeable {
								// Vanilla placement rules for Bedrock (mirrors the Java
								// path): only consume the held stack if the target is air
								// (or a replaceable block). Bedrock's UseItemActionClickBlock
								// covers both PLACE and USE; in vanilla, the crouch gate
								// (sneak-to-place on top of a solid) exists so a right-click
								// on a chest/door/furnace opens the UI instead of placing
								// a block on it. LivingWorld has no block-USE semantics
								// implemented yet, so the crouch gate would just stop
								// players from placing dirt/grass on the ground surface —
								// a frustrating dead end. Drop the crouch check and let
								// the player place on top of anything; the future
								// interaction service (Phase 4d) can add the gate per
								// block without touching this call site.
								targetPos := adjacentBlockPos(data.BlockPosition, data.BlockFace)
								wm := s.wm.GetDefaultWorld()
								pl := s.pm.GetPlayer(bs.id)
								if pl == nil {
									return
								}
								targetID := wm.GetBlock(int(targetPos[0]), int(targetPos[1]), int(targetPos[2])).ID()
								if targetID != world.AirID {
									// Block at the target position: refuse the place and
									// re-affirm the world so the client's prediction rolls
									// back cleanly.
									s.resyncBedrockBlock(conn, data.BlockPosition)
									s.resyncBedrockBlock(conn, protocol.BlockPos{int32(targetPos[0]), int32(targetPos[1]), int32(targetPos[2])})
									return
								}

								s.wm.SetBlockAndPublish(world.BlockUpdateSourceBedrock, int(targetPos[0]), int(targetPos[1]), int(targetPos[2]), world.BlockByID(int32(stateID)))

								// Decrement held item count (survival item consumption) —
								// only on a fully-valid placement, so the server's
								// authoritative state matches the client's predicted
								// decrement.
								if pl.Inventory != nil && pl.Inventory.HeldSlot >= 0 && pl.Inventory.HeldSlot < len(pl.Inventory.Items) {
									if pl.Inventory.Items[pl.Inventory.HeldSlot].Count > 0 {
										pl.Inventory.Items[pl.Inventory.HeldSlot].Count--
										s.syncBedrockInventory(bs, pl)
										// Held stack shrank: re-render the hand for others.
										s.pm.PublishEquipmentChange(bs.id)
									}
								}
								// Echo the placed block back to the placer inline so the
								// new block appears in the same network round-trip as
								// the placement confirm — no waiting for the block-event
								// bus goroutine to flush. The bus will still fan the
								// change out to foreign viewers, so this is purely a
								// latency win for the placer.
								//
								// Flags is intentionally just BlockUpdateNetwork. The
								// gophertunnel doc says "typically sending only the
								// BlockUpdateNetwork flag is sufficient" — adding
								// BlockUpdateNeighbours tells the client this is a
								// neighbour/light update (redstone-style), which in
								// some client builds suppresses the player-side place
								// sound and arm animation context, leaving the block
								// "placed silently".
								conn.WritePacket(&packet.UpdateBlock{
									Position:          protocol.BlockPos{int32(targetPos[0]), int32(targetPos[1]), int32(targetPos[2])},
									NewBlockRuntimeID: bedrockworld.LivingWorldBlockIDToBedrockRID(int32(stateID)),
									Flags:             packet.BlockUpdateNetwork,
									Layer:             0,
								})
								// Guaranteed place-sound. Bedrock has no generic
								// `LevelEvent` for "block placed", and the client-side
								// place sound is tied to the placement *prediction*,
								// not the server's UpdateBlock — if the client's
								// prediction is suppressed (e.g. late-joining the
								// session, the client got the UseItem transaction
								// out of order, etc.) the place happens silently. The
								// vanilla place SFX is `step.<material>` for most
								// blocks; we use a category-keyed lookup so grass/dirt
								// gets `step.grass`, stone-family gets `step.stone`,
								// wood gets `step.wood`, etc., and a generic
								// `step.stone` as the catch-all. Pitch 0.79/Volume 1
								// match vanilla's block-place call.
								s.playBlockPlaceSound(conn, itemName, protocol.BlockPos{int32(targetPos[0]), int32(targetPos[1]), int32(targetPos[2])})
								// Play the placer's own arm-swing animation. Bedrock's
								// client only auto-swings for PlayerActionStartBreak; a
								// placement arriving through UseItemActionClickBlock does
								// NOT trigger a local swing, so without this the placer
								// sees the new block appear with no arm follow-through
								// (the "no animation" reported in single-player testing).
								//
								// The EntityRuntimeID MUST be `bedrockLocalRuntime` (1),
								// not `pl.EntityRuntimeID`. The local Bedrock client
								// identifies its own player as runtime id 1 — the
								// per-session id (bs.runtimeID) is what foreign viewers
								// see, but addressing the local client with the
								// per-session id silently no-ops the packet. Same
								// gotcha as Push() (see session.go).
								//
								// SwingSource=Build is the second half: vanilla uses
								// that hint to pick the swing timing and the "place"
								// SFX; SwingSource=0 (none) makes many client versions
								// drop the packet.
								conn.WritePacket(&packet.Animate{
									ActionType:      packet.AnimateActionSwingArm,
									EntityRuntimeID: bedrockLocalRuntime,
									SwingSource:     packet.AnimateSwingSourceBuild,
								})
								// Replay the placer's arm swing on every other viewer so the
								// place animation shows up for foreign viewers too.
								s.pm.PublishSwing(bs.id)
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
			case *protocol.UseItemOnEntityTransactionData:
				// M5: player attacks on Bedrock arrive as
				// InventoryTransaction{UseItemOnEntityTransactionData}.
				// ActionType: 0=Interact, 1=Attack. We only act on
				// Attack; Interact is a no-op for now (M0 didn't
				// implement interact-on-entity either).
				if data.ActionType == protocol.UseItemOnEntityActionAttack {
					s.routeBedrockAttack(bs.id, int64(data.TargetEntityRuntimeID))
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
			breakSecs := bedrockCrackSecondsFor(s.wm.GetDefaultWorld().GetBlock(int(p.BlockPosition[0]), int(p.BlockPosition[1]), int(p.BlockPosition[2])).ID())
			s.crackSwitch(bs, p.BlockPosition, breakSecs)
			s.broadcastBlockCracking(p.BlockPosition, packet.LevelEventStartBlockCracking, breakSecs)
			s.publishCrack(bs, p.BlockPosition, 0)
		case protocol.PlayerActionAbortBreak:
			s.wm.CrackManager().StopBreaking(bs.id)
			s.broadcastBlockCracking(p.BlockPosition, packet.LevelEventStopBlockCracking, 0)
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
