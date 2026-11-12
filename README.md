# blockchain-insight

[![CI](https://github.com/mohsenm4/blockchain-insight/actions/workflows/ci.yml/badge.svg)](https://github.com/mohsenm4/blockchain-insight/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/mohsenm4/blockchain-insight)](https://goreportcard.com/report/github.com/mohsenm4/blockchain-insight)
[![Go Version](https://img.shields.io/github/go-mod/go-version/mohsenm4/blockchain-insight)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

A deposit and withdrawal backend for an ERC20 token on Ethereum (Sepolia), written in Go with Postgres — the core of a crypto exchange's wallet service.

It watches the chain for token transfers to users' deposit addresses, waits for confirmations, follows deposits through reorgs, and credits each deposit to the user's balance exactly once. Withdrawals take the amount from that balance, and they are idempotent: a retried request never pays twice.

This is a personal project used to practice production-grade Go. It started as a small block explorer, and the explorer endpoints are still here.

## How a deposit becomes money

```text
                 N blocks on top,          one DB transaction
  Transfer  ──►  pending  ──────────────►  confirmed  ──────────────►  credited
                    │      same block hash             + balances
                    │
                    └──► reverted   (reorg: the tx is in no block, or it failed)
```

1. **Watch** (`internal/watch`) — every 10 s, reads the ERC20 `Transfer` logs of Sepolia USDC, 5 blocks behind the head. A transfer to one of our deposit addresses (`POST /watch`) is saved as `pending`, with its block number and block hash. The deposits and the watcher's cursor are saved in one transaction, so after a restart the watcher goes on from the last block it finished, and no block is skipped. If the main node fails (for example, it has pruned old blocks), the archive node (`ARCHIVE_RPC_URL`) is asked.
2. **Confirm** (`internal/confirm`) — every 10 s, a `pending` deposit with `CONFIRMATIONS` blocks on top of it (default 12) is checked again: if its block hash is still the same, it becomes `confirmed`.
3. **Reorg** — if the block hash changed, the block was replaced. The tracker asks the node for the tx receipt:
   - the tx is in another block → the deposit moves there and counts its confirmations again;
   - the tx is in no block, or it failed → the deposit becomes `reverted`. If the tx comes back later, the watcher records it again and the `reverted` row is `pending` again.
4. **Credit** — `confirmed` → `credited`, and the amount is added to `balances`, in one DB transaction. The status change is `UPDATE … WHERE status = 'confirmed'`, so a deposit is credited once, even with two workers.
5. **Reversal** — a block can still be replaced until it is final (`FINALITY` blocks, default 64). Every 10 s, the credited deposits in that window are checked again. If the block hash changed and the transfer is not on the chain any more (the tx is in no block, failed, or the transfer is not in its new block), the credited row is **not edited**: a `deposit_reversals` row is added and the amount is taken back from `balances`, in one DB transaction, once per deposit. If the user has already spent it (balance too low), nothing is taken, the reversal is saved as `needs_review`, and an error is logged for a person to handle. If the transfer *is* in the new block, nothing changes, and the watcher does not record that copy again (it would be credited twice).

## Withdrawals

- `POST /send` needs an `idempotency_key`. There is one withdrawal row per `(user_id, idempotency_key)`, so the same key never signs a second tx.
- The amount comes out of `balances` in the same transaction that creates the withdrawal, with `UPDATE … WHERE amount >= x`, so two withdrawals at the same time can't spend the same money. Not enough balance → 422, and nothing is written. A `failed` withdrawal gives the amount back, once.
- States: `queued` → `signed` → `broadcast` → `confirmed`, or `failed` when no tx was sent or the tx failed on chain. The signed tx is saved **before** it is broadcast.
- If the broadcast fails or is cut off, the service asks the chain whether the tx left, and only ever resends the same signed tx (same nonce), so it pays at most once.
- The hot wallet's nonce comes from a nonce manager: it reads `PendingNonceAt` once, then counts up in memory under a mutex, and reads it again after a tx that may never reach the network.
- `GET /tx/:hash` returns `pending`, `mined` or `failed`, from the tx receipt.
- **Rate limit** — with `REDIS_URL` set, each user gets `SEND_LIMIT` requests to `POST /send` per minute (default 5). Over the limit → 429 with `Retry-After`, before any row is written. Retries with the same key count too.
- **Confirm** — every 10 s, each `broadcast` withdrawal's receipt is read. No receipt: not in a block yet, wait. Once its block has `CONFIRMATIONS` blocks on top: status 1 → `confirmed`; status 0 → `failed`, and the amount goes back to `balances` in the same DB transaction. A failed tx also waits for the confirmations, since after a reorg it could land again and succeed.

## Status

Actively developed. Runs against Sepolia. `v2.0.0` is the deposit and withdrawal pipeline above.

CI checks that `go.mod` is tidy, builds and vets with and without the `swagger` tag, checks that `docs/` matches the handlers, and runs golangci-lint. The tests run with the race detector and shuffled order against real Postgres 16 and Redis 7, after the migrations go up, all the way down, and up again; a skipped ledger or rate limit test fails the build, and so does coverage under the floor. It also runs govulncheck, builds the Docker image and starts it once. Every Monday it runs again, with the live Sepolia test.

## Endpoints

| Method | Path                        | Description                                                                 |
|--------|-----------------------------|-----------------------------------------------------------------------------|
| GET    | `/last/block`               | Latest block. Response is cached in memory.                                 |
| GET    | `/block/:id`                | Block by number.                                                            |
| GET    | `/balance/:address`         | ETH balance of an address in wei.                                           |
| POST   | `/watch`                    | Assign a deposit address to a user: `{"address", "user_id"}`. 409 if taken. |
| GET    | `/watch/:address/transfers` | Deposits recorded for an address, from Postgres. A reversed one shows `reversed` or `needs_review`. |
| POST   | `/send`                     | Send tokens: `{"user_id", "idempotency_key", "to", "amount"}`. Same key never signs a second tx (it may resend the first one if its state is unknown); 409 while in progress, 422 if reused for a different request or if the balance is too low, 429 over the rate limit. |
| GET    | `/tx/:hash`                 | Status of a sent transaction.                                               |
| GET    | `/swagger/*`                | Swagger UI for the API (only with the `swagger` build tag).                 |
| GET    | `/debug/pprof/*`            | Go profiling (only when `ENABLE_PPROF=true`).                               |

## Requirements

- Go 1.25 or newer
- Postgres 16 (`docker-compose.yml` has one) and [goose](https://github.com/pressly/goose) for the migrations
- Redis 7, for the `/send` rate limit (`docker-compose.yml` has one). Optional: without `REDIS_URL` there is no rate limit.
- An Ethereum **Sepolia** JSON-RPC endpoint (the watcher follows Sepolia USDC)
- A Sepolia hot wallet with test ETH and tokens, for `/send`

## Configuration

The server reads `app.env` from the project root (the file must exist). An environment variable with the same name overrides a key from the file — this is how Docker Compose points `DATABASE_URL` at the `postgres` container and `REDIS_URL` at the `redis` one.

```bash
cp app.env.example app.env
```

| Key               | Required   | What it is                                                                       |
|-------------------|------------|----------------------------------------------------------------------------------|
| `RPC_URL`         | yes        | Sepolia JSON-RPC endpoint                                                        |
| `PRIVATE_KEY`     | yes        | Hex private key of the hot wallet that signs withdrawals. **Testnet only.**      |
| `DATABASE_URL`    | yes        | Postgres connection string                                                       |
| `TOKEN_ADDRESS`   | for `/send`| The ERC20 contract that `/send` transfers                                        |
| `ARCHIVE_RPC_URL` | no         | Archive node, asked only when `RPC_URL` fails (for example, pruned old blocks)   |
| `TIMEOUT_SEC`     | no         | Seconds one RPC call may take (default 10). A node that takes the request and never answers gives an error after it, so the watcher and the trackers log it and retry (the watcher also asks the archive node), instead of waiting forever |
| `CONFIRMATIONS`   | no         | Blocks on top of a deposit before it is confirmed (default 12)                   |
| `FINALITY`        | no         | Blocks after which a credited deposit is no longer checked for a reorg (default 64) |
| `REDIS_URL`       | no         | Redis for the `/send` rate limit, e.g. `redis://localhost:6380/0`. Unset: no rate limit. Set but not reachable at start: the server exits. |
| `SEND_LIMIT`      | no         | `POST /send` requests per user per minute (default 5)                            |

Two settings are read only from the environment, not from `app.env`:

- `APP_ENV=production` — JSON logs at Info level. Anything else: text logs at Debug level, with source lines.
- `ENABLE_PPROF=true` — mounts `/debug/pprof/`.

## Run

```bash
docker compose up -d postgres
goose -dir migrations postgres "postgres://insight:insight@localhost:5433/ledger?sslmode=disable" up
go run ./cmd
```

The server listens on `:5050`. The watcher and the confirmation tracker start with it.

Example:

```bash
curl http://localhost:5050/last/block
curl http://localhost:5050/block/11898946

# give user 1 a deposit address, then list its deposits
curl -X POST http://localhost:5050/watch -H 'Content-Type: application/json' \
  -d '{"address": "<deposit address>", "user_id": 1}'
curl http://localhost:5050/watch/<deposit address>/transfers
```

## Swagger docs

Swagger UI is compiled in only when built with the `swagger` build tag. This keeps the default binary small and avoids shipping doc handlers to production.

```bash
# with Swagger UI at /swagger/index.html
go run -tags swagger ./cmd

# without (default)
go run ./cmd
```

## Run with Docker

Requires Docker and Docker Compose.

1. Create `app.env` and fill in `RPC_URL`, `PRIVATE_KEY` and `TOKEN_ADDRESS` (see [Configuration](#configuration)).
2. Start Postgres and run the migrations (Compose does not run them):

   ```bash
   docker compose up -d postgres
   goose -dir migrations postgres "postgres://insight:insight@localhost:5433/ledger?sslmode=disable" up
   ```

3. Build and start the server:

   ```bash
   docker compose up --build
   ```

The server listens on `http://localhost:5050`. To stop:

```bash
docker compose down
```

The `Makefile` also exposes `make docker-build`, `make docker-run`, and `make docker-stop` for running the image without Compose.

## Database

The ledger runs on Postgres 16 from `docker-compose.yml`. Data lives in the named volume `pgdata`, so it survives `docker compose down` (use `down -v` to wipe it).

The host port is **5433**, not 5432, so it does not clash with a local Postgres install.

Tables: `deposits`, `deposit_addresses`, `watcher_cursor`, `withdrawals`, `balances`, `deposit_reversals`. The design, the state machines and the reasons are in [docs/ledger-schema.md](docs/ledger-schema.md).

Migrations live in `migrations/` and run with [goose](https://github.com/pressly/goose):

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest

docker compose up -d postgres
goose -dir migrations postgres "postgres://insight:insight@localhost:5433/ledger?sslmode=disable" up
```

Inspect the schema:

```bash
docker compose exec postgres psql -U insight -d ledger -c '\d deposits'
```

Run the ledger tests against it:

```bash
TEST_DATABASE_URL="postgres://insight:insight@localhost:5433/ledger?sslmode=disable" go test ./internal/ledger/
```

## Redis

`docker-compose.yml` runs Redis 7 for the `/send` rate limit. The host port is **6380**, not 6379, so it does not clash with a local Redis. There is no volume: a counter lives one minute, so losing it on a restart is fine.

Run the rate limit tests against it:

```bash
docker compose up -d redis
TEST_REDIS_URL="redis://localhost:6380/0" go test ./internal/ratelimit/
```

## Project layout

```text
cmd/                 entry point: API server, watcher and confirmation tracker
cmd/tools/           small CLIs from building the pipeline (filter: print Transfer logs,
                     rawtx: build and sign a tx by hand, send: send tokens)
config/              viper-based config loader
internal/api/        HTTP server, handlers, cache middleware, watcher start
internal/watch/      polls ERC20 Transfer logs, falls back to an archive node
internal/confirm/    deposit confirmations, reorgs, crediting, reversals; withdrawal settling
internal/ledger/     Postgres: deposits, cursor, withdrawals, balances
internal/sender/     signs and broadcasts ERC20 transfers
internal/nonce/      nonce manager for the hot wallet
internal/ratelimit/  per-user request counter in Redis (fixed window)
internal/enth/       Ethereum client wrapper (go-ethereum)
internal/contracts/  ERC20 bindings (abigen)
internal/logging/    slog setup (JSON in production)
internal/models/     response types
internal/utils/      formatting helpers
docs/                Swagger files and the ledger schema
migrations/          goose SQL migrations
```

## Design notes

- **Money is never a float**: amounts are `NUMERIC(78,0)` in Postgres and `*big.Int` in Go.
- **Exactly once**: `UNIQUE (tx_hash, log_index)` for deposits and `UNIQUE (user_id, idempotency_key)` for withdrawals. Every status change is a conditional `UPDATE … WHERE status = …`, and the code checks how many rows it changed.
- **A balance is spent once**: the withdrawal row and the debit are one transaction, and the debit is `UPDATE balances … WHERE amount >= x`. A second withdrawal waits for the row lock, then checks the new amount.
- **Two facts, two statuses**: `confirmed` is a chain fact, `credited` is a money fact. Each step credits first, then confirms, so a deposit left `confirmed` by a crash is credited the next time.
- **Sign → save → broadcast**: the signed tx is in the database before it goes to the network, so after a crash the service knows which tx may have left.
- **Rate limit**: a fixed window per user. `INCR` counts, `EXPIRE … NX` starts the window at the first request, and both run in one `MULTI`, so a crash can't leave a counter without a TTL (that user would be blocked forever).
- **Cache**: `patrickmn/go-cache` in front of `/last/block`. TTL trades staleness for RPC cost.
- **Cache stampede**: guarded by `golang.org/x/sync/singleflight` so a single expired key does not fan out N concurrent RPC calls.
- **Testing**: the chain and the store are interfaces (`EthClient`, `watch.Source`, `confirm.Chain`, `confirm.Store`), so the logic is tested against fakes without a real node. The race detector runs on every CI build.

## Roadmap

- Withdrawals: detect a dropped tx (never mined, and its nonce taken by another tx). Now it stays `broadcast`.
- Redis: a lock for the withdrawal worker, and a "Redis is down" test (today a Redis error refuses `/send` with 503)
- An own Sepolia node (snap sync) with Prometheus and Grafana; this service uses it, with a fallback
- The watched token (Sepolia USDC) from config instead of code
- Docker Compose with everything: app, Postgres, Redis, node

## License

MIT
