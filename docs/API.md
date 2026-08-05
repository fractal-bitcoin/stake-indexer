# HTTP API Reference

This document describes the HTTP endpoints exposed by `stake-indexer`.

## Overview

The service exposes HTTP query endpoints for:

- indexer discovery
- stake and proof inspection
- user staking and reward queries
- reward synchronization status
- mempool protocol event inspection

All routes are registered at the root path.

## Response Format

All endpoints return HTTP `200 OK` with a unified JSON envelope:

```json
{
  "code": 0,
  "msg": "",
  "data": {}
}
```

Fields:

- `code`: application-level status code
- `msg`: error message when `code != 0`
- `data`: response payload on success, and may be omitted for error cases

## Application Error Codes

The implementation uses the following common application codes:

- `0`: success
- `1001`: invalid request parameters
- `1002`: resource not found
- `1003`: internal error

## Pagination

Endpoints with paginated responses use query parameters:

- `start`: zero-based offset, default `0`
- `limit`: page size, default `20`, maximum `500`

If `limit > 500` or `start < 0`, the request fails with `code = 1001`.

## Endpoints

### `GET /indexer/status`

Returns a high-level service and indexing status summary.

#### Response

`data` is an object with:

- `total_indexers`
- `total_staked`
- `latest_block_height`
- `stake_reward_sync_height`
- `latest_allocated_reward_height`
- `latest_allocated_reward_amount`

Notes:

- `total_staked` may include applied mempool deltas when cached status is available.
- `latest_allocated_reward_amount` uses the latest persisted `sync_blocks.coinbase_reward`.

### `GET /indexers`

Returns indexers sorted by displayed total staked amount in descending order.

#### Query Parameters

- `start`
- `limit`

#### Response

`data` contains:

- `total`: total number of returned indexer entries after combining confirmed and pending-only entries
- `total_staked`: aggregate displayed total staked amount across confirmed indexers after applying mempool indexer deltas
- `start`
- `detail`

Each `detail` item may include:

- `indexer_id`
- `name`
- `reward_address`
- `user_address`
- `index_ratio`
- `total_staked`
- `stake_ratio`
- `allocated_reward`
- `pending`

Notes:

- Confirmed indexers may include `pending: true` when mempool stake deltas affect the displayed totals.
- Pending-only mempool register events may appear as temporary entries.

### `GET /indexers/by-address/:address`

Returns the indexer associated with an indexer owner address.

#### Path Parameters

- `address`: target indexer owner address

#### Response Behavior

- If a confirmed indexer register exists for the address as `user_address`, the confirmed indexer is returned.
- If no confirmed record exists and a pending mempool register exists, a temporary pending indexer is returned.
- If neither exists, the endpoint returns `code = 1002` with `msg = "indexer not found"`.

### `GET /indexers/:id/stakers`

Returns staker entries for a given indexer.

#### Path Parameters

- `id`: indexer identifier

#### Query Parameters

- `start`
- `limit`

#### Response

`data` contains:

- `indexer_id`
- `name`
- `total`
- `start`
- `detail`

Each `detail` item includes:

- `user_address`
- `amount`
- `pending`

Notes:

- If a mempool staker snapshot exists for the indexer, the endpoint serves that snapshot.
- `pending: true` indicates that the displayed user amount is affected by mempool state.

### `GET /indexers/:id/proofs`

Returns proof records for a given indexer.

#### Path Parameters

- `id`: indexer identifier

#### Query Parameters

- `start`
- `limit`

#### Response

`data` contains:

- `indexer_id`
- `name`
- `total`
- `start`
- `detail`

Each `detail` item may include:

- `indexer_id`
- `prove_block_height`
- `prove_data_hash`
- `txid`
- `height`
- `tx_idx`
- `verify_status`
- `pending`

Notes:

- Proof records are returned with pending mempool proof entries first, followed by confirmed `stake_proofs` entries.
- `pending: true` marks an unconfirmed proof entry sourced from `stake_mempool_events`.
- `verify_status` uses the stored table value for confirmed rows, and `0` for pending mempool rows.

### `GET /users/:address/stakings`

Returns staking positions for a user address.

#### Path Parameters

- `address`: user address

#### Query Parameters

- `start`
- `limit`

#### Response

`data` contains:

- `total`
- `start`
- `total_rewards`
- `detail`

Each `detail` item includes:

- `indexer_id`
- `name`
- `stake_address`
- `amount`
- `rewards`
- `pending`

Notes:

- The list includes only entries with effective amount greater than zero.
- Pending first-bind mempool stake entries may appear before a confirmed binding exists.
- `pending: true` indicates that the displayed position is influenced by mempool state.

### `GET /users/:address/rewards`

Returns persisted reward allocation records for a user address.

#### Path Parameters

- `address`: user address

#### Query Parameters

- `start`
- `limit`

#### Response

`data` contains:

- `total`
- `start`
- `detail`

Each `detail` item includes:

- `user_address`
- `indexer_id`
- `stake_address`
- `reward_type`
- `height`
- `stake_amount_snapshot`
- `indexer_total_stake`
- `indexer_effective_percent`
- `stake_amount_effective`
- `platform_total_stake`
- `total_effective_stake`
- `release_percent`
- `block_reward_amount`
- `indexer_ratio`
- `allocate_amount`

Notes:

- Records are returned from persisted allocation data.
- Records are ordered by `height DESC, id DESC`.

### `GET /users/:address/reward-summary`

Returns the reward balance used to decide whether a payout may be signed.

#### Path Parameters

- `address`: reward recipient address

#### Query Parameters

- `amount` (optional): positive integer amount to validate for payout signing

#### Response

`data` contains:

- `user_address`
- `allocated_amount`: total persisted reward allocation for the address
- `claimed_amount`: total amount from confirmed claim transactions
- `pending_claimed_amount`: total amount from valid unconfirmed claim transactions
- `claimable_amount`: `max(allocated_amount - claimed_amount - pending_claimed_amount, 0)`
- `requested_amount`: the validated `amount` query parameter, or `0` when omitted
- `can_claim`: when `amount` is supplied, whether it is no greater than `claimable_amount`; otherwise whether any amount is claimable

The endpoint includes valid mempool claims in the deduction so a payout already awaiting confirmation is unavailable. It is not an atomic payout reservation; the signer must serialize or otherwise make payout requests idempotent per recipient.

### `GET /reward-pool/status`

Returns reward-pool accounting data.

`data` contains:

- `allocated_amount`: all persisted reward allocations
- `claimed_amount`: total confirmed reward payouts
- `pending_claimed_amount`: total valid unconfirmed reward payouts
- `required_reserve_amount`: `max(allocated_amount - claimed_amount, 0)`
- `post_pending_required_reserve_amount`: reserve required after pending payouts confirm

### `GET /stake-reward/sync-status`

Returns the slow-path reward synchronization status.

#### Response

`data` contains:

- `height`
- `block_reward`
- `block_hash`

Notes:

- If no reward sync height has been recorded, the endpoint returns zero values.

### `GET /mempool/protocol-txs`

Returns stored mempool protocol event snapshots from `stake_mempool_events`.

#### Query Parameters

- `start`
- `limit`
- `op`
- `indexer_id`
- `user_address`
- `reward_address`

#### Supported `op` Values

The implementation accepts:

- empty value
- `FIP_101_REGISTER`
- `FIP_101_STAKE`
- `FIP_101_PROVE_STAKE`
- `FIP_101_PLEDEGED_REWARD`
- `FIP_101_ALLOCAT_RATIO`

Any other value returns `code = 1001`.

#### Response

`data` contains:

- `total`
- `start`
- `detail`

Each `detail` item may include:

- `txid`
- `op`
- `height`
- `inscription_content`
- `indexer_id`
- `user_address`
- `reward_address`
- `stake_address`
- `amount`
- `index_ratio`
- `indexer_name`
- `prove_block_height`
- `prove_data_hash`
- `tx_idx`

Notes:

- This endpoint reads from `stake_mempool_events`.
- Only rows with `biz_invalid_flags = 0` are returned.

## Pending Semantics

Some endpoints include a `pending` field to indicate unconfirmed mempool influence.

Endpoints using `pending` include:

- `GET /indexers`
- `GET /indexers/by-address/:address`
- `GET /indexers/:id/stakers`
- `GET /indexers/:id/proofs`
- `GET /users/:address/stakings`

Interpretation by endpoint:

- for indexers, pending stake or pending register state may affect the displayed entry
- for stakers and user staking positions, pending mempool deltas may affect the displayed amount
- for proofs, pending indicates a mempool proof record
