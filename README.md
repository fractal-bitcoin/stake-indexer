# Stake Indexer

Stake Indexer is a Go service for indexing FIP-101 stake protocol activity from
Bitcoin blocks and mempool transactions. It parses protocol inscriptions,
maintains indexer, staking, proof, and reward state, and exposes HTTP endpoints
for querying the indexed data.

## Features

- Parse FIP-101 protocol inscriptions from confirmed blocks
- Track pending protocol activity from the mempool
- Maintain indexer registrations, stake bindings, proof records, and reward state
- Handle rollback checks and recent-chain reconciliation during block sync
- Store indexed state in PostgreSQL and Redis
- Expose HTTP APIs for indexers, stakers, rewards, proofs, sync status, and mempool events

## Requirements

- Go 1.25.10+
- PostgreSQL 14+
- Redis 6+
- Bitcoin-compatible RPC node
- External state API access for reward allocation workflows

## Quick Start

1. Review and update the configuration files in `conf/`:
   - `conf/config.yaml`
   - `conf/chain.yaml`
   - `conf/pg.yaml`
   - `conf/rdb_balance.yaml`
   - `conf/rdb_utxo.yaml`
2. Make sure PostgreSQL, Redis, the Bitcoin RPC node, and the external state API
   are reachable from the service.
3. Start the service from the repository root:

```bash
LISTEN=:8080 go run .
```

The service reads configuration from fixed paths under `conf/`, so run it from
the repository root or provide the same `conf/` layout in the runtime working
directory.

## Configuration

Core runtime settings live in `conf/config.yaml`, including:

- `index_start_height`
- `batch_block_count`
- `slow_lag_blocks`
- `proof_window`
- `delay_submit_trigger_blocks`
- `delay_submit_stage2_step_blocks`
- `delay_submit_stage2_step_percent`
- `delay_submit_stage3_blocks`
- `stage2_start_height`
- `stage3_start_height`
- `start_reward_height`
- `state_api_base_url`
- `state_api_auth`
- `reward_claim_sender_address_keys`
- `indexer_allowlist_windows`
- `reward_release_tiers`

Connection settings are split by dependency:

- `conf/chain.yaml`: Bitcoin RPC endpoint and authentication
- `conf/pg.yaml`: PostgreSQL DSN or structured connection fields
- `conf/rdb_balance.yaml`: Redis connection for balance state
- `conf/rdb_utxo.yaml`: Redis connection for UTXO state

The checked-in configuration files are bootstrap examples. Replace endpoints,
credentials, and network-specific values before production use.

## HTTP API

HTTP routes are registered at the root path. Common endpoints include:

- `GET /indexer/status`
- `GET /indexers`
- `GET /indexers/by-address/:address`
- `GET /indexers/:id/stakers`
- `GET /indexers/:id/proofs`
- `GET /users/:address/stakings`
- `GET /users/:address/rewards`
- `GET /stake-reward/sync-status`
- `GET /mempool/protocol-txs`

See [docs/API.md](docs/API.md) for response formats, pagination rules, and
endpoint details.

## Development

Run tests:

```bash
go test ./...
```

Build a local binary:

```bash
go build -o stake_indexer .
LISTEN=:8080 ./stake_indexer
```

Optional runtime endpoints:

- Set `ENABLE_METRICS=true` to expose metrics endpoints
- Set `ENABLE_PPROF=true` to expose `/debug/pprof/*`

Do not expose debug endpoints directly to the public internet.

## Docker

Build the image:

```bash
docker build -t stake-indexer:latest .
```

Run the container with configuration mounted at `/data/conf`:

```bash
docker run --name stake-indexer \
  -e LISTEN=:8080 \
  -p 8080:8080 \
  -v $(pwd)/conf:/data/conf:ro \
  stake-indexer:latest
```

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for deployment details.

## Project Layout

- `conf/`: runtime configuration and config loading
- `interfaces/http/`: HTTP response types and query handlers
- `internal/component/`: infrastructure clients and startup components
- `internal/entry/`: block, mempool, reward, and rollback entry points
- `internal/exec/manager/`: indexing manager and execution pipeline
- `internal/parser/`: chain and protocol parsing logic
- `lib/script/`: inscription and script parsing helpers
- `model/`: shared domain models
- `docs/`: API, deployment, protocol, and reward documentation

## Security

Do not commit real credentials, private endpoints, wallet secrets, or production
authorization tokens. Report vulnerabilities through the process described in
[SECURITY.md](SECURITY.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT, see [LICENSE](LICENSE).
