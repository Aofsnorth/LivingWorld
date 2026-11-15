package server

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"livingworld/internal/bedrock/inventory"
	bedrockworld "livingworld/internal/bedrock/world"
	"livingworld/internal/command"
	"livingworld/internal/item"
	"livingworld/internal/player"
	"livingworld/internal/skinbridge"
	"livingworld/internal/world"
	"livingworld/plugin"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func isBedrockFallResetBlock(name string) bool {
	switch name {
	case "minecraft:water", "minecraft:lava", "minecraft:bubble_column",
		"minecraft:cobweb", "minecraft:powder_snow", "minecraft:sweet_berry_bush",
		"minecraft:ladder", "minecraft:vine", "minecraft:scaffolding",
		"minecraft:weeping_vines", "minecraft:weeping_vines_plant",
		"minecraft:twisting_vines", "minecraft:twisting_vines_plant",
		"minecraft:cave_vines", "minecraft:cave_vines_plant",
		"minecraft:slime_block", "minecraft:honey_block":
		return true
	}
	return false
}

func (s *Server) isSolidGroundAt(x, feetY, z float64) bool {
	if s.wm == nil {
		return false
	}
	dw := s.wm.GetDefaultWorld()
	if dw == nil {
		return false
	}
	bx := int(math.Floor(x))
	by := int(math.Floor(feetY - 0.06))
	bz := int(math.Floor(z))
	blockID := dw.GetBlock(bx, by, bz).ID()
	if blockID == 0 || world.IsReplaceableBlock(blockID) || world.IsWaterBlockID(blockID) || world.IsLavaBlockID(blockID) {
		return false
	}
	name := world.StateName(blockID)
	return name != "minecraft:air" && !isBedrockFallResetBlock(name)
}

func (s *Server) trackBedrockFall(bs *bedrockSession, x, oldY, newY, z float64, onGround bool) {
	if bs == nil || s.pm == nil {
		return
	}
	pl := s.pm.GetPlayer(bs.id)
	if pl == nil || pl.Creative || pl.Gamemode == 1 || pl.Gamemode == 3 || pl.Flying || pl.Health <= 0 {
		bs.mu.Lock()
		bs.fallDistance = 0
		bs.mu.Unlock()
		return
	}

	if s.wm != nil {
		if dw := s.wm.GetDefaultWorld(); dw != nil {
			bx := int(math.Floor(x))
			by := int(math.Floor(newY))
			bz := int(math.Floor(z))
			feetBlock := dw.GetBlock(bx, by, bz).ID()
			if world.IsWaterBlockID(feetBlock) || world.IsLavaBlockID(feetBlock) || isBedrockFallResetBlock(world.StateName(feetBlock)) {
				bs.mu.Lock()
				bs.fallDistance = 0
				bs.mu.Unlock()
				return
			}
			belowBlock := dw.GetBlock(bx, int(math.Floor(newY-0.2)), bz).ID()
			if world.IsWaterBlockID(belowBlock) || world.IsLavaBlockID(belowBlock) || isBedrockFallResetBlock(world.StateName(belowBlock)) {
				bs.mu.Lock()
				bs.fallDistance = 0
				bs.mu.Unlock()
				return
			}
		}
	}

	bs.mu.Lock()
	if newY < oldY {
		bs.fallDistance += oldY - newY
	} else if newY > oldY+0.02 {
		bs.fallDistance = 0
	}
	curFall := bs.fallDistance
	if !onGround {
		bs.mu.Unlock()
		s.pm.SyncFallState(bs.id, false, curFall)
		return
	}
	dist := bs.fallDistance
	bs.fallDistance = 0
	bs.mu.Unlock()
	s.pm.SyncFallState(bs.id, true, 0)
	s.pm.SyncFallState(bs.id, true, 0)

	const fallSafeBlocks = 3.35
	if dist <= fallSafeBlocks {
		return
	}
	dmg := int(math.Ceil(dist - fallSafeBlocks))
	if dmg <= 0 {
		return
	}
	bs.Hurt(float32(dmg))
}

func (s *Server) publishBedrockMove(bs *bedrockSession, clientPos mgl32.Vec3, pitch, yaw float32, onGround bool) {
	now := time.Now()
	x, y, z := bedrockSharedFeetFromLocalClient(clientPos)
	bs.mu.Lock()
	oldY := bs.lastY
	hasPrev := !bs.lastMovePublish.IsZero()
	bs.mu.Unlock()
	publish, correct := bs.movementUpdate(now, x, y, z)
	if correct {
		// Do not hard-freeze the local Bedrock player for now; just ignore the
		// impossible sample and reassert survival abilities. Hard correction here
		// previously made movement feel broken/floaty.
		s.sendBedrockSurvivalState(bs.conn, bedrockLocalRuntime)
		return
	}
	if hasPrev {
		s.trackBedrockFall(bs, x, oldY, y, z, onGround)
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
			// out of range (reads the just-updated lastChunkX/Z), and do the
			// same for stationary item drops that won't emit OnMove anymore.
			s.reconcileViewers(bs)
			s.reconcileDrops(bs)
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
		if pl := s.pm.GetPlayer(bs.id); pl != nil && pl.Health <= 0 {
			return
		}
		// If PlayerAuthInput is active, ignore legacy MovePlayer packets so we
		// don't process two movement streams for the same player in the same tick.
		// Double-processing was one source of ultra-fast movement.
		if time.Since(bs.lastAuthInputAt) > 100*time.Millisecond {
			s.publishBedrockMove(bs, p.Position, p.Pitch, p.Yaw, p.OnGround)
		}

	case *packet.PlayerAuthInput:
		if pl := s.pm.GetPlayer(bs.id); pl != nil && pl.Health <= 0 {
			return
		}
		// Modern Bedrock clients commonly use PlayerAuthInput for movement. Use
		// it as the primary movement source and ignore duplicate MovePlayer bursts.
		bs.lastAuthInputAt = time.Now()
		fx, fy, fz := bedrockSharedFeetFromLocalClient(p.Position)
		onGround := p.Delta[1] <= 0.02 && (p.InputData.Load(packet.InputFlagVerticalCollision) || (math.Abs(float64(p.Delta[1])) < 0.001 && s.isSolidGroundAt(fx, fy, fz)))
		s.publishBedrockMove(bs, p.Position, p.Pitch, p.HeadYaw, onGround)
		if !onGround && p.Delta[1] < -0.005 {
			s.pm.SyncFallState(bs.id, false, float64(-p.Delta[1]))
		}
		if !onGround && p.Delta[1] < -0.005 {
			s.pm.SyncFallState(bs.id, false, float64(-p.Delta[1]))
		}

		// Update sneaking, sprinting, jumping, and swimming state
		sneaking := p.InputData.Load(packet.InputFlagSneaking)
		s.pm.UpdateSneak(bs.id, sneaking)
		if p.InputData.Load(packet.InputFlagStopSprinting) {
			s.pm.UpdateSprinting(bs.id, false)
		} else if p.InputData.Load(packet.InputFlagStartSprinting) || p.InputData.Load(packet.InputFlagSprinting) {
			s.pm.UpdateSprinting(bs.id, true)
		} else {
			s.pm.UpdateSprinting(bs.id, false)
		}
		if p.InputData.Load(packet.InputFlagStartJumping) {
			s.pm.AddJumpExhaustion(bs.id)
		}
		if p.InputData.Load(packet.InputFlagStartSwimming) {
			s.pm.UpdateSwimming(bs.id, true)
		} else if p.InputData.Load(packet.InputFlagStopSwimming) {
			s.pm.UpdateSwimming(bs.id, false)
		}

		if p.InputData.Load(packet.InputFlagMissedSwing) {
			s.pm.PublishSwing(bs.id)
		}

		actions, _ := p.BlockActions.Value()
		for _, action := range actions {
			switch action.Action {
			case protocol.PlayerActionStartBreak:
				s.pm.PublishSwing(bs.id)
				blockID := s.wm.GetDefaultWorld().GetBlock(int(action.BlockPos[0]), int(action.BlockPos[1]), int(action.BlockPos[2])).ID()
				isCreative := false
				if pl := s.pm.GetPlayer(bs.id); pl != nil {
					isCreative = pl.Gamemode == 1 || pl.Creative
				}
				if blockID != 0 && (isCreative || world.Hardness(blockID) == 0) {
					s.wm.CrackManager().StopBreaking(bs.id)
					s.breakBedrockBlock(bs, action.BlockPos)
					continue
				}
				breakSecs := s.bedrockCrackSecondsForPlayer(bs, blockID)
				s.crackSwitch(bs, action.BlockPos, breakSecs) // clears crack on a previously-targeted block
				s.broadcastBlockCracking(action.BlockPos, packet.LevelEventStartBlockCracking, breakSecs)
				s.publishCrack(bs, action.BlockPos, 0) // start the overlay on Java viewers too
			case protocol.PlayerActionContinueDestroyBlock:
				s.pm.PublishSwing(bs.id)
				// Holding break while the crosshair moves to a new block sends a
				// continue (not a fresh start); detect that switch and move the
				// crack overlay so it doesn't stick on the old block.
				curBlockID := s.wm.GetDefaultWorld().GetBlock(int(action.BlockPos[0]), int(action.BlockPos[1]), int(action.BlockPos[2])).ID()
				breakSecs := s.bedrockCrackSecondsForPlayer(bs, curBlockID)
				if s.crackSwitch(bs, action.BlockPos, breakSecs) {
					s.broadcastBlockCracking(action.BlockPos, packet.LevelEventStartBlockCracking, breakSecs)
					s.publishCrack(bs, action.BlockPos, 0) // overlay moved to the new block on Java
				} else {
					s.broadcastBlockCracking(action.BlockPos, packet.LevelEventUpdateBlockCracking, breakSecs)
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
				s.resyncBedrockBlock(conn, action.BlockPos)
			case protocol.PlayerActionPredictDestroyBlock, protocol.PlayerActionCreativePlayerDestroyBlock:
				s.pm.PublishSwing(bs.id)
				s.wm.CrackManager().StopBreaking(bs.id)
				s.breakBedrockBlock(bs, action.BlockPos)
			case protocol.PlayerActionStopBreak:
				s.pm.PublishSwing(bs.id)
				if st := s.wm.CrackManager().GetBreaking(bs.id); st != nil {
					s.wm.CrackManager().StopBreaking(bs.id)
					targetPos := action.BlockPos
					if targetPos == (protocol.BlockPos{}) {
						targetPos = protocol.BlockPos{int32(st.BlockPos.X), int32(st.BlockPos.Y), int32(st.BlockPos.Z)}
					}
					s.breakBedrockBlock(bs, targetPos)
				}
			case protocol.PlayerActionDropItem:
				if pl := s.pm.GetPlayer(bs.id); pl != nil && pl.Inventory != nil {
					s.handleBedrockPlayerDrop(bs, uint32(pl.Inventory.HeldSlot), 1)
				}
			case protocol.PlayerActionMissedSwing:
				s.pm.PublishSwing(bs.id)
			}
		}

		if p.InputData.Load(packet.InputFlagPerformItemInteraction) {
			if data, ok := p.ItemInteractionData.Value(); ok {
				switch data.ActionType {
				case protocol.UseItemActionBreakBlock:
					s.pm.PublishSwing(bs.id)
					s.wm.CrackManager().StopBreaking(bs.id)
					s.breakBedrockBlock(bs, data.BlockPosition)
				case protocol.UseItemActionClickAir, protocol.UseItemActionClickBlock:
					s.tryEatBedrockFood(bs)
				}
			}
		}
		if p.InputData.Load(packet.InputFlagPerformItemStackRequest) {
			if req, ok := p.ItemStackRequest.Value(); ok {
				s.handleBedrockItemStackRequest(bs, &packet.ItemStackRequest{
					Requests: []protocol.ItemStackRequest{req},
				})
			}
		}

	case *packet.InventoryTransaction:
		if p.TransactionData != nil {
			switch data := p.TransactionData.(type) {
			case *protocol.NormalTransactionData, *protocol.MismatchTransactionData:
				s.handleBedrockTransactionActions(bs, p.Actions)
			case *protocol.UseItemTransactionData:
				switch data.ActionType {
				case protocol.UseItemActionClickBlock:
					// Bedrock block placement is server-authoritative: resolve the held
					// item from the server inventory, not from data.HeldItem. Some Bedrock
					// clients send duplicate UseItemActionClickBlock packets for one tap,
					// and the duplicate can carry the pre-decrement stack. Trusting that
					// packet let a 1-block stack place twice.
					pl := s.pm.GetPlayer(bs.id)
					if pl == nil || pl.Inventory == nil {
						return
					}
					if !pl.Sneaking {
						if s.wm.InteractBlock(world.BlockUpdateSourceBedrock, int(data.BlockPosition[0]), int(data.BlockPosition[1]), int(data.BlockPosition[2])) {
							return
						}
					}
					heldSlot := pl.Inventory.HeldSlot
					held := pl.Inventory.GetHeldItem()
					if held == nil || held.ID == 0 || held.Count <= 0 {
						s.syncBedrockInventory(bs, pl)
						s.resyncBedrockBlock(conn, data.BlockPosition)
						s.resyncBedrockBlock(conn, adjacentBlockPos(data.BlockPosition, data.BlockFace))
						return
					}
					heldItem, ok := item.ByID(held.ID)
					if !ok {
						s.syncBedrockInventory(bs, pl)
						s.resyncBedrockBlock(conn, data.BlockPosition)
						s.resyncBedrockBlock(conn, adjacentBlockPos(data.BlockPosition, data.BlockFace))
						return
					}
					if clientRID := data.HeldItem.Stack.ItemType.NetworkID; clientRID != 0 {
						serverRID, ok := inventory.RuntimeIDByName(heldItem.Name)
						if !ok || serverRID != clientRID {
							s.syncBedrockInventory(bs, pl)
							s.resyncBedrockBlock(conn, data.BlockPosition)
							s.resyncBedrockBlock(conn, adjacentBlockPos(data.BlockPosition, data.BlockFace))
							return
						}
					}
					stateID, placeable := item.BlockStateID(heldItem.Name)
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
						clickedID := wm.GetBlock(int(data.BlockPosition[0]), int(data.BlockPosition[1]), int(data.BlockPosition[2])).ID()
						if world.IsReplaceableBlock(clickedID) {
							targetPos = data.BlockPosition
						}
						targetID := wm.GetBlock(int(targetPos[0]), int(targetPos[1]), int(targetPos[2])).ID()
						if !world.IsReplaceableBlock(targetID) {
							// Block at the target position: refuse the place and
							// re-affirm the world so the client's prediction rolls
							// back cleanly.
							s.resyncBedrockBlock(conn, data.BlockPosition)
							s.resyncBedrockBlock(conn, protocol.BlockPos{int32(targetPos[0]), int32(targetPos[1]), int32(targetPos[2])})
							return
						}
						if !bs.acceptBlockPlace() {
							s.syncBedrockInventory(bs, pl)
							s.resyncBedrockBlock(conn, data.BlockPosition)
							s.resyncBedrockBlock(conn, protocol.BlockPos{int32(targetPos[0]), int32(targetPos[1]), int32(targetPos[2])})
							return
						}
						if !pl.Inventory.RemoveItem(heldSlot, 1) {
							s.syncBedrockInventory(bs, pl)
							s.resyncBedrockBlock(conn, data.BlockPosition)
							s.resyncBedrockBlock(conn, protocol.BlockPos{int32(targetPos[0]), int32(targetPos[1]), int32(targetPos[2])})
							return
						}
						s.wm.SetBlockAndPublish(world.BlockUpdateSourceBedrock, int(targetPos[0]), int(targetPos[1]), int(targetPos[2]), world.BlockByID(int32(stateID)))
						s.wm.OnBlockChanged(world.BlockUpdateSourceBedrock, int(targetPos[0]), int(targetPos[1]), int(targetPos[2]))
						s.syncBedrockInventory(bs, pl)
						// Held stack shrank: re-render the hand for others.
						s.pm.PublishEquipmentChange(bs.id)
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
							NewBlockRuntimeID: mustBedrockRID(int32(stateID)),
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
						s.playBlockPlaceSound(conn, heldItem.Name, protocol.BlockPos{int32(targetPos[0]), int32(targetPos[1]), int32(targetPos[2])})
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
					// Fallback: try eating if holding food, then resync if not placeable
					_ = s.tryEatBedrockFood(bs)
					s.resyncBedrockBlock(conn, data.BlockPosition)
					s.resyncBedrockBlock(conn, adjacentBlockPos(data.BlockPosition, data.BlockFace))
				case protocol.UseItemActionBreakBlock:
					s.pm.PublishSwing(bs.id)
					s.breakBedrockBlock(bs, data.BlockPosition)
				case protocol.UseItemActionClickAir:
					s.pm.PublishSwing(bs.id)
					s.tryEatBedrockFood(bs)
				}
			case *protocol.UseItemOnEntityTransactionData:
				// M5: player attacks on Bedrock arrive as
				// InventoryTransaction{UseItemOnEntityTransactionData}.
				// ActionType: 0=Interact, 1=Attack. We only act on
				// Attack; Interact is a no-op for now (M0 didn't
				// implement interact-on-entity either).
				if data.ActionType == protocol.UseItemOnEntityActionAttack {
					s.pm.PublishSwing(bs.id)
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
		case protocol.PlayerActionRespawn:
			s.handleBedrockRespawn(bs, conn)
		case protocol.PlayerActionJump:
			s.pm.AddJumpExhaustion(bs.id)
		case protocol.PlayerActionStartSneak:
			s.pm.UpdateSneak(bs.id, true)
		case protocol.PlayerActionStopSneak:
			s.pm.UpdateSneak(bs.id, false)
		case protocol.PlayerActionStartSprint:
			s.pm.UpdateSprinting(bs.id, true)
		case protocol.PlayerActionStopSprint:
			s.pm.UpdateSprinting(bs.id, false)
		case protocol.PlayerActionStartSwimming:
			s.pm.UpdateSwimming(bs.id, true)
		case protocol.PlayerActionStopSwimming:
			s.pm.UpdateSwimming(bs.id, false)
		case protocol.PlayerActionStartBreak:
			s.pm.PublishSwing(bs.id)
			blockID := s.wm.GetDefaultWorld().GetBlock(int(p.BlockPosition[0]), int(p.BlockPosition[1]), int(p.BlockPosition[2])).ID()
			isCreative := false
			if pl := s.pm.GetPlayer(bs.id); pl != nil {
				isCreative = pl.Gamemode == 1 || pl.Creative
			}
			if blockID != 0 && (isCreative || world.Hardness(blockID) == 0) {
				s.wm.CrackManager().StopBreaking(bs.id)
				s.breakBedrockBlock(bs, p.BlockPosition)
				break
			}
			breakSecs := s.bedrockCrackSecondsForPlayer(bs, blockID)
			s.crackSwitch(bs, p.BlockPosition, breakSecs)
			s.broadcastBlockCracking(p.BlockPosition, packet.LevelEventStartBlockCracking, breakSecs)
			s.publishCrack(bs, p.BlockPosition, 0)
		case protocol.PlayerActionContinueDestroyBlock:
			s.pm.PublishSwing(bs.id)
			curBlockID := s.wm.GetDefaultWorld().GetBlock(int(p.BlockPosition[0]), int(p.BlockPosition[1]), int(p.BlockPosition[2])).ID()
			breakSecs := s.bedrockCrackSecondsForPlayer(bs, curBlockID)
			if s.crackSwitch(bs, p.BlockPosition, breakSecs) {
				s.broadcastBlockCracking(p.BlockPosition, packet.LevelEventStartBlockCracking, breakSecs)
				s.publishCrack(bs, p.BlockPosition, 0)
			} else {
				s.broadcastBlockCracking(p.BlockPosition, packet.LevelEventUpdateBlockCracking, breakSecs)
				if stage, changed := s.wm.CrackManager().AdvanceStage(bs.id, 0); changed {
					s.publishCrack(bs, p.BlockPosition, stage)
				}
			}
		case protocol.PlayerActionAbortBreak:
			s.wm.CrackManager().StopBreaking(bs.id)
			s.broadcastBlockCracking(p.BlockPosition, packet.LevelEventStopBlockCracking, 0)
			s.publishCrack(bs, p.BlockPosition, -1)
			s.resyncBedrockBlock(conn, p.BlockPosition)
		case protocol.PlayerActionPredictDestroyBlock, protocol.PlayerActionCreativePlayerDestroyBlock:
			s.pm.PublishSwing(bs.id)
			s.wm.CrackManager().StopBreaking(bs.id)
			s.breakBedrockBlock(bs, p.BlockPosition)
		case protocol.PlayerActionStopBreak:
			s.pm.PublishSwing(bs.id)
			if st := s.wm.CrackManager().GetBreaking(bs.id); st != nil {
				s.wm.CrackManager().StopBreaking(bs.id)
				targetPos := p.BlockPosition
				if targetPos == (protocol.BlockPos{}) {
					targetPos = protocol.BlockPos{int32(st.BlockPos.X), int32(st.BlockPos.Y), int32(st.BlockPos.Z)}
				}
				s.breakBedrockBlock(bs, targetPos)
			}
		case protocol.PlayerActionDropItem:
			if pl := s.pm.GetPlayer(bs.id); pl != nil && pl.Inventory != nil {
				s.handleBedrockPlayerDrop(bs, uint32(pl.Inventory.HeldSlot), 1)
			}
		case protocol.PlayerActionMissedSwing:
			s.pm.PublishSwing(bs.id)
		case protocol.PlayerActionStartFlying, protocol.PlayerActionStopFlying:
			s.sendBedrockSurvivalState(conn, bedrockLocalRuntime)
		}

	case *packet.Animate:
		if p.ActionType == packet.AnimateActionSwingArm {
			s.pm.PublishSwing(bs.id)
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
		bs.mu.Lock()
		bs.cursorSlot = -1
		bs.mu.Unlock()
		// Acknowledge the close so the client allows the inventory to be reopened.
		_ = conn.WritePacket(&packet.ContainerClose{WindowID: p.WindowID, ContainerType: p.ContainerType})
		if pl := s.pm.GetPlayer(bs.id); pl != nil {
			s.syncBedrockInventory(bs, pl)
		}

	case *packet.ItemStackRequest:
		s.handleBedrockItemStackRequest(bs, p)

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

	case *packet.MobEquipment:
		// Player switched their held hotbar slot; publish so other clients
		// (both editions) re-render this player's hand item.
		s.pm.UpdateHeldSlot(bs.id, int(p.HotBarSlot))

	case *packet.PlayerSkin:
		// Player changed their skin in-game via Bedrock dressing room.
		sk := p.Skin
		w, h := int(sk.SkinImageWidth), int(sk.SkinImageHeight)
		if w > 0 && h > 0 && len(sk.SkinData) >= w*h*4 && s.skins != nil {
			model := "wide"
			if sk.ArmSize == protocol.ArmSizeSlim {
				model = "slim"
			}
			skinURL, normRGBA, pngData := s.skins.RegisterSkin(bs.id, w, h, sk.SkinData, bs.clientData.SkinColour)
			var skinData *player.SkinData
			if len(normRGBA) == skinbridge.JavaSkinRGBABytes {
				skinData = player.NewSkinData(skinURL, model, normRGBA)
			} else {
				skinData = player.NewSkinData(skinURL, model, sk.SkinData)
			}
			s.pm.UpdateBedrockSkin(bs.id, skinURL, skinData)
			if len(pngData) > 0 {
				go s.uploadBedrockSkinPNGToMineSkin(bs.id, bs.username, pngData, model)
			}
		}

	case *packet.Respawn:
		switch p.State {
		case packet.RespawnStateSearchingForSpawn, packet.RespawnStateClientReadyToSpawn:
			s.handleBedrockRespawn(bs, conn)
		}

	case *packet.ResourcePackClientResponse:
		log.Printf("[Bedrock] Resource pack response: %d", p.Response)

	case *packet.PacketViolationWarning:
		log.Printf("[Bedrock] Packet violation: %v", p)
	}
}

func (s *Server) safeSpawnFeet() (float64, float64, float64) {
	spawn := s.cfg.World.Spawn
	feetX, feetY, feetZ := spawn.X, spawn.Y, spawn.Z
	if dw := s.wm.GetDefaultWorld(); dw != nil {
		feetX, feetY, feetZ = safeRestoredFeet(dw, feetX, feetY, feetZ)
	}
	return feetX, feetY, feetZ
}

func (s *Server) handleBedrockRespawn(bs *bedrockSession, conn *minecraft.Conn) {
	feetX, feetY, feetZ := s.safeSpawnFeet()
	spawn := s.cfg.World.Spawn
	spawnClientPos := bedrockLocalClientPosFromFeet(feetX, feetY, feetZ)

	bs.mu.Lock()
	bs.health = 20
	bs.fallDistance = 0
	bs.lastX = feetX
	bs.lastY = feetY
	bs.lastZ = feetZ
	bs.lastMovePublish = time.Now()
	bs.spawnPos = spawnClientPos
	bs.mu.Unlock()

	_ = s.pm.RespawnPlayer(bs.id, feetX, feetY, feetZ, spawn.Pitch, spawn.Yaw)
	if s.wm != nil && s.wm.Hunger() != nil {
		s.wm.Hunger().ResetPlayer(bs.id, s.cfg.World.Difficulty)
	}

	s.updateBedrockChunks(bs, int32(math.Floor(feetX))>>4, int32(math.Floor(feetZ))>>4)

	bs.write(&packet.Respawn{
		Position:        spawnClientPos,
		State:           packet.RespawnStateReadyToSpawn,
		EntityRuntimeID: bedrockLocalRuntime,
	})
	bs.write(&packet.MovePlayer{
		EntityRuntimeID: bedrockLocalRuntime,
		Position:        spawnClientPos,
		Pitch:           spawn.Pitch,
		Yaw:             spawn.Yaw,
		HeadYaw:         spawn.Yaw,
		Mode:            packet.MoveModeReset,
		OnGround:        true,
	})
	bs.write(&packet.PlayStatus{Status: packet.PlayStatusPlayerSpawn})
	bs.write(&packet.UpdateAttributes{
		EntityRuntimeID: bedrockLocalRuntime,
		Attributes: []protocol.Attribute{
			{
				AttributeValue: protocol.AttributeValue{
					Name:  "minecraft:health",
					Value: 20,
					Min:   0,
					Max:   20,
				},
				Default:    20,
				DefaultMin: 0,
				DefaultMax: 20,
			},
			{
				AttributeValue: protocol.AttributeValue{
					Name:  "minecraft:player.hunger",
					Value: 20,
					Min:   0,
					Max:   20,
				},
				Default:    20,
				DefaultMin: 0,
				DefaultMax: 20,
			},
			{
				AttributeValue: protocol.AttributeValue{
					Name:  "minecraft:movement",
					Value: 0.10,
					Min:   0,
					Max:   math.MaxFloat32,
				},
				Default:    0.10,
				DefaultMin: 0,
				DefaultMax: math.MaxFloat32,
			},
		},
	})

	mode := s.cfg.DefaultGamemode
	if pl := s.pm.GetPlayer(bs.id); pl != nil {
		mode = pl.Gamemode
		s.syncBedrockInventory(bs, pl)
	}
	s.sendBedrockGameMode(conn, bedrockLocalRuntime, mode)
	s.sendLocalPlayerActorData(conn)
	s.reconcileViewers(bs)

	s.forEachSession(func(v *bedrockSession) {
		if v.id == bs.id {
			return
		}
		s.reconcileViewers(v)
	})
}

func (s *Server) tryEatBedrockFood(bs *bedrockSession) bool {
if s.wm == nil || s.wm.Hunger() == nil {
	return false
}
pl := s.pm.GetPlayer(bs.id)
if pl == nil || pl.Health <= 0 || pl.Inventory == nil {
	return false
}
held := pl.Inventory.GetHeldItem()
if held == nil || held.ID == 0 || held.Count <= 0 {
	return false
}
it, ok := item.ByID(held.ID)
if !ok {
	return false
}
foodDef, isFood := world.LookupFood(it.Name)
if !isFood {
	return false
}
isCreative := pl.Gamemode == 1 || pl.Creative
if !isCreative && !s.wm.Hunger().CanEat(bs.id, foodDef.AlwaysEdible) {
	return false
}
s.wm.Hunger().Feed(bs.id, foodDef.FoodPoints, foodDef.Saturation)
if it.Name == "minecraft:golden_apple" || it.Name == "minecraft:enchanted_golden_apple" {
	s.pm.AddEffect(bs.id, player.EffectRegeneration, 2, 100)
}
if !isCreative {
	slot := pl.Inventory.HeldSlot
	newCount := held.Count - 1
	if newCount <= 0 {
		pl.Inventory.SetItem(slot, player.ItemStack{})
	} else {
		pl.Inventory.SetItem(slot, player.ItemStack{ID: held.ID, Count: newCount, Meta: held.Meta})
	}
	s.syncBedrockInventory(bs, pl)
	s.pm.PublishEquipmentChange(bs.id)
}
bs.write(&packet.ActorEvent{
	EntityRuntimeID: bedrockLocalRuntime,
	EventType:       57,
})
return true
}
