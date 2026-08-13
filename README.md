# blockchain-insight

[![CI](https://github.com/mohsenm4/blockchain-insight/actions/workflows/ci.yml/badge.svg)](https://github.com/mohsenm4/blockchain-insight/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/mohsenm4/blockchain-insight)](https://goreportcard.com/report/github.com/mohsenm4/blockchain-insight)
[![Go Version](https://img.shields.io/github/go-mod/go-version/mohsenm4/blockchain-insight)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

A small Ethereum block explorer written in Go. It exposes a REST API that reads blocks and account balances from an Ethereum RPC node.

This is a personal project used to practice production-grade Go: clean packages, HTTP handlers, config loading, caching, and Swagger docs.

## Status

Actively developed. Runs locally against a public RPC endpoint. Cached, race-tested, and covered by GitHub Actions CI. Test coverage is ~78% and moving toward 80%.

## Endpoints

| Method | Path                    | Description                                     |
|--------|-------------------------|-------------------------------------------------|
| GET    | `/last/block`           | Latest block. Response is cached in memory.     |
| GET    | `/block/:id`            | Block by number.                                |
| GET    | `/balance/:address`     | ETH balance of an address in wei.               |
| GET    | `/swagger/*`            | Swagger UI for the API.                         |

## Requirements

- Go 1.24 or newer
- An Ethereum JSON-RPC endpoint (Infura, Alchemy, or a local node)

## Configuration

The server reads configuration from an `app.env` file (via `viper`) and from environment variables.

Create `app.env` in the project root:

```env
RPC_URL=https://mainnet.infura.io/v3/YOUR_PROJECT_ID
NETWORK_NAME=mainnet
TIMEOUT_SEC=10
```

`RPC_URL` is required. The server will fail to start without it.

## Run

```bash
go run ./cmd
```

The server listens on `:5050`.

Example:

```bash
curl http://localhost:5050/last/block
curl http://localhost:5050/block/19000000
curl http://localhost:5050/balance/0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045
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

1. Copy the example env file and fill in your `RPC_URL`:

   ```bash
   cp cmd/app.env.example cmd/app.env
   ```

2. Build and start the container:

   ```bash
   docker compose up --build
   ```

The server listens on `http://localhost:5050`. To stop:

```bash
docker compose down
```

The `Makefile` also exposes `make docker-build`, `make docker-run`, and `make docker-stop` for running the image without Compose.

## Project layout

```text
cmd/            entry point
config/         viper-based config loader
internal/api/   HTTP server, handlers, cache middleware
internal/enth/  Ethereum client wrapper (go-ethereum)
internal/models/ response types
internal/utils/ formatting helpers
docs/           generated Swagger files
```

## Design notes

- **Cache**: `patrickmn/go-cache` in front of `/last/block`. TTL trades staleness for RPC cost.
- **Cache stampede**: guarded by `golang.org/x/sync/singleflight` so a single expired key does not fan out N concurrent RPC calls.
- **Testing**: `EthClient` is an interface consumed by the API layer, so handlers are tested against a stub without hitting a real node. Race detector runs on every CI build.

## Roadmap

- Transaction lookup endpoint (`GET /tx/:hash`)
- Structured logging with `slog` (JSON in prod, text in dev)
- Graceful shutdown on `SIGTERM`
- Push coverage above 80%
- Benchmarks for the cache layer

## License

MIT
