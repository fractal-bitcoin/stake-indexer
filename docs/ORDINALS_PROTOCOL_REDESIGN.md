# FIP-101 Ordinals Parsing and Event Semantics

This document describes the FIP-101 inscription parsing and event normalization implemented in this repository.

## Parsing Scope

The parser:

- scans transaction witness data
- extracts one supported inscription from a Taproot witness script
- derives actor identity from the tapscript prefix
- parses the inscription body as an FIP-101 CSV record
- converts accepted input into a normalized `FIP101InscriptionEvent`

Transactions that do not satisfy the parser rules do not produce a parsed FIP-101 event.

## Transaction Acceptance Rules

A transaction produces a parsed event only when all of the following are true:

1. at least one input contains witness data
2. exactly one supported inscription is found across inspected witnesses
3. the inscription content type is `text/plain;charset=utf-8`
4. the inscription body is valid UTF-8
5. the inscription body is a single CSV record with no `\n` or `\r`
6. the CSV record matches a supported FIP-101 operation schema
7. the tapscript prefix contains a valid actor public key and address type

If more than one supported inscription is found, the transaction does not produce a parsed event.

## Inscription Envelope Rules

For a candidate tapscript:

- exactly one inscription payload must be extracted
- inscriptions flagged as unrecognized-even are ignored
- inscriptions flagged as duplicate-field are ignored
- inscriptions flagged as incomplete-field are ignored
- empty inscription bodies are ignored

## Body Format

Accepted inscription bodies use this CSV prefix:

```text
fip101,1,<op>,...
```

Prefix columns:

- column `0` is the protocol name and must be `fip101`
- column `1` is the FIP-101 CSV protocol version and must be `1`
- column `2` is the operation name

Version `1` is the only version currently accepted by this implementation. It defines
the operation names, column order, and validation rules described below.

Validation rules:

- column `0` is `fip101`
- column `1` is protocol version `1`
- column `2` is one of the supported operation names
- the record contains the exact column count for the selected operation

## Actor Identity

Actor identity is derived from the beginning of the tapscript:

1. the first pushed element is a 32-byte x-only public key
2. the next opcode is `OP_CHECKSIGVERIFY`
3. the next pushed value is a one-byte address type in the range `1..8`

The parser derives these normalized fields:

- `actor_pubkey`
- `actor_address`
- `address_type`

## Supported Operations

Supported operation names:

- `register_indexer`
- `stake`
- `submit_proof`
- `commission_rate`
- `claim`

## Operation Schemas

### `register_indexer`

Format:

```text
fip101,1,register_indexer,<index_ratio_bp>,<reward_addr>,<name>
```

Rules:

- exactly 6 columns
- `index_ratio_bp` is an unsigned integer in the range `0..10000`
- `reward_addr` is a valid Bitcoin mainnet address accepted by the parser
- `name` is non-empty after trimming
- `name` is truncated to at most 64 runes

Normalized fields:

- `user_address`: actor address derived from the tapscript
- `reward_address`: `reward_addr`
- `indexer_name`: normalized `name`
- `index_ratio`: `index_ratio_bp / 10000`

`indexer_id` is assigned during event application from `<block_height>:<tx_index>`.

### `commission_rate`

Format:

```text
fip101,1,commission_rate,<indexer_id>,<index_ratio_bp>
```

Rules:

- exactly 5 columns
- `indexer_id` is non-empty
- `index_ratio_bp` is an unsigned integer in the range `0..10000`

Normalized fields:

- `user_address`: actor address derived from the tapscript
- `indexer_id`
- `index_ratio`: `index_ratio_bp / 10000`

### `submit_proof`

Format:

```text
fip101,1,submit_proof,<indexer_id>,<prove_height>,<prove_hash>
```

Rules:

- exactly 6 columns
- `indexer_id` is non-empty
- `prove_height` is a valid unsigned integer
- `prove_hash` decodes to exactly 32 bytes

Normalized fields:

- `user_address`: actor address derived from the tapscript
- `indexer_id`
- `prove_block_height`
- `prove_data_hash`

### `stake`

Format:

```text
fip101,1,stake,<indexer_id>
```

Rules:

- exactly 4 columns
- `indexer_id` is non-empty

Normalized fields:

- `user_address`: actor address derived from the tapscript
- `indexer_id`
- `stake_address`: the first non-`OP_RETURN` output address in the transaction
- `amount`: the value of that output

### `claim`

Format:

```text
fip101,1,claim
```

Rules:

- exactly 3 columns

Normalized fields:

- `user_address`: the first spendable output address in the transaction
- `amount`: the value of that output

## Normalized Event Fields

Accepted inscriptions are converted into `FIP101InscriptionEvent` records. Depending on the operation, populated fields include:

- `txid`
- `op`
- `height`
- `tx_idx`
- `inscription_content`
- `user_address`
- `indexer_id`
- `reward_address`
- `indexer_name`
- `index_ratio`
- `prove_block_height`
- `prove_data_hash`
- `stake_address`
- `amount`

## Parsing Result

Successful parsing produces a normalized inscription event.

Later processing stages may mark the event with business validation flags in `biz_invalid_flags`.
