# LivingWorld Plugin & Library Guide

LivingWorld can be embedded in your own Go program and extended with plugins
using only **public** packages:

- `livingworld/server` — create and run the server, and the `Host` capabilities.
- `livingworld/plugin` — events, the `Plugin` interface, and the plugin manager.

You never need to import anything under `internal/`.

## 1. Embedding the server

```go
import "livingworld/server"

srv := server.New(server.DefaultConfig())  // or server.New(nil)
srv.Run()                                   // Start + block on Ctrl-C + graceful save
```

Lower-level control:

```go
srv := server.New(cfg)
if err := srv.Start(); err != nil { /* ... */ }
defer srv.Stop()   // stops listeners and saves all worlds
```

Load configuration from YAML:

```go
cfg, err := server.LoadConfig("config/config.yml")
srv := server.New(cfg)
```

## 2. The Host capability surface

`*server.Server` implements `plugin.Host`, so the same methods are available both
directly on the server and to every plugin via the `Host` it receives in
`OnEnable`:

| Method | Purpose |
|--------|---------|
| `Broadcast(msg)` | Chat to all players |
| `Message(name, msg)` | Chat to one player |
| `Players() []string` | Connected player names |
| `PlayerCount() int` | Number of players |
| `GetBlock(x,y,z) int32` | Block state ID at a position |
| `SetBlock(x,y,z, stateID)` | Set a block (synced to Java + Bedrock) |
| `StateID(name) int32` | Resolve a block state ID by name |
| `Log(format, args...)` | Write to the server log |

Block IDs are **vanilla Java global block-state IDs**. Resolve them by name:

```go
stone := srv.StateID("minecraft:stone")
srv.SetBlock(0, 64, 0, stone)
```

## 3. Event handlers (the easy path)

Register typed handlers on the plugin manager — no plugin struct required:

```go
pm := srv.Plugins()

pm.OnPlayerJoin(func(e *plugin.PlayerJoinEvent) {
    srv.Broadcast(e.PlayerName + " joined!")
})

pm.OnPlayerChat(func(e *plugin.PlayerChatEvent) {
    if e.Message == "!ping" {
        srv.Message(e.PlayerName, "pong")
        e.Cancel() // suppress the original chat line
    }
})
```

### Cancellable events

`player.chat`, `block.break`, and `block.place` are **cancellable**. Calling
`e.Cancel()` makes the server skip the default action (and roll back the client's
optimistic prediction where applicable).

```go
bedrock := srv.StateID("minecraft:bedrock")
pm.OnBlockBreak(func(e *plugin.BlockBreakEvent) {
    if e.BlockID == bedrock {
        e.Cancel() // block stays in place
    }
})
```

## 4. Full plugin objects (lifecycle)

For stateful plugins, implement `plugin.Plugin`:

```go
type Greeter struct{ host plugin.Host }

func (g *Greeter) Name() string    { return "greeter" }
func (g *Greeter) Version() string { return "1.0.0" }

func (g *Greeter) OnEnable(host plugin.Host) error {
    g.host = host
    host.Log("greeter enabled")
    return nil
}
func (g *Greeter) OnDisable() error { return nil }

// register:
srv.Plugins().Register(&Greeter{})
```

`OnEnable` receives the `Host`, so a plugin can act on the server without holding
a reference to `*server.Server`.

## 5. Events reference

| Event type constant | Struct | Cancellable | Fields |
|---------------------|--------|-------------|--------|
| `EventPlayerJoin` | `PlayerJoinEvent` | no | `PlayerName`, `UUID` |
| `EventPlayerLeave` | `PlayerLeaveEvent` | no | `PlayerName`, `Reason` |
| `EventPlayerChat` | `PlayerChatEvent` | yes | `PlayerName`, `Message` |
| `EventPlayerMove` | `PlayerMoveEvent` | no | `PlayerName`, `X`,`Y`,`Z` |
| `EventBlockBreak` | `BlockBreakEvent` | yes | `PlayerName`, `X`,`Y`,`Z`, `BlockID` |
| `EventBlockPlace` | `BlockPlaceEvent` | yes | `PlayerName`, `X`,`Y`,`Z`, `BlockID` |
| `EventServerStart` | `ServerStartEvent` | no | — |
| `EventServerStop` | `ServerStopEvent` | no | — |

Raw registration is also available for advanced use:

```go
pm.On(plugin.EventPlayerJoin, func(e plugin.Event) {
    je := e.(*plugin.PlayerJoinEvent)
    _ = je
})
```

## 6. Complete example

See [`examples/exampleplugin/main.go`](../examples/exampleplugin/main.go) for a
runnable program combining all of the above.
