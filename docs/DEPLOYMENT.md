# Deployment Guide

This document describes how to deploy `stake-indexer`.

## Overview

`stake-indexer` is a Go service that:

- parses FIP-101 protocol activity from blocks and mempool transactions
- maintains indexer, staking, proof, and reward state
- stores state in PostgreSQL and Redis
- exposes HTTP query endpoints

At startup, the service:

1. initializes logging
2. loads `conf/config.yaml`
3. loads `conf/chain.yaml`
4. initializes the external state API client
5. initializes PostgreSQL and creates required schema objects
6. initializes Redis clients
7. resets mempool snapshot state
8. initializes cached status state
9. starts the HTTP server
10. starts block sync and reward sync loops

## Runtime Dependencies

The service requires:

- Go `1.25.10` for source-based builds and `go run`
- PostgreSQL
- two Redis connections configured through `conf/rdb_balance.yaml` and `conf/rdb_utxo.yaml`
- a Bitcoin RPC node

Reward allocation also requires a reachable external state API.

## Bitcoin RPC Requirements

The service uses Bitcoin RPC methods including:

- `getblockcount`
- `getblockhash`
- `getblock`
- `getrawtransaction`
- `getrawtxmempool`
- `getblockindexrange`

Protocol parsing and address validation use Bitcoin mainnet address rules.

## Configuration Files

The service reads configuration from fixed paths relative to the working directory:

- `conf/config.yaml`
- `conf/chain.yaml`
- `conf/pg.yaml`
- `conf/rdb_balance.yaml`
- `conf/rdb_utxo.yaml`

Run the service from the repository root, or provide the same `conf/` layout in the runtime working directory.

### `conf/config.yaml`

Primary runtime settings include:

- `index_start_height`
- `batch_block_count`
- `slow_lag_blocks`
- `proof_window`
- `delay_submit_trigger_blocks`
- `start_reward_height`
- `state_api_base_url`
- `state_api_auth`
- `state_api_timeout`
- `cold_wallet_address_keys`
- `indexer_allowlist_windows`
- `reward_release_tiers`

The values below describe runtime behavior. Example values in this section use `conf.DefaultConfig()` from [conf/init.go], not the sample file contents.

- `index_start_height`
  The first block height that the main indexer loop is allowed to process. On startup, the service checks the stored sync height, but if that height is lower than `index_start_height`, it will start from `index_start_height` instead.
  Example with default `1760000`: on a fresh deployment with no prior sync state, block sync begins at height `1760000`.

- `batch_block_count`
  The maximum number of blocks the manager pulls into one parse window when catching up from chain data.
  Example with default `500`: if the next unsynced height is `1760000`, one parse cycle can prepare at most the range `1760000` through `1760499` before submit logic takes over.

- `slow_lag_blocks`
  The number of committed main-indexer blocks that the reward snapshot consumer intentionally stays behind. The reward sync loop only consumes heights up to `latest_committed_sync_block_height - slow_lag_blocks`, so it never follows the main indexer all the way to the current committed tip.
  Example with default `20160`: if the latest committed block in `sync_blocks` is `1525000`, the reward snapshot loop will only advance up to height `1504840`. Heights above `1504840` are deferred until the main indexer has moved further ahead.

- `proof_window`
  The maximum block-distance after `prove_block_height` within which a proof transaction is still considered for that reward height. When rewards are computed for height `H`, the service loads proofs with `prove_block_height = H` and `proof_tx_height > H` and `proof_tx_height <= H + proof_window`.
  Example with default `20160`: for reward height `1500000`, the service only checks proofs submitted in blocks `1500001` through `1520160`. A proof for `prove_block_height = 1500000` first appearing at height `1520161` is ignored for that reward round.

- `delay_submit_trigger_blocks`
  The threshold used to mark an otherwise valid proof as delayed. If a proof's submission height is more than `delay_submit_trigger_blocks` blocks after its `prove_block_height`, it is marked `valid_delayed`, and the corresponding indexer receives a 5% effective-stake penalty during reward allocation.
  Example with default `120`: if a proof declares `prove_block_height = 1500000` and the proof transaction lands at height `1500100`, it is still treated as normal valid proof. If it lands at height `1500121`, it is marked delayed-valid and that indexer's effective stake is reduced to `95%` of raw stake for that reward calculation.

- `start_reward_height`
  The first block height from which reward allocation logic is allowed to produce reward records.
  Example with default `1764000`: even if older stake activity exists before `1764000`, reward allocation begins only from height `1764000`.

- `state_api_base_url`
  Base URL of the external state API used by reward-related workflows.

- `state_api_auth`
  Authentication token or credential string passed to the external state API client.

- `state_api_timeout`
  Request timeout for external state API calls.
  Default example: `5s`.

- `cold_wallet_address_keys`
  Optional list of configured cold-wallet address keys used by business logic that needs to recognize protected or reserved addresses.

- `indexer_allowlist_windows`
  Optional controlled-launch windows for indexer recognition. Each window has `start_height`, `end_height`, and `indexer_ids`. Windows are half-open: `start_height <= height < end_height`. When the current block height is inside a configured window, only listed indexer IDs are recognized; outside configured windows, all syntactically valid indexer IDs are recognized. This is intended for production launch phases with persisted state, not disposable test mode.

- `reward_release_tiers`
  Tiered release percentages keyed by block height. The effective release percent is the highest tier whose `start_height` is less than or equal to the current block height.
  Example with default tiers: at height `1764000` the release percent is `30`; at height `1844640` it becomes `60`; at height `1925280` or above it becomes `100`.

### `conf/chain.yaml`

Relevant fields include:

- `rpc`
- `rpc_auth`

`rpc_auth` is used as the `username:password` value for HTTP Basic authentication.

### `conf/pg.yaml`

PostgreSQL configuration supports either:

- a DSN through `dsn`
- structured connection fields through `host`, `port`, `user`, `password`, `dbname`, and `sslmode`

Connection pool settings include:

- `max_open_conns`
- `max_idle_conns`
- `conn_max_lifetime`
- `conn_max_idle_time`

If `dsn` is set, it takes precedence over the structured connection fields.

### `conf/rdb_balance.yaml` and `conf/rdb_utxo.yaml`

Each Redis config supports:

- `addrs`
- `useCluster`
- `database`
- `password`
- `dialTimeout`
- `readTimeout`
- `writeTimeout`
- `idleTimeout`
- `idleCheckFrequency`
- `poolSize`

`useCluster: true` enables Redis Cluster mode.

## Running from Source

### Prerequisites

Ensure the following services are reachable:

- PostgreSQL
- Redis
- Bitcoin RPC

Ensure the external state API is reachable when reward allocation is enabled.

### Start the Service

The HTTP listen address is read from the `LISTEN` environment variable.

Debug endpoints are disabled by default:

- set `ENABLE_METRICS=true` to expose `/metrics` and `/metrics/:time/:type/:stage`
- set `ENABLE_PPROF=true` to expose `/debug/pprof/*`

Do not expose debug endpoints directly to the public internet.

PowerShell:

```powershell
$env:LISTEN=":8080"
go run .
```

Bash:

```bash
LISTEN=:8080 go run .
```

### Build a Binary

```bash
go build -o stake_indexer .
LISTEN=:8080 ./stake_indexer
```

## Docker Deployment

The repository includes a `Dockerfile`.

The build stage uses `golang:1.25.10-alpine`. The runtime stage uses `alpine:3.18.4`.

Build the image:

```bash
docker build -t stake-indexer:latest .
```

The image copies only the compiled binary into `/data/stake_indexer`. Mount the `conf/` directory into `/data/conf` when running the container.

Example:

```bash
docker run --name stake-indexer \
  -e LISTEN=:8080 \
  -p 8080:8080 \
  -v $(pwd)/conf:/data/conf:ro \
  stake-indexer:latest
```

PowerShell:

```powershell
docker run --name stake-indexer `
  -e LISTEN=:8080 `
  -p 8080:8080 `
  -v ${PWD}\conf:/data/conf:ro `
  stake-indexer:latest
```

## Database Initialization

At startup, the service creates required PostgreSQL tables and indexes if they do not already exist.

The schema includes data for:

- proof records
- indexer registrations
- claimed rewards
- allocated rewards
- stake bindings
- synchronized blocks
- mempool protocol events
- FIP-101 inscription events
- rollback support

The configured PostgreSQL user must have permission to create tables and indexes.

## First Deployment Checklist

Before first startup, verify:

- the PostgreSQL database exists
- the PostgreSQL user has schema creation privileges
- Redis is reachable
- Bitcoin RPC is reachable
- `rpc_auth` is configured correctly
- `LISTEN` is set
- `index_start_height` is set
- `start_reward_height` is set
- public configuration files do not contain live credentials

When reward allocation is required, also verify:

- `state_api_base_url` is set
- the external state API is reachable

## Post-Startup Verification

After startup, verify these endpoints:

- `GET /indexer/status`
- `GET /stake-reward/sync-status`
- `GET /indexers`
- `GET /mempool/protocol-txs`

Example:

```bash
curl http://127.0.0.1:8080/indexer/status
```

Successful responses use the JSON envelope documented in [API.md](./API.md).

## Operational Constraints

Current runtime behavior includes:

- configuration file paths are fixed under `conf/`
- configuration is loaded at startup
- the container image does not include runtime config files
- PostgreSQL schema initialization runs at startup
- block and mempool processing depend on Bitcoin RPC
- reward allocation depends on the external state API


