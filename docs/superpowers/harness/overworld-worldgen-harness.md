# Overworld Worldgen — Harness Engineering

Outer harness untuk agent yang mengerjakan LivingWorld Overworld generation.

> Agent = Model + Harness. Dokumen ini adalah harness-nya.

## 1. Feedforward Controls (Guides)

### 1.1 Design constraints

- **Deterministic**: same `(seed, cx, cz)` → same chunk, always.
- **Chunk-independent**: generator tidak boleh menyimpan state antar chunk.
- **Dependency-injected**: `WorldgenContext`/`DensityContext` membawa `NoiseSet`, `ClimateReader`, seed, sea level. Tidak ada package globals.
- **Layered pipeline**: density → aquifer → carver → surface → ores → cave biome → structure → decoration.
- **No runtime network calls**: data vanilla di-pre-extract dan di-commit; server tidak download saat runtime.

### 1.2 File boundaries

| Concern | Where |
|--------|-------|
| Vanilla data extraction | `third_party/go-mc/tools/` |
| Data loader | `internal/dimension/overworld/vanilladata/` |
| Density functions | `internal/dimension/overworld/density/` |
| Aquifer | `internal/dimension/overworld/aquifer/` |
| Carvers | `internal/dimension/overworld/carver/` |
| Surface rules | `internal/dimension/overworld/surface/` |
| Cave biomes | `internal/dimension/overworld/cavebiome/` |
| Structures | `internal/dimension/overworld/structure/` |
| Pipeline orchestration | `internal/dimension/overworld/pipeline/` |

### 1.3 Code conventions

- Follow existing `go-mc` and `LivingWorld` style.
- Tambah dependency hanya jika codebase sudah memakainya.
- `pk.VarInt`, `pk.UUID`, `pk.String` dsb. untuk protocol; `int32`/`string` untuk logika bisnis.
- Semua `Load*` / `New*` return error yang meaningful, jangan swallow.
- `nolint` hanya dengan komentar yang menjelaskan *mengapa*.

## 2. Feedback Controls (Sensors)

### 2.1 Computational (cheap, run every change)

```bash
go build ./...
go vet ./...
go test ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

- `go test -race` di CI (Ubuntu), lokal pakai `go test ./...`.
- `go-mc/tools` test: `cd third_party/go-mc/tools && go test ./...`.

### 2.2 Inferential (review, when task complete)

- `superpowers:subagent-driven-development` task review antar task.
- `superpowers:requesting-code-review` final whole-branch review.
- `superpowers:verification-before-completion` sebelum commit yang klaim berhasil.

### 2.3 Manual / smoke

- `go build ./cmd/worldview && ./worldview` untuk visual check.
- Seed-pair comparison dengan vanilla di masa depan (Phase 5).

## 3. Process

1. Baca spec `docs/superpowers/specs/2026-07-12-overworld-vanilla-design.md`.
2. Baca plan `docs/superpowers/plans/2026-07-12-overworld-phase0-data-extraction.md`.
3. Jalankan task secara subagent-driven (satu task per subagent, review antar task).
4. Setiap task selesai: run CI gates, commit, lanjut ke task berikutnya.
5. Akhir branch: final review dan `superpowers:finishing-a-development-branch`.

## 4. Red Flags (Stop and Reassess)

- Mengubah `LWVersion`, packet IDs, atau protocol bridge di tengah worldgen.
- Menambah global `var` untuk state worldgen.
- Mengabaikan `go vet`/`go test` karena "kecil".
- Menulis data hasil extract ke repo tanpa `.gitignore` pass-through.
- Refactor besar di luar scope task.
