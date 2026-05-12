package pgdb

import (
	"context"
	"fmt"
	"strings"
)

type StakeMempoolEvent struct {
	TxID               string
	Op                 string
	Height             int64
	InscriptionContent string
	IndexerID          string
	UserAddress        string
	RewardAddress      string
	StakeAddress       string
	Amount             uint64
	IndexRatio         float64
	IndexerName        string
	ProveBlockHeight   uint32
	ProveDataHash      string
	BizInvalidFlags    uint64
	TxIdx              uint32
}

func UpsertStakeMempoolEvent(ctx context.Context, item StakeMempoolEvent) error {
	if StakeDB == nil {
		return nil
	}

	const sqlText = `
INSERT INTO stake_mempool_events (
    txid, op, height, inscription_content, indexer_id, user_address, reward_address, stake_address,
    amount, index_ratio, indexer_name,
    prove_block_height, prove_data_hash, biz_invalid_flags, tx_idx
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
ON CONFLICT (txid) DO UPDATE
SET
    op = EXCLUDED.op,
    height = EXCLUDED.height,
    inscription_content = EXCLUDED.inscription_content,
    indexer_id = EXCLUDED.indexer_id,
    user_address = EXCLUDED.user_address,
    reward_address = EXCLUDED.reward_address,
    stake_address = EXCLUDED.stake_address,
    amount = EXCLUDED.amount,
    index_ratio = EXCLUDED.index_ratio,
    indexer_name = EXCLUDED.indexer_name,
    prove_block_height = EXCLUDED.prove_block_height,
    prove_data_hash = EXCLUDED.prove_data_hash,
    biz_invalid_flags = EXCLUDED.biz_invalid_flags,
    tx_idx = EXCLUDED.tx_idx
`

	if _, err := StakeDB.ExecContext(
		ctx,
		sqlText,
		item.TxID,
		item.Op,
		item.Height,
		item.InscriptionContent,
		item.IndexerID,
		item.UserAddress,
		item.RewardAddress,
		item.StakeAddress,
		item.Amount,
		item.IndexRatio,
		item.IndexerName,
		item.ProveBlockHeight,
		item.ProveDataHash,
		item.BizInvalidFlags,
		item.TxIdx,
	); err != nil {
		return fmt.Errorf("upsert stake mempool event failed: %w", err)
	}
	return nil
}

func ListStakeMempoolEventTxIDs(ctx context.Context) ([]string, error) {
	if StakeDB == nil {
		return nil, nil
	}

	const sqlText = `
SELECT txid
FROM stake_mempool_events
`
	rows, err := StakeDB.QueryContext(ctx, sqlText)
	if err != nil {
		return nil, fmt.Errorf("query stake mempool event txids failed: %w", err)
	}
	defer rows.Close()

	result := make([]string, 0, 64)
	for rows.Next() {
		var txID string
		if err := rows.Scan(&txID); err != nil {
			return nil, fmt.Errorf("scan stake mempool event txid failed: %w", err)
		}
		result = append(result, txID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake mempool event txids failed: %w", err)
	}
	return result, nil
}

func DeleteStakeMempoolEventsByTxIDs(ctx context.Context, txIDs []string) error {
	if StakeDB == nil || len(txIDs) == 0 {
		return nil
	}

	const chunkSize = 500
	for i := 0; i < len(txIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(txIDs) {
			end = len(txIDs)
		}
		chunk := txIDs[i:end]
		if len(chunk) == 0 {
			continue
		}

		placeholders := make([]string, 0, len(chunk))
		args := make([]interface{}, 0, len(chunk))
		for idx, txID := range chunk {
			placeholders = append(placeholders, fmt.Sprintf("$%d", idx+1))
			args = append(args, txID)
		}

		sqlText := fmt.Sprintf(`
DELETE FROM stake_mempool_events
WHERE txid IN (%s)
`, strings.Join(placeholders, ","))
		if _, err := StakeDB.ExecContext(ctx, sqlText, args...); err != nil {
			return fmt.Errorf("delete stake mempool events by txids failed: %w", err)
		}
	}
	return nil
}

func ListStakeMempoolEvents(ctx context.Context, op, userAddress, rewardAddress, indexerID string, limit, offset int) ([]StakeMempoolEvent, error) {
	if StakeDB == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	clauses := make([]string, 0, 4)
	args := make([]interface{}, 0, 5)
	argPos := 1
	clauses = append(clauses, "biz_invalid_flags = 0")
	if strings.TrimSpace(op) != "" {
		clauses = append(clauses, fmt.Sprintf("op = $%d", argPos))
		args = append(args, strings.TrimSpace(op))
		argPos++
	}
	if strings.TrimSpace(userAddress) != "" {
		clauses = append(clauses, fmt.Sprintf("user_address = $%d", argPos))
		args = append(args, strings.TrimSpace(userAddress))
		argPos++
	}
	if strings.TrimSpace(rewardAddress) != "" {
		clauses = append(clauses, fmt.Sprintf("reward_address = $%d", argPos))
		args = append(args, strings.TrimSpace(rewardAddress))
		argPos++
	}
	if strings.TrimSpace(indexerID) != "" {
		clauses = append(clauses, fmt.Sprintf("indexer_id = $%d", argPos))
		args = append(args, strings.TrimSpace(indexerID))
		argPos++
	}

	var sb strings.Builder
	sb.WriteString(`
SELECT
    txid, op, height, inscription_content, indexer_id, user_address, reward_address, stake_address,
    amount, index_ratio, indexer_name,
    prove_block_height, prove_data_hash, biz_invalid_flags, tx_idx
FROM stake_mempool_events
`)
	if len(clauses) > 0 {
		sb.WriteString("WHERE ")
		sb.WriteString(strings.Join(clauses, " AND "))
		sb.WriteByte('\n')
	}
	sb.WriteString(fmt.Sprintf("ORDER BY tx_idx DESC, txid DESC LIMIT $%d OFFSET $%d", argPos, argPos+1))
	args = append(args, limit, offset)

	rows, err := StakeDB.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query stake mempool events failed: %w", err)
	}
	defer rows.Close()

	result := make([]StakeMempoolEvent, 0, limit)
	for rows.Next() {
		var item StakeMempoolEvent
		var amount int64
		var proveBlockHeight int64
		var txIdx int64
		if err := rows.Scan(
			&item.TxID,
			&item.Op,
			&item.Height,
			&item.InscriptionContent,
			&item.IndexerID,
			&item.UserAddress,
			&item.RewardAddress,
			&item.StakeAddress,
			&amount,
			&item.IndexRatio,
			&item.IndexerName,
			&proveBlockHeight,
			&item.ProveDataHash,
			&item.BizInvalidFlags,
			&txIdx,
		); err != nil {
			return nil, fmt.Errorf("scan stake mempool event failed: %w", err)
		}
		if amount < 0 {
			amount = 0
		}
		if proveBlockHeight < 0 {
			proveBlockHeight = 0
		}
		if txIdx < 0 {
			txIdx = 0
		}
		item.Amount = uint64(amount)
		item.ProveBlockHeight = uint32(proveBlockHeight)
		item.TxIdx = uint32(txIdx)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake mempool events failed: %w", err)
	}
	return result, nil
}

func CountStakeMempoolEvents(ctx context.Context, op, userAddress, rewardAddress, indexerID string) (int, error) {
	if StakeDB == nil {
		return 0, nil
	}

	clauses := make([]string, 0, 4)
	args := make([]interface{}, 0, 3)
	argPos := 1
	clauses = append(clauses, "biz_invalid_flags = 0")
	if strings.TrimSpace(op) != "" {
		clauses = append(clauses, fmt.Sprintf("op = $%d", argPos))
		args = append(args, strings.TrimSpace(op))
		argPos++
	}
	if strings.TrimSpace(userAddress) != "" {
		clauses = append(clauses, fmt.Sprintf("user_address = $%d", argPos))
		args = append(args, strings.TrimSpace(userAddress))
		argPos++
	}
	if strings.TrimSpace(rewardAddress) != "" {
		clauses = append(clauses, fmt.Sprintf("reward_address = $%d", argPos))
		args = append(args, strings.TrimSpace(rewardAddress))
		argPos++
	}
	if strings.TrimSpace(indexerID) != "" {
		clauses = append(clauses, fmt.Sprintf("indexer_id = $%d", argPos))
		args = append(args, strings.TrimSpace(indexerID))
		argPos++
	}

	sqlText := "SELECT COUNT(*) FROM stake_mempool_events"
	if len(clauses) > 0 {
		sqlText += " WHERE " + strings.Join(clauses, " AND ")
	}

	var count int
	if err := StakeDB.QueryRowContext(ctx, sqlText, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count stake mempool events failed: %w", err)
	}
	return count, nil
}

func DeleteStakeMempoolEventsAll(ctx context.Context) error {
	if StakeDB == nil {
		return nil
	}

	const sqlText = `
DELETE FROM stake_mempool_events
`
	if _, err := StakeDB.ExecContext(ctx, sqlText); err != nil {
		return fmt.Errorf("delete all stake mempool events failed: %w", err)
	}
	return nil
}
