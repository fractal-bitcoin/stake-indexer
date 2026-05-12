package pgdb

import (
	"context"
	"database/sql"
	"fmt"
)

// StakeBinding binds a user address + indexer id to a unique stake address.
// It is written by syncIndexer (fast path) when a valid stake tx is seen.
type StakeBinding struct {
	UserAddress  string
	IndexerID    string
	AddressType  string
	StakeAddress string
	Height       uint32
	StakeTxID    string
	StakeTxIdx   uint32
}

func InsertStakeBinding(ctx context.Context, item StakeBinding) error {
	if StakeDB == nil {
		return nil
	}
	return insertStakeBinding(ctx, StakeDB, item)
}

func InsertStakeBindingTx(ctx context.Context, tx *sql.Tx, item StakeBinding) error {
	if tx == nil {
		return InsertStakeBinding(ctx, item)
	}
	return insertStakeBinding(ctx, tx, item)
}

func insertStakeBinding(ctx context.Context, execer stakeExecer, item StakeBinding) error {
	if execer == nil {
		return nil
	}

	const sqlText = `
INSERT INTO stake_bindings (
    stake_address, user_address, indexer_id, address_type,
    height, txid, tx_idx
) VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (stake_address) DO UPDATE
SET
    height = EXCLUDED.height,
    txid = EXCLUDED.txid,
    tx_idx = EXCLUDED.tx_idx
WHERE stake_bindings.user_address = EXCLUDED.user_address
  AND stake_bindings.indexer_id = EXCLUDED.indexer_id
  AND stake_bindings.address_type = EXCLUDED.address_type
`
	res, err := execer.ExecContext(
		ctx,
		sqlText,
		item.StakeAddress,
		item.UserAddress,
		item.IndexerID,
		item.AddressType,
		item.Height,
		item.StakeTxID,
		item.StakeTxIdx,
	)
	if err != nil {
		return fmt.Errorf("insert stake binding failed: %w", err)
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return fmt.Errorf("insert stake binding conflict: stake_address=%s indexer_id=%s user_address=%s address_type=%s", item.StakeAddress, item.IndexerID, item.UserAddress, item.AddressType)
	}
	return nil
}

func ListStakeBindingsRange(ctx context.Context, startHeightExclusive, endHeightInclusive uint32) ([]StakeBinding, error) {
	if StakeDB == nil {
		return nil, nil
	}
	if endHeightInclusive <= startHeightExclusive {
		return nil, nil
	}

	const sqlText = `
SELECT
    user_address, indexer_id, address_type, stake_address,
    height, txid, tx_idx
FROM stake_bindings
WHERE height > $1 AND height <= $2
ORDER BY height ASC, tx_idx ASC, stake_address ASC
`
	rows, err := StakeDB.QueryContext(ctx, sqlText, startHeightExclusive, endHeightInclusive)
	if err != nil {
		return nil, fmt.Errorf("query stake bindings range failed: %w", err)
	}
	defer rows.Close()

	result := make([]StakeBinding, 0, 64)
	for rows.Next() {
		var item StakeBinding
		var h int64
		var txIdx int64
		if err := rows.Scan(&item.UserAddress, &item.IndexerID, &item.AddressType, &item.StakeAddress, &h, &item.StakeTxID, &txIdx); err != nil {
			return nil, fmt.Errorf("scan stake binding failed: %w", err)
		}
		if h < 0 {
			h = 0
		}
		if txIdx < 0 {
			txIdx = 0
		}
		item.Height = uint32(h)
		item.StakeTxIdx = uint32(txIdx)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake bindings failed: %w", err)
	}
	return result, nil
}

func ListStakeBindingsUpToHeight(ctx context.Context, height uint32) ([]StakeBinding, error) {
	if StakeDB == nil {
		return nil, nil
	}

	const sqlText = `
SELECT
    user_address, indexer_id, address_type, stake_address,
    height, txid, tx_idx
FROM stake_bindings
WHERE height <= $1
ORDER BY height ASC, tx_idx ASC, stake_address ASC
`
	rows, err := StakeDB.QueryContext(ctx, sqlText, height)
	if err != nil {
		return nil, fmt.Errorf("query stake bindings up to height failed: %w", err)
	}
	defer rows.Close()

	result := make([]StakeBinding, 0, 256)
	for rows.Next() {
		var item StakeBinding
		var h int64
		var txIdx int64
		if err := rows.Scan(&item.UserAddress, &item.IndexerID, &item.AddressType, &item.StakeAddress, &h, &item.StakeTxID, &txIdx); err != nil {
			return nil, fmt.Errorf("scan stake binding failed: %w", err)
		}
		if h < 0 {
			h = 0
		}
		if txIdx < 0 {
			txIdx = 0
		}
		item.Height = uint32(h)
		item.StakeTxIdx = uint32(txIdx)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake bindings failed: %w", err)
	}
	return result, nil
}

func ListStakeBindingsByIndexerID(ctx context.Context, indexerID string) ([]StakeBinding, error) {
	if StakeDB == nil {
		return nil, nil
	}
	const sqlText = `
SELECT
    user_address, indexer_id, address_type, stake_address,
    height, txid, tx_idx
FROM stake_bindings
WHERE indexer_id = $1
ORDER BY height ASC, tx_idx ASC, stake_address ASC
`
	rows, err := StakeDB.QueryContext(ctx, sqlText, indexerID)
	if err != nil {
		return nil, fmt.Errorf("query stake bindings by indexer id failed: %w", err)
	}
	defer rows.Close()

	result := make([]StakeBinding, 0, 64)
	for rows.Next() {
		var item StakeBinding
		var h int64
		var txIdx int64
		if err := rows.Scan(&item.UserAddress, &item.IndexerID, &item.AddressType, &item.StakeAddress, &h, &item.StakeTxID, &txIdx); err != nil {
			return nil, fmt.Errorf("scan stake binding failed: %w", err)
		}
		if h < 0 {
			h = 0
		}
		if txIdx < 0 {
			txIdx = 0
		}
		item.Height = uint32(h)
		item.StakeTxIdx = uint32(txIdx)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake bindings failed: %w", err)
	}
	return result, nil
}

func ListStakeBindingsByUserAddress(ctx context.Context, userAddress string) ([]StakeBinding, error) {
	if StakeDB == nil {
		return nil, nil
	}
	const sqlText = `
SELECT
    user_address, indexer_id, address_type, stake_address,
    height, txid, tx_idx
FROM stake_bindings
WHERE user_address = $1
ORDER BY height ASC, tx_idx ASC, stake_address ASC
`
	rows, err := StakeDB.QueryContext(ctx, sqlText, userAddress)
	if err != nil {
		return nil, fmt.Errorf("query stake bindings by user address failed: %w", err)
	}
	defer rows.Close()

	result := make([]StakeBinding, 0, 64)
	for rows.Next() {
		var item StakeBinding
		var h int64
		var txIdx int64
		if err := rows.Scan(&item.UserAddress, &item.IndexerID, &item.AddressType, &item.StakeAddress, &h, &item.StakeTxID, &txIdx); err != nil {
			return nil, fmt.Errorf("scan stake binding failed: %w", err)
		}
		if h < 0 {
			h = 0
		}
		if txIdx < 0 {
			txIdx = 0
		}
		item.Height = uint32(h)
		item.StakeTxIdx = uint32(txIdx)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake bindings failed: %w", err)
	}
	return result, nil
}
