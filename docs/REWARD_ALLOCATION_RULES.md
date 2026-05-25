# Reward Allocation Rules

This document describes the reward allocation behavior implemented in this repository.

## Overview

Reward allocation runs during slow-path block processing.

For each processed block, the reward flow:

1. checks whether the block is reward-eligible
2. resolves proof validity for the reward height
3. derives eligible indexers and proof penalty flags
4. computes the reward release percentage from block-height tiers
5. allocates unlocked reward across eligible indexers
6. splits each indexer allocation between the indexer owner and stakers
7. persists allocation records and status fields

## Reward Eligibility

Reward processing continues only when all of the following are true:

1. `block.Height >= start_reward_height`
2. the block version satisfies the reward block version rule
3. the selected reward amount is greater than zero
4. at least one proof resolves as valid for the reward height
5. at least one eligible indexer has non-zero effective stake

The selected reward amount is `block.CoinbaseReward`.

## Proof Resolution

Proof validity is resolved for the current reward height with:

- the block hash of the reward height
- the state hash returned by the configured external state API
- `proof_window`
- `delay_submit_trigger_blocks`

### Proof Selection Window

For reward height `h`, the proof query includes rows where:

- `prove_block_height = h`
- `height > h`
- `height <= h + proof_window`

### Grouping

Proofs are grouped by:

- `indexer_id`
- `prove_data_hash`

Within each group, the canonical proof is the earliest row ordered by:

1. lower `height`
2. lower `tx_idx`
3. lower `txid`

## Expected Proof Hash

The expected proof hash for an indexer is:

```text
sha256(lower(indexer_id) + ":" + lower(block_hash) + ":" + lower(state_hash))
```

## Proof Status

The proof model uses these statuses:

- `Pending`
- `Valid`
- `InvalidHash`
- `InvalidDuplicate`
- `ValidDelayed`

Reward-time verification writes:

- `Valid`
- `InvalidHash`
- `InvalidDuplicate`
- `ValidDelayed`

### Status Rules

For each `(indexer_id, prove_data_hash)` group:

- when the canonical proof hash matches the expected proof hash, the canonical row becomes `Valid` or `ValidDelayed`
- when the canonical proof hash matches the expected proof hash, later rows in the same group become `InvalidDuplicate`
- when the canonical proof hash does not match the expected proof hash, all rows in the group become `InvalidHash`

### Delayed Proof Rule

An otherwise valid proof becomes `ValidDelayed` when:

```text
delay_submit_trigger_blocks > 0
```

and:

```text
proof.Height - proof.ProveBlockHeight > delay_submit_trigger_blocks
```

Resolved proof status is stored in `stake_proofs.verify_status`.

## Eligible Indexers

An indexer is eligible for reward allocation at a reward height when it has at least one resolved proof with status `Valid` or `ValidDelayed`.

For each eligible indexer, the penalty flag is:

- `true` when any valid proof for the indexer is `ValidDelayed`
- `false` otherwise

## Stake Weights

For each eligible indexer:

- `raw stake` is the sum of participating user stake amounts for that indexer
- `effective stake` is the value used for first-layer reward allocation

Penalty rule:

- in phase 1, when the indexer penalty flag is `true`, `effective_stake = raw_stake * 95 / 100`
- in phase 2, when the indexer penalty flag is `true`, `effective_stake` decreases by 10% of `raw_stake` for every full 100 delayed blocks, down to 0%
- otherwise, `effective_stake = raw_stake`

Indexers with `effective_stake = 0` do not participate in allocation.

## Reward Release

Reward release is computed before per-indexer allocation.

The release percentage is derived from `reward_release_tiers` by block height. The effective release percent is the highest configured tier whose `start_height` is less than or equal to the reward height.

The release percentage is capped at `100`.

The latest release percentage is exposed through `reward_release_percent` in the stake indexer status hash.

Unlocked reward is:

```text
unlocked_reward = reward_amount * release_percent / 100
```

When `unlocked_reward = 0`, no reward records are written for the block.

## Allocation Formula

### Phase Switch

Reward allocation has two phases separated by
`REWARD_ALLOCATION_STAGE2_CHECKPOINT_HEIGHT` in `constant/constant.go`.

- phase 1 (`height < checkpoint`): uses `round(...)`
- phase 2 (`height >= checkpoint`): uses decimal truncation (floor) to avoid over-allocation
- proof settlement window switches at the same checkpoint:
  - phase 1: use configured `proof_window`
  - phase 2: fixed `REWARD_ALLOCATION_STAGE2_PROOF_WINDOW` (`1000`)

### First-Layer Allocation

For each eligible indexer:

```text
first_layer_reward_i = quantize(unlocked_reward * effective_stake_i / total_effective_stake)
```

### Indexer Share

The indexer commission ratio is read from the snapshot view for the processing height.

For each eligible indexer:

```text
indexer_reward = quantize(first_layer_reward_i * index_ratio_snapshot)
user_reward_pool = first_layer_reward_i - indexer_reward
```

If rounding produces `indexer_reward > first_layer_reward_i`, the value is capped to `first_layer_reward_i`.

### Staker Distribution

Staker rewards are distributed by raw stake share within the indexer:

```text
user_reward_u = quantize(user_reward_pool * stake_u / raw_stake_i)
```

## Ratio Snapshot Timing

During slow-path processing, valid `commission_rate` inscription events are applied to the snapshot state before reward allocation for the same processing height.

## Persistence

### Proof Status

Proof verification results are stored in:

- `stake_proofs.verify_status`

### Reward Records

Reward allocation rows are stored in:

- `stake_allocated_rewards`

The uniqueness key is:

```text
(height, indexer_id, stake_address)
```

The stored reward types are:

- `indexer`
- `stake`

For indexer reward rows, `stake_address` is set to `indexer_id`.

### Status Fields

When reward rows are written, the service also updates:

- `latest_allocated_reward_height`
- `latest_allocated_reward_amount`

in the stake indexer status hash.
