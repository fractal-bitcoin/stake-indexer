package pgdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type StakeIndexerRegister struct {
	IndexerID        string
	Name             string
	RewardAddress    string
	UserAddress      string
	IndexRatio       float64
	LastUpdateHeight uint32
	RegisterTxID     string
	Height           uint32
	TxIdx            uint32
}

func UpsertStakeIndexerRegister(ctx context.Context, reg StakeIndexerRegister) error {
	if StakeDB == nil {
		return nil
	}

	return upsertStakeIndexerRegister(ctx, StakeDB, reg)
}

func UpsertStakeIndexerRegisterTx(ctx context.Context, tx *sql.Tx, reg StakeIndexerRegister) error {
	if tx == nil {
		return UpsertStakeIndexerRegister(ctx, reg)
	}

	return upsertStakeIndexerRegister(ctx, tx, reg)
}

func upsertStakeIndexerRegister(ctx context.Context, execer stakeExecer, reg StakeIndexerRegister) error {
	if execer == nil {
		return nil
	}

	reg.UserAddress = strings.TrimSpace(reg.UserAddress)

	const sqlText = `
INSERT INTO stake_indexer_registers (
    indexer_id, name, reward_address, user_address, index_ratio,
    last_update_height, txid, height, tx_idx
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (indexer_id) DO UPDATE
SET
    name = EXCLUDED.name,
    reward_address = EXCLUDED.reward_address,
    user_address = EXCLUDED.user_address,
    index_ratio = EXCLUDED.index_ratio,
    last_update_height = EXCLUDED.last_update_height,
    txid = EXCLUDED.txid,
    height = EXCLUDED.height,
    tx_idx = EXCLUDED.tx_idx
`
	_, err := execer.ExecContext(
		ctx,
		sqlText,
		reg.IndexerID,
		reg.Name,
		reg.RewardAddress,
		reg.UserAddress,
		reg.IndexRatio,
		reg.LastUpdateHeight,
		reg.RegisterTxID,
		reg.Height,
		reg.TxIdx,
	)
	if err != nil {
		return fmt.Errorf("upsert stake indexer register failed: %w", err)
	}
	return nil
}

func ListStakeIndexerRegistersByRewardAddress(ctx context.Context, address string) ([]StakeIndexerRegister, error) {
	if StakeDB == nil {
		return nil, nil
	}
	const sqlText = `
SELECT
    indexer_id, name, reward_address, user_address, index_ratio,
    last_update_height, txid, height, tx_idx
FROM stake_indexer_registers
WHERE reward_address = $1
ORDER BY height ASC, tx_idx ASC
`
	rows, err := StakeDB.QueryContext(ctx, sqlText, address)
	if err != nil {
		return nil, fmt.Errorf("query stake indexer registers by reward address failed: %w", err)
	}
	defer rows.Close()

	result := make([]StakeIndexerRegister, 0, 8)
	for rows.Next() {
		var reg StakeIndexerRegister
		if err := rows.Scan(
			&reg.IndexerID,
			&reg.Name,
			&reg.RewardAddress,
			&reg.UserAddress,
			&reg.IndexRatio,
			&reg.LastUpdateHeight,
			&reg.RegisterTxID,
			&reg.Height,
			&reg.TxIdx,
		); err != nil {
			return nil, fmt.Errorf("scan stake indexer register failed: %w", err)
		}
		result = append(result, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake indexer registers by reward address failed: %w", err)
	}
	return result, nil
}

func GetStakeIndexerRegisterByUserAddress(ctx context.Context, userAddress string) (*StakeIndexerRegister, error) {
	if StakeDB == nil {
		return nil, nil
	}
	userAddress = strings.TrimSpace(userAddress)
	if userAddress == "" {
		return nil, nil
	}

	const sqlText = `
SELECT
    indexer_id, name, reward_address, user_address, index_ratio,
    last_update_height, txid, height, tx_idx
FROM stake_indexer_registers
WHERE user_address = $1
ORDER BY height ASC, tx_idx ASC
LIMIT 1
`
	var reg StakeIndexerRegister
	err := StakeDB.QueryRowContext(ctx, sqlText, userAddress).Scan(
		&reg.IndexerID,
		&reg.Name,
		&reg.RewardAddress,
		&reg.UserAddress,
		&reg.IndexRatio,
		&reg.LastUpdateHeight,
		&reg.RegisterTxID,
		&reg.Height,
		&reg.TxIdx,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query stake indexer register by user address failed: %w", err)
	}
	return &reg, nil
}

func GetStakeIndexerRegisterByID(ctx context.Context, indexerID string) (*StakeIndexerRegister, error) {
	if StakeDB == nil {
		return nil, nil
	}
	const sqlText = `
SELECT
    indexer_id, name, reward_address, user_address, index_ratio,
    last_update_height, txid, height, tx_idx
FROM stake_indexer_registers
WHERE indexer_id = $1
LIMIT 1
`
	var reg StakeIndexerRegister
	err := StakeDB.QueryRowContext(ctx, sqlText, indexerID).Scan(
		&reg.IndexerID,
		&reg.Name,
		&reg.RewardAddress,
		&reg.UserAddress,
		&reg.IndexRatio,
		&reg.LastUpdateHeight,
		&reg.RegisterTxID,
		&reg.Height,
		&reg.TxIdx,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query stake indexer register by id failed: %w", err)
	}
	return &reg, nil
}

func ListStakeIndexerRegisters(ctx context.Context) ([]StakeIndexerRegister, error) {
	if StakeDB == nil {
		return nil, nil
	}
	const sqlText = `
SELECT
    indexer_id, name, reward_address, user_address, index_ratio,
    last_update_height, txid, height, tx_idx
FROM stake_indexer_registers
ORDER BY height ASC, tx_idx ASC
`
	rows, err := StakeDB.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, fmt.Errorf("query stake indexer registers failed: %w", err)
	}
	defer rows.Close()

	result := make([]StakeIndexerRegister, 0, 32)
	for rows.Next() {
		var reg StakeIndexerRegister
		if err := rows.Scan(
			&reg.IndexerID,
			&reg.Name,
			&reg.RewardAddress,
			&reg.UserAddress,
			&reg.IndexRatio,
			&reg.LastUpdateHeight,
			&reg.RegisterTxID,
			&reg.Height,
			&reg.TxIdx,
		); err != nil {
			return nil, fmt.Errorf("scan stake indexer register failed: %w", err)
		}
		result = append(result, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake indexer registers failed: %w", err)
	}
	return result, nil
}

func CountStakeIndexerRegisters(ctx context.Context) (int, error) {
	if StakeDB == nil {
		return 0, nil
	}
	const sqlText = `SELECT COUNT(*) FROM stake_indexer_registers`
	var count int
	if err := StakeDB.QueryRowContext(ctx, sqlText).Scan(&count); err != nil {
		return 0, fmt.Errorf("count stake indexer registers failed: %w", err)
	}
	return count, nil
}

func ExistsStakeIndexerRegisterByUserAddress(ctx context.Context, userAddress string) (bool, error) {
	if StakeDB == nil {
		return false, nil
	}
	userAddress = strings.TrimSpace(userAddress)
	if userAddress == "" {
		return false, nil
	}

	const sqlText = `
SELECT 1
FROM stake_indexer_registers
WHERE user_address = $1
LIMIT 1
`
	var marker int
	if err := StakeDB.QueryRowContext(ctx, sqlText, userAddress).Scan(&marker); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("query stake indexer register by user address failed: %w", err)
	}
	return true, nil
}
