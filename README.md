# stake-indexer

A Go indexer service for FIP-101 stake protocol parsing, mempool synchronization, and reward-state execution.

## Features

- Parse FIP-101 inscriptions from protocol transactions
- Maintain mempool snapshots and derived protocol events
- Execute fast/slow reward processing pipelines
- Expose HTTP query endpoints for indexer and staking state

## Requirements

- Go 1.25.10+
- PostgreSQL 14+
- Redis 6+
- Bitcoin-compatible node RPC access

## Quick Start

1. Prepare config files in `conf/`:
   - `config.yaml`
   - `chain.yaml`
   - `pg.yaml`
   - `rdb_balance.yaml`
   - `rdb_utxo.yaml`
2. Update endpoint/auth values for your environment.
3. Run:

```bash
go run .
```

## Deployment

Open-source deployment guidance is available in [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Test

```bash
go test ./...
```

## Configuration Notes

- `conf/` files are safe defaults for local/dev bootstrap and must be customized before production usage.
- Do not commit real credentials, private endpoints, or wallet secrets.

## Security

Please report vulnerabilities through the process described in `SECURITY.md`.

## Contributing

See `CONTRIBUTING.md`.

## License

MIT, see `LICENSE`.
