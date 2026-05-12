package pgdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type SyncBlock struct {
	Height               uint32
	BlockHash            string
	ParentHash           string
	Version              uint32
	CoinbaseReward       uint64
	State                string
	IsRewardBlockVersion bool
}

func UpsertSyncBlock(ctx context.Context, tx *sql.Tx, block SyncBlock) error {
	if StakeDB == nil {
		return nil
	}
	execer := stakeExecer(StakeDB)
	const sqlText = `
INSERT INTO sync_blocks (
    height, block_hash, parent_hash, version, coinbase_reward, state, is_reward_block_version
) VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (height) DO UPDATE
SET
    block_hash = EXCLUDED.block_hash,
    parent_hash = EXCLUDED.parent_hash,
    version = EXCLUDED.version,
    coinbase_reward = EXCLUDED.coinbase_reward,
    state = EXCLUDED.state,
    is_reward_block_version = EXCLUDED.is_reward_block_version
`
	if _, err := execer.ExecContext(
		ctx,
		sqlText,
		block.Height,
		block.BlockHash,
		block.ParentHash,
		block.Version,
		block.CoinbaseReward,
		block.State,
		isRewardBlockVersionByVersion(block.Version),
	); err != nil {
		return fmt.Errorf("upsert sync block failed: %w", err)
	}
	return nil
}

func UpsertSyncBlocksBatch(ctx context.Context, tx *sql.Tx, blocks []SyncBlock) error {
	if StakeDB == nil || len(blocks) == 0 {
		return nil
	}
	execer := stakeExecer(StakeDB)
	if tx != nil {
		execer = tx
	}

	const (
		valueCols = 7
		chunkSize = 200
	)
	for i := 0; i < len(blocks); i += chunkSize {
		end := i + chunkSize
		if end > len(blocks) {
			end = len(blocks)
		}
		chunk := blocks[i:end]
		if len(chunk) == 0 {
			continue
		}

		args := make([]interface{}, 0, len(chunk)*valueCols)
		var sqlText strings.Builder
		sqlText.WriteString(`
INSERT INTO sync_blocks (
    height, block_hash, parent_hash, version, coinbase_reward, state, is_reward_block_version
) VALUES `)
		argPos := 1
		for idx, block := range chunk {
			if idx > 0 {
				sqlText.WriteByte(',')
			}
			fmt.Fprintf(&sqlText, "($%d,$%d,$%d,$%d,$%d,$%d,$%d)", argPos, argPos+1, argPos+2, argPos+3, argPos+4, argPos+5, argPos+6)
			args = append(args,
				block.Height,
				block.BlockHash,
				block.ParentHash,
				block.Version,
				block.CoinbaseReward,
				block.State,
				isRewardBlockVersionByVersion(block.Version),
			)
			argPos += valueCols
		}
		sqlText.WriteString(`
ON CONFLICT (height) DO UPDATE
SET
    block_hash = EXCLUDED.block_hash,
    parent_hash = EXCLUDED.parent_hash,
    version = EXCLUDED.version,
    coinbase_reward = EXCLUDED.coinbase_reward,
    state = EXCLUDED.state,
    is_reward_block_version = EXCLUDED.is_reward_block_version
`)
		if _, err := execer.ExecContext(ctx, sqlText.String(), args...); err != nil {
			return fmt.Errorf("upsert sync blocks batch failed: %w", err)
		}
	}
	return nil
}

func DeleteSyncBlocksFromHeight(ctx context.Context, fromHeight uint32) error {
	if StakeDB == nil {
		return nil
	}
	if _, err := StakeDB.ExecContext(ctx, `DELETE FROM sync_blocks WHERE height >= $1`, fromHeight); err != nil {
		return fmt.Errorf("delete sync blocks from height failed: %w", err)
	}
	return nil
}

func MarkSyncBlocksCommittedRange(ctx context.Context, startHeight, endHeight uint32) error {
	if StakeDB == nil || endHeight < startHeight {
		return nil
	}
	const sqlText = `
UPDATE sync_blocks
SET state = 'committed'
WHERE height >= $1 AND height <= $2
`
	if _, err := StakeDB.ExecContext(ctx, sqlText, startHeight, endHeight); err != nil {
		return fmt.Errorf("mark sync blocks committed range failed: %w", err)
	}
	return nil
}

func GetSyncBlock(ctx context.Context, height uint32) (*SyncBlock, error) {
	if StakeDB == nil {
		return nil, nil
	}
	const sqlText = `
SELECT height, block_hash, parent_hash, version, coinbase_reward, state, is_reward_block_version
FROM sync_blocks
WHERE height = $1
LIMIT 1
`
	var item SyncBlock
	err := StakeDB.QueryRowContext(ctx, sqlText, height).Scan(
		&item.Height,
		&item.BlockHash,
		&item.ParentHash,
		&item.Version,
		&item.CoinbaseReward,
		&item.State,
		&item.IsRewardBlockVersion,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query sync block failed: %w", err)
	}
	return &item, nil
}

func GetLatestCommittedSyncBlockHeight(ctx context.Context) (uint32, bool, error) {
	if StakeDB == nil {
		return 0, false, nil
	}
	const sqlText = `
SELECT height
FROM sync_blocks
WHERE state = 'committed'
ORDER BY height DESC
LIMIT 1
`
	var height uint32
	err := StakeDB.QueryRowContext(ctx, sqlText).Scan(&height)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("query latest committed sync block height failed: %w", err)
	}
	return height, true, nil
}

func ListSyncBlocksRange(ctx context.Context, startHeight, endHeight uint32) ([]SyncBlock, error) {
	if StakeDB == nil {
		return nil, nil
	}
	const sqlText = `
SELECT height, block_hash, parent_hash, version, coinbase_reward, state, is_reward_block_version
FROM sync_blocks
WHERE height >= $1 AND height < $2
ORDER BY height ASC
`
	rows, err := StakeDB.QueryContext(ctx, sqlText, startHeight, endHeight)
	if err != nil {
		return nil, fmt.Errorf("query sync blocks range failed: %w", err)
	}
	defer rows.Close()

	result := make([]SyncBlock, 0, endHeight-startHeight)
	for rows.Next() {
		var item SyncBlock
		if err := rows.Scan(
			&item.Height,
			&item.BlockHash,
			&item.ParentHash,
			&item.Version,
			&item.CoinbaseReward,
			&item.State,
			&item.IsRewardBlockVersion,
		); err != nil {
			return nil, fmt.Errorf("scan sync block failed: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sync blocks failed: %w", err)
	}
	return result, nil
}

func CountRewardVersionBlocksInRange(ctx context.Context, startHeight, endHeight uint32) (int, error) {
	if StakeDB == nil {
		return 0, nil
	}
	if endHeight < startHeight {
		return 0, nil
	}

	const sqlText = `
SELECT COUNT(*)
FROM sync_blocks
WHERE height >= $1 AND height <= $2 AND is_reward_block_version = TRUE
`

	var count int
	if err := StakeDB.QueryRowContext(ctx, sqlText, startHeight, endHeight).Scan(&count); err != nil {
		return 0, fmt.Errorf("count reward version blocks in range failed: %w", err)
	}
	return count, nil
}

func isRewardBlockVersionByVersion(version uint32) bool {
	hexVersion := fmt.Sprintf("%08x", version)
	return strings.HasPrefix(hexVersion, "2026") && len(hexVersion) > 5 && hexVersion[5] == '1'
}
