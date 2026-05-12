package indexer

import (
	"database/sql"
	"fmt"
	rollbackpkg "stake_indexer/internal/entry/rollback"
	"stake_indexer/internal/component/pg"
)

func FlushIndexerBlockArtifacts(startHeight, endHeight uint32) error {
	if endHeight < startHeight {
		return nil
	}

	stagedByHeight, err := buildStagedBlockMapForRange(startHeight, endHeight)
	if err != nil {
		return err
	}

	if err := flushPGArtifactsBatch(startHeight, endHeight, stagedByHeight); err != nil {
		return err
	}
	return nil
}

func buildStagedBlockMapForRange(startHeight, endHeight uint32) (map[uint32]*rollbackpkg.StagedIndexerBlock, error) {
	return rollbackpkg.BuildStagedBlockMapForRange(startHeight, endHeight)
}

func flushPGArtifactsBatch(startHeight, endHeight uint32, stagedByHeight map[uint32]*rollbackpkg.StagedIndexerBlock) error {
	if pgdb.StakeDB == nil {
		return nil
	}

	syncBlocks := make([]pgdb.SyncBlock, 0, int(endHeight-startHeight)+1)
	for h := startHeight; ; h++ {
		item := stagedByHeight[h]
		if item == nil {
			return fmt.Errorf("nil staged item in pg batch [%d,%d] at height %d", startHeight, endHeight, h)
		}
		syncBlocks = append(syncBlocks, pgdb.SyncBlock{
			Height:         item.Height,
			BlockHash:      item.HashHex,
			ParentHash:     item.ParentHash,
			Version:        item.Version,
			CoinbaseReward: item.CoinbaseReward,
			State:          "prepared",
		})
		if h == endHeight {
			break
		}
	}

	undoMinKeepHeight := rollbackpkg.MinIndexerUndoKeepHeight(endHeight)
	if err := withStakeDBTx(func(tx *sql.Tx) error {
		if err := pgdb.UpsertSyncBlocksBatch(ctx, tx, syncBlocks); err != nil {
			return fmt.Errorf("persist sync block batch failed [%d,%d]: %w", startHeight, endHeight, err)
		}
		for h := startHeight; ; h++ {
			item := stagedByHeight[h]
			if err := pgdb.ReplaceIndexerAddrDeltasTx(ctx, tx, item.Height, item.AddressDeltas); err != nil {
				return fmt.Errorf("persist indexer addr deltas failed at height %d: %w", item.Height, err)
			}
			if rollbackpkg.ShouldPersistIndexerUndo(endHeight, item.Height) {
				if err := pgdb.ReplaceIndexerUndoNewTx(ctx, tx, item.Height, item.UndoNew); err != nil {
					return fmt.Errorf("persist indexer undo new failed at height %d: %w", item.Height, err)
				}
				if err := pgdb.ReplaceIndexerUndoSpentTx(ctx, tx, item.Height, item.UndoSpent); err != nil {
					return fmt.Errorf("persist indexer undo spent failed at height %d: %w", item.Height, err)
				}
			}
			if h == endHeight {
				break
			}
		}
		if err := pgdb.DeleteIndexerUndoBeforeHeightTx(ctx, tx, undoMinKeepHeight); err != nil {
			return fmt.Errorf("cleanup stale indexer undo rows before %d failed: %w", undoMinKeepHeight, err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("flush pg indexer artifact batch failed [%d,%d]: %w", startHeight, endHeight, err)
	}

	return nil
}

func withStakeDBTx(fn func(*sql.Tx) error) error {
	tx, err := pgdb.StakeDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func MarkIndexerBlocksCommitted(startHeight, endHeight uint32) error {
	if endHeight < startHeight {
		return nil
	}
	if err := pgdb.MarkSyncBlocksCommittedRange(ctx, startHeight, endHeight); err != nil {
		return fmt.Errorf("mark committed failed: %w", err)
	}
	return nil
}


