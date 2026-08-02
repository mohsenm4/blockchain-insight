# blockchain-insight

A small Ethereum block explorer written in Go. It exposes a REST API that reads blocks and account balances from an Ethereum RPC node.

This is a personal project used to practice production-grade Go: clean packages, HTTP handlers, config loading, caching, and Swagger docs.

## Status

Early. The API is stable enough to run locally against a public RPC endpoint. Test coverage, CI, and Docker packaging are being added.

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

## Run with Docker

### Prerequisites
- Docker & Docker Compose installed

### Setup
1. Copy the example env file and fill in your values:
   ```bash
   cp cmd/app.env.example cmd/app.env


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

## Roadmap

- Transaction lookup endpoint (`GET /tx/:hash`)
- Structured logging
- Graceful shutdown on SIGTERM
- `go test -race` coverage above 80%
- GitHub Actions CI (lint + test + coverage)
- Docker + docker-compose for local development
- Benchmarks for the cache layer

## License

MIT
