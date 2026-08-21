## qkdpostproc

BBM92-style classical post-processing chain: coincidence **sifting**, block **error detection** (never bit-flipping), CRC **verification**, **QBER** estimation, **privacy amplification** (Devetak-Winter rate + Toeplitz hashing).

Library package (`import "qkdpostproc"`); `cmd/qkdpostproc` is a thin CLI driver that simulates click streams and calls `qkdpostproc.Run(...)`. For real detector logs, build `[]qkdpostproc.DetectionEvent` yourself and call `Run` directly.

### Layout

| file | step |
|---|---|
| `events.go` | shared `DetectionEvent`/`Basis` types |
| `sifting.go` | coincidence-window sifting |
| `blockdetect.go` | block Hamming-syndrome detection (parallel) |
| `crcverify.go` | CRC-32 chunk verification (parallel) |
| `qber.go` | QBER from discarded material |
| `toeplitz.go` | bit-packed, parallel Toeplitz universal hash (portable) |
| `toeplitz_clmul_amd64.{go,s}` | hardware-CLMUL Toeplitz hash (amd64 only) |
| `privacyamp.go` | secure key length + privacy amplification |
| `pipeline.go` | `Run()`: orchestrates steps 1-5 |
| `cmd/tuneblock` | sweeps step 2 block size for the highest key rate |

### Quick start

```bash
go build ./...
go run ./cmd/qkdpostproc -n 300000 -err 0.005 -block 24 -chunk 3000
```

Key flags (`-h` for all): `-n` raw attempts, `-err` intrinsic bit error rate, `-window` coincidence window (ps), `-block` step 2 Hamming block size, `-chunk` step 3 CRC chunk size.

### Library usage

```go
import qkd "qkdpostproc"

cfg := qkd.Config{CoincidenceWindowPS: 500, BlockSize: 24, ChunkSize: 2048}
res, err := qkd.Run(aliceEvents, bobEvents, cfg)
res.FinalKeyBytes // ready to feed into your KMS
```

### Performance

Steps 2 and 3 split per-block/per-chunk work across a `runtime.GOMAXPROCS(0)` worker pool. Step 5's Toeplitz hash is the quadratic-shaped bottleneck (`O(ln)`): on amd64 with `PCLMULQDQ` (checked at runtime), the whole output is one hardware carry-less-multiply polynomial product instead of a popcount per output bit --> `2-10x` over the portable `POPCNT` fallback used everywhere else, both parallelized. `toeplitzHashNaive` is kept only as a correctness oracle for tests, never use it in production. Run `make bench` for numbers on your machine.

### Tuning

This is detect-and-discard, never-flip, so step 2's block size trades survival rate against leaked syndrome bits: smaller blocks survive more often per-block but leak proportionally more. At high QBER + small blocks the final key can hit zero: that's expected, not a bug.

`make tune` (or `./scripts/tune-blocksize.sh -h`) sweeps block sizes over one fixed simulated batch and reports which one gives the highest key rate (final key bits per raw attempt), plus how many fixed-size keys (`-keybits`, default 256) that batch yields at each size.

### Makefile targets

```
make build     # go build ./...
make test      # go test ./... -v
make test-all  # build+vet+fmt+race tests+cross-arch build, see scripts/test.sh
make bench     # go test -run '^$' -bench . -benchmem ./...
make tune      # sweep step 2 block size for the highest key rate
make run       # go run ./cmd/qkdpostproc with sane demo defaults
make vet       # go vet ./...
make fmt       # gofmt -l . (fails if anything is unformatted)
make clean     # remove build artifacts
```
