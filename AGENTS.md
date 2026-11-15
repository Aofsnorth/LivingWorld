# LivingWorld — Engineering & Agent Guidelines

LivingWorld is a high-performance, native hybrid Minecraft server (Java Edition + Bedrock Edition cross-play) written in Go (`1.27.2`) with a Rust terrain-generation bridge (`native/steel-bridge`).

---

## 0. Mandatory Professional Engineering, Git Commit & Documentation Standards

Every human contributor and AI coding agent operating in this repository MUST enforce strict, production-grade software engineering discipline inspired by industry benchmarks ([GoCraft](https://github.com/GoCraft-MC/GoCraft), PaperMC, and empirical software engineering best practices):

1. **Strict Conventional Commits 1.0.0 (Zero Vague Commits)**:
   - Every git commit message MUST follow the format: `<type>(<scope>): <imperative summary>` (under 72 characters in the subject line).
   - Allowed `<type>` values: `feat`, `fix`, `perf`, `refactor`, `test`, `docs`, `build`, `ci`, `chore`.
   - Allowed `<scope>` examples: `world`, `player`, `mobs`, `java`, `bedrock`, `combat`, `hunger`, `weather`, `skin`, `ci`, `native`.
   - **STRICTLY FORBIDDEN**: Single-word, ambiguous, or placeholder commit messages such as `Add`, `Idk`, `update`, `fix bug`, `wip`, or `test`.
   - Write the subject in the **imperative mood** (e.g., `feat(mobs): implement binary-heap A* pathfinding and spatial grid collisions`, `fix(bedrock): synchronize 3-way respawn handshake and death metadata`).
2. **Atomic Commits, Size Budget & Clean Worktree Discipline**:
   - **Git Author Attribution**: Every commit in this repository MUST be authored by `Aofsnorth <aidanpitra@gmail.com>` (`git config user.name "Aofsnorth"` and `git config user.email "aidanpitra@gmail.com"`).
   - **Small Atomic Diffs**: Keep each commit small and focused (ideally `<= 100–150` changed lines per commit) so `git bisect` and code review remain effortless. Split large features into sequential, self-contained atomic commits.
   - Each commit MUST represent a single, cohesive, and self-contained logical change that compiles cleanly and passes all unit/integration tests.
   - Never mix unrelated refactors, formatting sweeps, and feature work in the same commit.
   - Never commit temporary build artifacts (`*.exe`, `*.test`, `*.prof`, `*.out`), secrets, API keys, or local world saves.
3. **Edition-Agnostic Core Isolation**:
   - Domain packages (`internal/world`, `internal/player`, `internal/mobs`, `internal/combat`, `internal/drops`, `internal/item`) MUST remain 100% edition-agnostic and MUST NEVER import `internal/java/...` or `internal/bedrock/...`.
   - Protocol-specific wire formats, packet IDs, and metadata indices belong exclusively in `internal/java` and `internal/bedrock`.
4. **Pre-Commit Quality & Security Gate**:
   - Before finalizing any task or commit, the agent MUST verify:
     1. `go fmt ./...` (zero formatting drift)
     2. `go build ./...` (zero compilation errors)
     3. `go vet ./...` (zero static analysis warnings)
     4. `go test ./...` (100% test pass rate; every bug fix or feature MUST include regression tests)
     5. `govulncheck ./...` (zero known security vulnerabilities when dependencies change)
5. **Documentation & Specification Synchronization**:
   - Keep `README.md`, `AGENTS.md`, package doc comments, and `config/config.yml` comments synchronized whenever protocols, CLI flags, configuration keys, or gameplay systems change.

---

## 1. Core Mandate #1: Performance & Optimization First

**Performance and resource optimization are the #1 priority across every execution, architectural decision, and code change.** Every modification MUST adhere to the following non-negotiable standards:

1. **Zero-Allocation & Low-Overhead Hot Paths**:
   - The 20 Hz world tick loop (`internal/world`), chunk encoding/streaming (`internal/java/world`, `internal/bedrock/world`), packet serialization, entity/player movement synchronization, and AI ticks are critical hot paths.
   - Avoid unnecessary heap allocations, reflection, boxed interface conversions, or redundant slice/map copies on hot paths. Reuse buffers and pre-size slices (`make([]T, 0, cap)`) wherever bounds are known.
2. **Lock Contention & Concurrency Discipline**:
   - Keep critical sections under `sync.Mutex` / `sync.RWMutex` minimal and strictly non-blocking. Never perform disk I/O, network I/O, PNG encoding, compression, or synchronous event fan-out while holding a shared lock.
   - Prefer `RLock` for read-only lookups and copy minimal snapshots before releasing locks.
3. **I/O, Persistence & Network Efficiency**:
   - Guard against redundant disk reads/writes (e.g., idempotent storage attachment, dirty-chunk tracking before region writes, SHA-256 caching for signed skin uploads).
   - Offload expensive or external I/O (such as MineSkin HTTP signing or bulk region flushes) off the main tick and packet-handling loops.
4. **Algorithmic & Memory Efficiency**:
   - Bound all caches, ring buffers, and in-flight maps. Evict unused probe chunks and avoid retaining large byte slices longer than necessary.

---

## 2. Mandatory Continuous Full-Project Bug & Performance Audit (Every Turn)

**In EVERY single turn and execution — without exception — the AI agent MUST explicitly and continuously audit the project for bugs and performance bottlenecks:**

1. **Continuous Proactive Auditing**:
   - Every execution MUST include an explicit check for bugs, race conditions, deadlocks, memory/goroutine leaks, protocol desyncs (Java <-> Bedrock), and performance regressions across the project.
   - **Even when no known bugs remain or after a user-requested feature/fix is finished**, the agent MUST continue performing a full, proactive bug and performance audit on every turn rather than assuming the codebase is clean.
2. **Explicit Audit Reporting**:
   - Every response MUST explicitly state the findings of the turn's bug and performance audit (either detailing and fixing discovered issues or confirming the exact subsystems audited and verified).

---

## 3. Strict Test Organization Policy

- **All test files (`*_test.go`) MUST be placed exclusively inside the `tests/` directory (`tests/bedrock`, `tests/java`, `tests/blockconv`, `tests/vanillagen`, etc.).**
- **NEVER create or place `*_test.go` files inside `internal/`, `server/`, `cmd/`, `config/`, or `plugin/`.**

---

## 4. Verification Gates

Run all three verification gates before completing any code change:

```bash
go build ./...
go vet ./...
go test ./...
```

---

## 5. Architecture & Lifecycle Reference

- **Composition Root (`server/new.go`)**:
  - `server.New(cfg)` wires the `world.Manager`, attaches disk persistence (`worlds.EnablePersistence`) prior to terrain generator initialization so `level.json` (including persisted seed, time, and weather) is loaded first, resolves the world seed (`cfg.World.Seed`: configured value, persisted `level.json` seed, or cryptographically generated random 64-bit seed when unset/`0`), and initializes the `player.Manager`, `skinbridge.Service`, `java.Server`, and `bedrock.Server`.
- **Direct Server Lifecycle (`server/server.go`, `server/tui.go`)**:
  - `Server.Start()` starts the local `skinbridge` HTTP service, starts world autosave and player persistence, loads drop-in plugins from `./plugins`, and starts the Java and Bedrock listeners.
  - `Server.Run()` starts the server, launches the interactive stdin operator console (`server/console.go`), blocks on OS termination signals (`SIGINT`/`SIGTERM`) or the console `stop` command, and invokes `Server.Stop()`.
  - `Server.RunTUI()` runs the BubbleTea terminal UI (`server/tui.go`) directly over `Server.Start()` and `Server.Stop()`.
  - `Server.Stop()` is strictly idempotent (`sync.Once`): it emits `EventServerStop`, saves all online players, stops both protocol listeners, and flushes/closes all worlds.
- **Cross-Edition Skin Bridge (`internal/skinbridge`)**:
  - Normalizes and compresses Bedrock skins (`64x32`, `64x64`, `128x128`, `256x256`) into Java-compatible `64x64` PNG/RGBA textures (`CompressBedrockSkinRGBA`) with perceptual salience downscaling and base-layer UV alpha sanitization, serving them locally and asynchronously signing via MineSkin with SHA-256 caching.
