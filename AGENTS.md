# LivingWorld — Agent Notes

Project-wide context for agents working in this repo. Append-only; keep it
short and high-signal.

## Verification (CI gates, must stay green)

```
go build ./...
go vet ./...
go test ./...
govulncheck ./...
```

`go test -race` requires cgo (gcc). It runs in CI (Ubuntu) but not on a
Windows dev box without a C toolchain — use `go test ./...` locally.

## Application Harness (`internal/harness`)

LivingWorld's lifecycle is owned by the `internal/harness` package, an
application harness built on harness-engineering principles (lifecycle state
machine, dependency-ordered component startup / reverse shutdown, health
probes as computational feedback sensors, phase hooks as the feedforward/
feedback seam, injectable signal source, metrics recorder seam). It is
Minecraft-agnostic; the `server` package adapts to it.

Key types:
- `Component` / `Dependent` / `Healthchecked` — component contracts (ISP).
- `Runtime` — per-phase context (context.Context + Logger + Metrics +
  sibling lookup). This is the DI seam; components never reach for globals.
- `Harness` — orchestrator. `Start` runs Init+Start phases with rollback on
  failure; `Run` adds signal-driven blocking + graceful shutdown; `Stop` is
  idempotent.
- `Registry` — topological sort of components by `DependsOn()`.

Integration points in `server`:
- `server.NewHarness` — builds a harness with the server as a component
  (`livingworld.server`) plus before/after-stop log hooks. Public extensibility
  seam for embedders.
- `serverComponent` / `consoleComponent` — adapters in `server/harness.go`.
- `Server.Run` / `Server.RunTUI` are now hosted by the harness. Entry points
  (`cmd/server/main.go`, `server.Main`, `examples/...`) are unchanged because
  the public `Run`/`RunTUI` API is preserved.

When adding a new long-running subsystem, prefer implementing
`harness.Component` and registering it in `NewHarness` (or via a hook) rather
 than ad-hoc goroutines in `Server.Start`.
