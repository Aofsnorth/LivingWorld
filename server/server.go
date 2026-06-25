// Package server is LivingWorld's public, embeddable API. Import it to run a
// native dual-protocol (Java + Bedrock) Minecraft server from your own Go program,
// dragonfly-style:
//
//	func main() {
//	    srv := server.New(server.DefaultConfig())
//	    srv.Plugins().OnPlayerJoin(func(e *plugin.PlayerJoinEvent) {
//	        srv.Broadcast("Welcome, " + e.PlayerName + "!")
//	    })
//	    srv.Run() // blocks until Ctrl-C, then saves and shuts down
//	}
//
// All capabilities a plugin needs are also available directly on *Server, which
// implements plugin.Host.
package server

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"livingworld/config"
	"livingworld/internal/bedrock"
	"livingworld/internal/command"
	"livingworld/internal/dimension"
	"livingworld/internal/infrastructure/logging"
	"livingworld/internal/java"
	"livingworld/internal/mobs"
	"livingworld/internal/player"
	"livingworld/internal/skinbridge"
	"livingworld/internal/world"
	worldgen "livingworld/internal/world/generator"
	"livingworld/plugin"

	"github.com/google/uuid"
)

// zeroUUID returns a [16]byte of zeros. Used as a placeholder UUID when
// emitting a side-effect event whose primary subject is an entityID
// (e.g. a mob swing) and not a player.
func zeroUUID() [16]byte { return [16]byte{} }

// serverRNG is the package-level RNG used by the drop / split
// callbacks (M1). Seeded once with the current time at startup.
// math/rand's global is also fine for v1 — the callbacks run
// once per mob despawn, not per tick, so the contention cost is
// negligible.
var serverRNG = rand.New(rand.NewSource(time.Now().UnixNano()))

// findLandSpawn searches outward from (x, z) for a column whose surface
// is dry land (above sea level and not water/ice). Overworld seeds can
// put the configured spawn in the middle of an ocean; vanilla performs
// the same kind of search. Scans rings every 16 blocks up to ~160
// blocks out and falls back to the original point if everything is wet.
func findLandSpawn(w *world.World, x, z int) (int, int) {
	waterID := world.WaterID
	iceID := world.StateID("minecraft:ice")
	isLand := func(px, pz int) bool {
		y := w.HighestSolidY(px, pz) // feet Y, one above the surface block
		if y <= 63 {
			return false
		}
		below := w.GetBlock(px, y-1, pz).ID()
		return below != waterID && below != iceID
	}
	if isLand(x, z) {
		return x, z
	}
	for radius := 16; radius <= 160; radius += 16 {
		for dx := -radius; dx <= radius; dx += 16 {
			for dz := -radius; dz <= radius; dz += 16 {
				// Ring only: skip interior points already scanned.
				if dx > -radius && dx < radius && dz > -radius && dz < radius {
					continue
				}
				if isLand(x+dx, z+dz) {
					return x + dx, z + dz
				}
			}
		}
	}
	return x, z
}

// Config is the server configuration. It is an alias of the YAML config type, so
// it can be built in code or loaded from a file via LoadConfig.
type Config = config.Config

// DefaultConfig returns a ready-to-run configuration (superflat world, offline
// mode, persistence enabled).
func DefaultConfig() *Config { return config.Default() }

// LoadConfig reads configuration from a YAML file, falling back to defaults for
// any missing fields.
func LoadConfig(path string) (*Config, error) { return config.Load(path) }

// Server is a running (or runnable) LivingWorld instance. The zero value is not
// usable; construct one with New.
type Server struct {
	cfg     *Config
	worlds  *world.Manager
	players *player.Manager
	skins   *skinbridge.Service
	java    *java.Server
	bedrock *bedrock.Server
	logger  logging.Logger

	ops       *OpsList
	whitelist *Whitelist
}

// New builds a server from cfg. Pass nil to use DefaultConfig. The server does
// not listen until Start (or Run) is called.
func New(cfg *Config) *Server {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	logger := logging.GetLogger("Server")

	worlds := world.NewManager()
	dw := worlds.GetDefaultWorld()
	switch cfg.World.Type {
	case "", "superflat":
		dw.SetGenerator(worldgen.NewSuperflat())
	case "overworld", "default", "normal":
		overworld := dimension.NewOverworld()
		dw.SetGenerator(overworld.Generator(cfg.World.Seed))
		// Spawn on the generated surface (overworld terrain isn't flat at
		// y=4), nudging the spawn point to dry land if the configured XZ
		// is in an ocean or river.
		sx, sz := findLandSpawn(dw, int(cfg.World.Spawn.X), int(cfg.World.Spawn.Z))
		cfg.World.Spawn.X = float64(sx) + 0.5
		cfg.World.Spawn.Z = float64(sz) + 0.5
		cfg.World.Spawn.Y = float64(dw.HighestSolidY(sx, sz))
	case "nether":
		n := dimension.NewNether()
		dw.SetGenerator(n.Generator(cfg.World.Seed))
	case "end":
		e := dimension.NewEnd()
		dw.SetGenerator(e.Generator(cfg.World.Seed))
	default:
		logger.Warn("Unknown world.type %q, falling back to superflat", cfg.World.Type)
		dw.SetGenerator(worldgen.NewSuperflat())
	}

	players := player.NewManager()
	players.StartPushLoop() // cross-edition player-to-player pushing
	// Let the world's mob-spawn director read live player positions (the world
	// package can't import player directly — import cycle).
	worlds.SetPlayerLocator(func() []world.Position {
		all := players.GetAllPlayers()
		pts := make([]world.Position, 0, len(all))
		for _, p := range all {
			pts = append(pts, p.Position)
		}
		return pts
	})
	// Mob-AI player surface: a richer struct than world.Position because
	// the AI needs uuid + sneaking + gamemode for target selection. Head
	// detection (skeleton/zombie/creeper skull reduces follow-range by 50%)
	// isn't on the Player struct today, so v1 always passes false; the
	// detection math is harmless when worn-head is a no-op.
	worlds.SetMobAIPlayerList(func() []mobs.PlayerTarget {
		all := players.GetAllPlayers()
		out := make([]mobs.PlayerTarget, 0, len(all))
		for _, p := range all {
			out = append(out, mobs.PlayerTarget{
				UUID:        p.UUID,
				X:           p.Position.X,
				Y:           p.Position.Y,
				Z:           p.Position.Z,
				Sneaking:    p.Sneaking,
				WearingGold: p.WearingGold(),
				Gamemode:    p.Gamemode,
				LookYaw:     float64(p.Rotation.Yaw),
				LookPitch:   float64(p.Rotation.Pitch),
			})
		}
		return out
	})
	// Mob-AI side-effect hooks. The bridge subscriptions for OnSpawn/OnDespawn
	// (arrows, explosions) are wired below in the bridge.NewServer calls; this
	// block wires the player-damage half of each event.
	worlds.SetMobAICallbacks(
		// aiMeleeAttack: apply HP loss + swing animation packet.
		func(targetUUID [16]byte, attackerID int64, damage float32) {
			if tgt := players.GetPlayer(targetUUID); tgt != nil {
				tgt.Damage(damage)
			}
			// Publish a swing from the attacker's own controller so the
			// animation plays on every nearby client. (The mob doesn't
			// have a UUID; we use the entityID.)
			players.PublishSwing(zeroUUID())
		},
		// aiShootArrow: spawn a Projectile in the shared store. Bridges
		// subscribe to OnSpawn (in the bedrock.NewServer / java.NewServer
		// constructors) so this single Spawn call fans out to every
		// connected client on both editions.
		func(shooterID int64, x, y, z, yaw, pitch float64) {
			worlds.Projectiles().Spawn(shooterID, [16]byte{}, x, y, z, yaw, pitch)
		},
		// aiExplode: build the ExplosionResult, apply player damage +
		// knockback, then publish to all bridges.
		func(attackerID int64, x, y, z, power float64) {
			pl := players.GetAllPlayers()
			targets := make([]mobs.PlayerTarget, 0, len(pl))
			for _, p := range pl {
				targets = append(targets, mobs.PlayerTarget{
					UUID: p.UUID, X: p.Position.X, Y: p.Position.Y, Z: p.Position.Z,
				})
			}
			hits := mobs.AffectedPlayers(targets, x, y, z, power)
			for _, h := range hits {
				if tgt := players.GetPlayer(h.UUID); tgt != nil {
					tgt.Damage(h.Damage)
					tgt.Push(h.KnockX, h.KnockY, h.KnockZ)
				}
			}
			worlds.PublishExplosion(mobs.ExplosionResult{
				X: x, Y: y, Z: z, Power: power, Radius: power * 2, Hits: hits,
			})
		},
		// aiProjectileHit: skeleton arrow landed on a player. Apply
		// 1.5 hearts of damage (vanilla arrow damage rolls 1.5-2.5).
		func(arrowID int64, targetUUID [16]byte) {
			if tgt := players.GetPlayer(targetUUID); tgt != nil {
				tgt.Damage(1.5)
			}
		},
		// aiFireDamage: M0.6 — a mob in direct sunlight took fire
		// damage. For v1 we just subtract HP from the mob. v2
		// should add a fire-overlay packet to the bridges so the
		// mob visibly burns; the world tick is the right place to
		// also broadcast that (the bridge mob sync subscribes to
		// OnMove which already runs every tick, so it can be wired
		// by reading m.OnFire / a per-mob FireTicks field).
		func(mobID int64, damage float32) {
			worlds.Mobs().HurtFire(mobID, damage)
		},
		// aiSound: UX — fan out mob sounds to bridges. The Java
		// bridge translates each emit into ClientboundGameSoundEntity;
		// the Bedrock bridge translates into LevelSoundEvent. Both
		// use the same mob.EntityID as the source. The bridges
		// register their listeners via worlds.OnMobSound at boot.
		func(emits []mobs.SoundEmit) {
			if len(emits) == 0 {
				return
			}
			worlds.PublishMobSounds(emits)
		},
		// aiHitEffect: M6 — melee swing applies a status effect
		// (husk → hunger, cave spider → poison, wither skeleton →
		// wither, witch → instant_damage). The effect is stored in
		// the player's per-effect bag, which the world tick
		// (Phase 4e) drains: poison / wither apply 0.5 HP per tick
		// at vanilla cadence, hunger is rendered client-side
		// (vanilla applies the food-level debuff, not direct HP).
		// instant_damage is one-shot: 6 HP × 2^level applied here
		// and no effect is added to the bag.
		//
		// The bridges see EffectStatus when a tickable effect is
		// added, and EffectStatusRemove when it expires or is
		// removed — AddEffect publishes both, plus the per-tick
		// damage path goes through Manager.TickEffects.
		func(targetUUID [16]byte, attackerID int64, effect mobs.HitEffect) {
			if effect.Type == "instant_damage" {
				// Witch's harming potion: 6 HP × 2^level, applied
				// immediately; no bag entry.
				if tgt := players.GetPlayer(targetUUID); tgt != nil {
					mul := 1
					for i := 0; i < effect.Level; i++ {
						mul *= 2
					}
					tgt.Damage(6 * float32(mul))
				}
				return
			}
			eid := player.EffectIDForHitEffectType(effect.Type)
			if eid == 0 {
				return // unknown / empty effect type; no-op
			}
			// effect.Seconds is in ticks / 20 in mobs.HitEffect
			// (see defs.go comments). Round to ticks for the bag.
			ticks := effect.Seconds * 20
			if ticks <= 0 {
				ticks = 20 // safety net: never apply a 0-tick effect
			}
			players.AddEffect(uuid.UUID(targetUUID), eid, effect.Level, ticks)
		},
		// aiThrow: M1 — iron golem picks up a player and throws them.
		// Vanilla: 3+ b upward velocity + 4 fall damage on landing.
		// v1: just the upward velocity; the player takes fall
		// damage naturally when they land.
		func(targetUUID [16]byte, attackerID int64, damage float32) {
			if tgt := players.GetPlayer(targetUUID); tgt != nil {
				// 3+ b upward + slight horizontal nudge.
				tgt.Push(0, 1.0, 0)
			}
		},
		// aiShootProjectile: M1 — unified ranged-fire hook. The
		// projectileType string picks the kind. The default path
		// spawns a vanilla arrow; the bridges extend this for
		// fireball / potion / trident types (M1.6 — projectile
		// store + bridges).
		func(shooterID int64, x, y, z, yaw, pitch float64, projectileType string) {
			// M1.6: for v1 we route all ranged fire through
			// the existing arrow store; the kind tag is
			// carried as a ProjectileKind field that the
			// bridges read on OnSpawn.
			worlds.Projectiles().SpawnKind(shooterID, [16]byte{}, x, y, z, yaw, pitch, projectileType)
		},
		// aiWaterAt: M1 — water-cell probe for WaterSensitive mobs
		// (enderman). For v1 we return true if the cell is a
		// water source/block — the world package is the
		// authoritative source.
		func(x, y, z int) bool {
			w := worlds.GetDefaultWorld()
			return w.GetBlock(x, y, z).ID() == world.WaterID
		},
	)
	// M1: slime / magma cube splits. The world tick fires this
	// for every despawning mob with def.SplitsOnDeath && Size>1.
	// We spawn 2 children at the parent's last position with
	// Size-1. The bridges get the OnSpawn callback via
	// SpawnAtSize.
	worlds.SetMobSplitCallback(func(w *world.World, mobID int64) {
		mob := worlds.Mobs().Get(mobID)
		if mob.Type == "" {
			return
		}
		def := mobs.DefFor(mob.Type)
		if !def.SplitsOnDeath || mob.Size <= 1 {
			return
		}
		worlds.Mobs().SpawnAtSize(mob.Type, mob.X, mob.Y, mob.Z, mob.Size-1)
		worlds.Mobs().SpawnAtSize(mob.Type, mob.X, mob.Y, mob.Z, mob.Size-1)
	})
	// M1: loot drops. For each despawning mob with def.Drops,
	// roll the loot and queue a drops.Store.Spawn for each
	// rolled item.
	worlds.SetMobDropCallback(func(w *world.World, mobID int64) {
		mob := worlds.Mobs().Get(mobID)
		if mob.Type == "" {
			return
		}
		def := mobs.DefFor(mob.Type)
		if len(def.Drops) == 0 {
			return
		}
		// Use the world's RNG via a local closure (server's
		// RNG is package-level).
		for _, d := range def.Drops {
			if d.Item == "" {
				continue
			}
			if d.Chance < 1 && serverRNG.Float32() > d.Chance {
				continue
			}
			n := d.Min
			if d.Max > d.Min {
				n += int(serverRNG.Int31n(int32(d.Max - d.Min + 1)))
			}
			for i := 0; i < n; i++ {
				worlds.Drops().Spawn(d.Item, 0, mob.X+0.5, mob.Y+0.25, mob.Z+0.5)
			}
		}
	})
	// M6: status-effect wiring. The player manager publishes an
	// EffectStatus / EffectStatusRemove world event from
	// AddEffect / TickEffects; the bridges translate those into
	// their edition's wire packet. The world tick Phase 4e calls
	// aiTickEffects, which is the closure that drives the per-tick
	// damage engine. The split mirrors the existing aiMeleeAttack
	// pattern: the world package stays protocol-agnostic, the
	// server.go bootstrap owns the player manager reference.
	players.SetEffectBus(worlds)
	worlds.SetEffectTickCallback(players.TickEffects)
	// M7: I-frames countdown. The bridge routeAttack path stamps
	// a fresh window via players.HitIFrames; the per-tick engine
	// here decrements once per 20 Hz tick. Both share Phase 4e
	// with the effect tick.
	worlds.SetIFramesTickCallback(players.IFramesTick)
	worlds.StartTimeLoop(cfg.World.DayNightCycle)
	worlds.SetDifficulty(cfg.World.Difficulty)
	worlds.SetSpawnMode(cfg.World.SpawnMode)
	worlds.SetSpawnMobsEnabled(cfg.World.SpawnMobs)
	wd := cfg.World.WeatherDurations
	worlds.StartWeatherCycle(cfg.World.WeatherCycle, world.WeatherDurations{
		ClearMin:   wd.ClearMinSeconds,
		ClearMax:   wd.ClearMaxSeconds,
		RainMin:    wd.RainMinSeconds,
		RainMax:    wd.RainMaxSeconds,
		ThunderMin: wd.ThunderMinSeconds,
		ThunderMax: wd.ThunderMaxSeconds,
	})
	worlds.StartDropPhysics()
	command.Bind(players, worlds)
	command.RegisterBuiltins(command.Default())
	skins := skinbridge.New()
	s := &Server{
		cfg:     cfg,
		worlds:  worlds,
		players: players,
		skins:   skins,
		java:    java.NewServer(cfg, players, worlds),
		bedrock: bedrock.NewServer(cfg, players, worlds, skins),
		logger:  logger,
	}
	// Ops are seeded from cfg (Config.Ops drives the login op-check); the
	// whitelist starts empty and disabled. Main replaces these with
	// file-backed instances when launched from the command line.
	s.ops = newOps(cfg.Ops...)
	s.whitelist = newWhitelist()
	// Make this server the capability surface handed to plugins.
	plugin.Manager().SetHost(s)
	// Wire /op and /deop to the server's operator list.
	command.BindOps(s)
	return s
}

// Start begins listening on both protocols and enables world persistence and
// autosave. It returns once both listeners are up.
func (s *Server) Start() error {
	s.skins.Start()

	if s.cfg.World.Persistence {
		if err := s.worlds.EnablePersistence(s.cfg.World.Directory); err != nil {
			s.logger.Warn("World persistence disabled (setup failed): %v", err)
		} else {
			s.worlds.StartAutosave(time.Duration(s.cfg.World.AutosaveSeconds) * time.Second)
			if err := s.players.EnablePersistence(filepath.Join(s.cfg.World.Directory, "playerdata")); err != nil {
				s.logger.Warn("Player persistence disabled (setup failed): %v", err)
			}
			s.logger.Info("World persistence enabled at %q (autosave every %ds)", s.cfg.World.Directory, s.cfg.World.AutosaveSeconds)
		}
	}

	// Load drop-in JavaScript plugins from ./plugins before accepting players so
	// their event handlers are registered up front.
	if n, err := plugin.Manager().LoadScripts("plugins"); err != nil {
		s.logger.Warn("Plugin scripts: %v", err)
	} else if n > 0 {
		s.logger.Info("Loaded %d script plugin(s) from ./plugins", n)
	}

	if err := s.java.Start(); err != nil {
		return err
	}
	if err := s.bedrock.Start(); err != nil {
		s.java.Stop()
		return err
	}
	plugin.Manager().Emit(&plugin.ServerStartEvent{BaseEvent: plugin.BaseEvent{Type_: plugin.EventServerStart}})
	s.logger.Info("LivingWorld started — Java %s, Bedrock %s", s.cfg.Address(), s.cfg.BedrockAddress())
	return nil
}

// Stop shuts down both listeners and saves all worlds. Safe to call more than once.
func (s *Server) Stop() {
	plugin.Manager().Emit(&plugin.ServerStopEvent{BaseEvent: plugin.BaseEvent{Type_: plugin.EventServerStop}})
	s.players.SaveAll() // persist player data before disconnecting everyone
	if s.bedrock != nil {
		s.bedrock.Stop()
	}
	if s.java != nil {
		s.java.Stop()
	}
	if err := s.worlds.Close(); err != nil {
		s.logger.Error("Error saving worlds on shutdown: %v", err)
	}
}

// Run starts the server and blocks until SIGINT/SIGTERM (or the console
// "stop" command), then shuts down gracefully via the application harness.
// The harness owns signal handling, the operator console, ordered component
// shutdown, and health reporting; see NewHarness for the extensibility seam.
// It returns the Start error, if any.
func (s *Server) Run() error {
	h := s.NewHarness()
	_ = h.Register(&consoleComponent{srv: s, out: os.Stdout, stop: func() { _ = h.Stop() }})
	return h.Run(context.Background())
}

// Plugins returns the plugin manager for registering plugins and event handlers.
func (s *Server) Plugins() *plugin.PluginManager { return plugin.Manager() }

// Worlds returns the underlying world manager for advanced use.
func (s *Server) Worlds() *world.Manager { return s.worlds }

// PlayerManager returns the underlying player manager for advanced use.
func (s *Server) PlayerManager() *player.Manager { return s.players }

// Ops returns the operator list (admins) for runtime management.
func (s *Server) Ops() *OpsList { return s.ops }

// SetOp grants or revokes operator status by name: it updates the ops list,
// keeps the login op-check (Config.Ops) in sync, and applies to the connected
// player immediately. Returns whether membership actually changed.
func (s *Server) SetOp(name string, op bool) bool {
	var changed bool
	if op {
		changed, _ = s.ops.Add(name)
	} else {
		changed, _ = s.ops.Remove(name)
	}
	s.cfg.Ops = s.ops.List()
	if pl := s.players.GetPlayerByName(name); pl != nil {
		pl.Op = op
	}
	return changed
}

// ListOps returns the current operator names.
func (s *Server) ListOps() []string { return s.ops.List() }

// Whitelist returns the join whitelist. Enforcement at login is performed by
// the protocol layer (it should reject players for which Allowed is false);
// toggle it with Whitelist().SetEnabled.
func (s *Server) Whitelist() *Whitelist { return s.whitelist }

// --- plugin.Host implementation ---

// Broadcast sends a chat message to every connected player.
func (s *Server) Broadcast(msg string) { s.players.Broadcast(msg) }

// Message sends a chat message to a single player by name.
func (s *Server) Message(playerName, msg string) {
	if p := s.players.GetPlayerByName(playerName); p != nil {
		s.players.Message(p.UUID, msg)
	}
}

// Players returns the names of currently connected players.
func (s *Server) Players() []string {
	all := s.players.GetAllPlayers()
	names := make([]string, 0, len(all))
	for _, p := range all {
		names = append(names, p.Username)
	}
	return names
}

// PlayerCount returns the number of connected players.
func (s *Server) PlayerCount() int { return s.players.PlayerCount() }

// GetBlock returns the block state ID at a world position.
func (s *Server) GetBlock(x, y, z int) int32 {
	return s.worlds.GetDefaultWorld().GetBlock(x, y, z).ID()
}

// SetBlock sets the block state ID at a world position and notifies clients on
// both protocols.
func (s *Server) SetBlock(x, y, z int, stateID int32) {
	s.worlds.SetBlockAndPublish(world.BlockUpdateSourceServer, x, y, z, world.BlockByID(stateID))
}

// StateID resolves a block state ID from a namespaced name ("minecraft:stone").
func (s *Server) StateID(name string) int32 { return world.StateID(name) }

// Log writes a line to the server log.
func (s *Server) Log(format string, args ...any) { s.logger.Info(format, args...) }
