package pgdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"stake_indexer/constant"

	"github.com/lib/pq"
)

type FIP101InscriptionEvent struct {
	TxID               string  `json:"txid"`
	Op                 string  `json:"op"`
	Height             int64   `json:"height"`
	InscriptionContent string  `json:"inscription_content"`
	IndexerID          string  `json:"indexer_id"`
	UserAddress        string  `json:"user_address"`
	RewardAddress      string  `json:"reward_address"`
	StakeAddress       string  `json:"stake_address"`
	Amount             uint64  `json:"amount"`
	IndexRatio         float64 `json:"index_ratio"`
	IndexerName        string  `json:"indexer_name"`
	ProveBlockHeight   uint32  `json:"prove_block_height"`
	ProveDataHash      string  `json:"prove_data_hash"`
	BizInvalidFlags    uint64  `json:"biz_invalid_flags"`
	TxIdx              uint32  `json:"tx_idx"`
}

func UpsertFIP101InscriptionEventsBatch(ctx context.Context, events []*FIP101InscriptionEvent) error {
	if StakeDB == nil || len(events) == 0 {
		return nil
	}

	const sqlText = `
INSERT INTO fip101_inscription_events (
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

	execer := stakeExecer(StakeDB)
	for _, event := range events {
		if event == nil || strings.TrimSpace(event.TxID) == "" || strings.TrimSpace(event.Op) == "" {
			continue
		}

		if _, err := execer.ExecContext(
			ctx,
			sqlText,
			event.TxID,
			event.Op,
			event.Height,
			event.InscriptionContent,
			event.IndexerID,
			event.UserAddress,
			event.RewardAddress,
			event.StakeAddress,
			event.Amount,
			event.IndexRatio,
			event.IndexerName,
			event.ProveBlockHeight,
			event.ProveDataHash,
			event.BizInvalidFlags,
			event.TxIdx,
		); err != nil {
			return fmt.Errorf("upsert fip101 inscription event failed txid=%s op=%s: %w", event.TxID, event.Op, err)
		}
	}

	return nil
}

func ListMempoolFIP101InscriptionEventTxIDs(ctx context.Context) ([]string, error) {
	if StakeDB == nil {
		return nil, nil
	}

	const sqlText = `
SELECT DISTINCT txid
FROM fip101_inscription_events
WHERE height = $1
`
	rows, err := StakeDB.QueryContext(ctx, sqlText, int64(constant.MEMPOOL_HEIGHT))
	if err != nil {
		return nil, fmt.Errorf("query mempool fip101 inscription txids failed: %w", err)
	}
	defer rows.Close()

	result := make([]string, 0, 128)
	for rows.Next() {
		var txid string
		if err := rows.Scan(&txid); err != nil {
			return nil, fmt.Errorf("scan mempool fip101 inscription txid failed: %w", err)
		}
		txid = strings.TrimSpace(strings.ToLower(txid))
		if txid == "" {
			continue
		}
		result = append(result, txid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mempool fip101 inscription txids failed: %w", err)
	}
	return result, nil
}

func DeleteMempoolFIP101InscriptionEventsByTxIDs(ctx context.Context, txIDs []string) error {
	if StakeDB == nil || len(txIDs) == 0 {
		return nil
	}

	cleaned := make([]string, 0, len(txIDs))
	for _, txid := range txIDs {
		txid = strings.TrimSpace(strings.ToLower(txid))
		if txid == "" {
			continue
		}
		cleaned = append(cleaned, txid)
	}
	if len(cleaned) == 0 {
		return nil
	}

	const sqlText = `
DELETE FROM fip101_inscription_events
WHERE height = $1
  AND txid = ANY($2)
`
	if _, err := StakeDB.ExecContext(ctx, sqlText, int64(constant.MEMPOOL_HEIGHT), pq.Array(cleaned)); err != nil {
		return fmt.Errorf("delete mempool fip101 inscription events failed: %w", err)
	}
	return nil
}

func DeleteFIP101InscriptionEventsFromHeight(ctx context.Context, fromHeight uint32) error {
	if StakeDB == nil {
		return nil
	}
	const sqlText = `
DELETE FROM fip101_inscription_events
WHERE height >= $1
`
	if _, err := StakeDB.ExecContext(ctx, sqlText, int64(fromHeight)); err != nil {
		return fmt.Errorf("delete fip101 inscription events from height failed: %w", err)
	}
	return nil
}

func ListFIP101InscriptionEventsByHeight(ctx context.Context, height uint32) ([]FIP101InscriptionEvent, error) {
	if StakeDB == nil {
		return nil, nil
	}

	const sqlText = `
SELECT
    txid, op, height, inscription_content, indexer_id, user_address, reward_address, stake_address,
    amount, index_ratio, indexer_name,
    prove_block_height, prove_data_hash, biz_invalid_flags, tx_idx
FROM fip101_inscription_events
WHERE height = $1
ORDER BY tx_idx ASC, txid ASC
`
	rows, err := StakeDB.QueryContext(ctx, sqlText, int64(height))
	if err != nil {
		return nil, fmt.Errorf("query fip101 inscription events by height failed: %w", err)
	}
	defer rows.Close()

	result := make([]FIP101InscriptionEvent, 0, 64)
	for rows.Next() {
		var item FIP101InscriptionEvent
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
			return nil, fmt.Errorf("scan fip101 inscription event failed: %w", err)
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
		return nil, fmt.Errorf("iterate fip101 inscription events failed: %w", err)
	}
	return result, nil
}

func GetFIP101ActorAddressByTxIDAndOp(ctx context.Context, txID, op string) (string, error) {
	if StakeDB == nil {
		return "", nil
	}
	txID = strings.TrimSpace(strings.ToLower(txID))
	op = strings.TrimSpace(strings.ToLower(op))
	if txID == "" || op == "" {
		return "", nil
	}

	const sqlText = `
SELECT user_address
FROM fip101_inscription_events
WHERE LOWER(txid) = LOWER($1) AND op = $2
LIMIT 1
`
	var actorAddress string
	if err := StakeDB.QueryRowContext(ctx, sqlText, txID, op).Scan(&actorAddress); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("query fip101 actor address by txid/op failed: %w", err)
	}
	return strings.TrimSpace(actorAddress), nil
}
