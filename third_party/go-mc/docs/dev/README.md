# Developer Documentation

Internal documentation for go-mc contributors and maintainers.

## Contents

- [tools.md](tools.md) — Code generation & MC data extraction pipeline

## What's Generated vs Hand-Written

**Generated** (do not edit manually — see [tools.md](tools.md) for the pipeline):
- All files in `data/packetid/`, `data/soundid/`, `data/item/`, `data/entity/`
- `data/registryid/*.go` (95 files)
- `data/lang/<locale>/<locale>.go` (147 languages)
- `level/block/blocks.go`, `block_states.nbt`, `properties_enum.go`
- `level/block/blockentity.go` (constants), `blockentities.go` (mappings)
- `level/biome/list.go` — biome list (65 entries)
- `level/component/components.go` (component constructor switch)
- `level/component/*_gen.go` (component type structs)

**Hand-written** (not generated):
- `level/component/*.go` (without `_gen` suffix) — shared helpers (EitherHolder, Holder, etc.)
- `bot/`, `server/`, `net/`, `nbt/`, `chat/`, `save/` — all framework code
- `data/packetid/legacy.go` — backward-compat aliases
