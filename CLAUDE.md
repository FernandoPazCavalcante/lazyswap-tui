# CLAUDE.md — lazyswap

Go rewrite of the original Bun/TS app (now `lazyswap-old/`). Bubble Tea TUI + non-interactive
CLI for on-chain DEX swaps (Uniswap V2 / PancakeSwap on EVM) plus cross-chain BTC via THORchain.

Module: `github.com/FernandoPazCavalcante/lazyswap`. Needs **Go 1.26+**.

## Commands

```bash
go build -o lazyswap .           # build (gitignored binary); or `go run .`
go test ./...                    # all tests
go test ./internal/swap/         # single package
go test -run TestQuote ./...     # filter by name
go test -cover ./...             # coverage
bash scripts/build-release.sh    # cross-compile tarballs → dist/ (CGO_ENABLED=0)
```

## Architecture

Entry: `main.go`. **With args → `cli.Run`; with none → launches the TUI.**

Layers: **TUI → services → DAO / blockchain**. All packages live under `internal/`:

- `internal/crypto` — AES-256-GCM + PBKDF2 (100k iters)
- `internal/wallet` — wallet CRUD + SQLite DAO (`modernc.org/sqlite`, cgo-free)
- `internal/chain` — `config.go` `CHAINS` map + contract ABIs; `DefaultKey = "bsc"`
- `internal/dex`, `internal/swap`, `internal/thorchain` — quote/orchestrate/execute (EVM + BTC)
- `internal/balance`, `internal/explorer` — balance fetch/format, explorer API
- `internal/pass` — LazySwapPass (ERC-721) mint + validity/expiry reads, **on-chain only**
  (no backend). Per-chain address in `chain.Config.PassAddress` (empty = feature inert;
  deployed on `bsc_testnet` only — set `bsc`'s address after mainnet deploy). Surfaced as the
  TUI "Lazyswap Pass" tab (6); one-click mint reuses the in-session decrypted key.
- `internal/settings` — persisted chain/slippage/default-wallet, **shared by CLI and TUI**
- `internal/paths` — filesystem SSOT; `internal/applog` — file logger
- `internal/cli` — non-interactive commands; `internal/tui` — screens/panels/overlays/theme/keys

Most packages mirror a TS file (`// Mirrors src/...`, in `lazyswap-old/`). When changing
behavior, keep parity with the Bun reference unless intentionally diverging.

## Critical Rules

- **`internal/chain/config.go` `CHAINS` is the single source of truth** for RPC URLs, router
  and token addresses. Never hardcode chain-specific values elsewhere.
- **Outside `internal/cli`, never write to stdout/stderr** (`fmt.Print*`, `log`, `println`) —
  it corrupts the TUI. Use `internal/applog` (writes `~/.lazyswap/lazyswap.log`, never panics).
  The CLI prints to stdout/stderr on purpose; the TUI does not.
- **Filesystem paths only via `internal/paths`.** Data dir `~/.lazyswap/`, override with
  `LAZYSWAP_DATA_DIR`. `paths.Override` / `applog.SetPath` isolate tests; `LAZYSWAP_TEST=1`
  routes the log to `/dev/null`.
- Wallet/key handling stays in `internal/wallet` + `internal/crypto`; the private key is never
  logged or printed in plaintext.
- Never commit the compiled binary (`lazyswap` / `lazyswap-tui`), `dist/`, `*.db`, `*.log`, or `.claude/` (see `.gitignore`).
- Release version is injected via `-ldflags "-X .../internal/cli.version=..."`; default `"dev"`.
- Releases are driven by **semantic-release / Conventional Commits** (`.releaserc.json`): `master`
  → `beta` prereleases, `stable` → releases. The release binary is named `lazyswap`.

## Testing

- Behavior + data only. **No tests on `View()` / layout / ASCII art** — they churn.
- CLI env vars: `LAZYSWAP_PASSWORD` (skips prompt), `LAZYSWAP_DATA_DIR`.
- `lazyswap set password` prints an `export LAZYSWAP_PASSWORD=…` line for the caller to `eval`
  (a child process can't set the parent shell's env). It only emits the secret when stdout is
  captured (not a TTY), refusing on a bare terminal to avoid leaking it. See `internal/cli/setpassword.go`.
