package rollback

import (
	"fmt"
	"stake_indexer/constant"
	idxbootstrap "stake_indexer/internal/component/init"
	"stake_indexer/internal/component/pg"
	"stake_indexer/internal/component/redis"

	redis "github.com/go-redis/redis/v8"
)

func RollbackIndexerFromHeight(fromHeight uint32) error {
	utxoLatest, utxoExists, err := GetIndexerUtxoHeight()
	if err != nil {
		return err
	}
	realtimeLatest, realtimeExists, err := GetInfoFieldUInt32(constant.TASK_BLOCK_HEIGHT)
	if err != nil {
		return err
	}
	pgCommittedLatest, pgCommittedExists, err := pgdb.GetLatestCommittedSyncBlockHeight(ctx)
	if err != nil {
		return err
	}

	maxHeight := uint32(0)
	if utxoExists && utxoLatest > maxHeight {
		maxHeight = utxoLatest
	}
	if realtimeExists && realtimeLatest > maxHeight {
		maxHeight = realtimeLatest
	}
	if pgCommittedExists && pgCommittedLatest > maxHeight {
		maxHeight = pgCommittedLatest
	}
	if maxHeight < fromHeight {
		return nil
	}
	if utxoExists && utxoLatest >= fromHeight && utxoLatest-fromHeight > indexerUndoRetentionDistance {
		return fmt.Errorf("rollback range exceeds undo retention: from=%d utxo_latest=%d retention=%d", fromHeight, utxoLatest, indexerUndoRetentionDistance)
	}

	if err := rollbackRealtimeBalanceFromHeight(fromHeight); err != nil {
		return err
	}

	for h := int64(maxHeight); h >= int64(fromHeight); h-- {
		height := uint32(h)
		if utxoExists && height <= utxoLatest {
			if err := rollbackUtxoOneBlock(height); err != nil {
				return err
			}
			continue
		}
	}

	rollbackTo := uint32(0)
	if fromHeight > 0 {
		rollbackTo = fromHeight - 1
	}
	if utxoExists {
		if _, err := rdb.RdbUtxoClient.HSet(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_UTXO, rollbackTo).Result(); err != nil {
			return err
		}
	}
	if err := pgdb.DeleteFIP101InscriptionEventsFromHeight(ctx, fromHeight); err != nil {
		return err
	}
	if err := pgdb.RollbackStakeIndexerFromHeight(ctx, fromHeight); err != nil {
		return err
	}
	if err := pgdb.DeleteIndexerArtifactsFromHeight(ctx, fromHeight); err != nil {
		return err
	}
	if err := pgdb.DeleteSyncBlocksFromHeight(ctx, fromHeight); err != nil {
		return err
	}
	if err := idxbootstrap.InitIndexerStatusCache(ctx); err != nil {
		return fmt.Errorf("rebuild indexer status cache after rollback failed: %w", err)
	}
	return nil
}

func rollbackUtxoOneBlock(height uint32) error {
	newMap, err := pgdb.ListIndexerUndoNewByHeight(ctx, height)
	if err != nil {
		return err
	}
	spentMap, err := pgdb.ListIndexerUndoSpentByHeight(ctx, height)
	if err != nil {
		return err
	}

	pipe := rdb.RdbUtxoClient.Pipeline()
	for outpoint := range newMap {
		pipe.Del(ctx, constant.GetUtxoKey(outpoint))
	}
	for outpoint, raw := range spentMap {
		pipe.Set(ctx, constant.GetUtxoKey(outpoint), raw, 0)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		pipe.Close()
		return err
	}
	pipe.Close()

	return nil
}

func rollbackRealtimeBalanceFromHeight(fromHeight uint32) error {
	current, err := rdb.RdbBalanceClient.HGet(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_BLOCK_HEIGHT).Int64()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	if current < int64(fromHeight) {
		return nil
	}

	type stakeAddressInfo struct {
		Address   string
		IndexerID string
	}
	stakeAddrToInfo := make(map[string]stakeAddressInfo)
	if pgdb.StakeDB != nil {
		bindings, err := pgdb.ListStakeBindingsUpToHeight(ctx, uint32(current))
		if err == nil {
			for _, b := range bindings {
				stakeAddrToInfo[b.StakeAddress] = stakeAddressInfo{
					Address:   b.UserAddress,
					IndexerID: b.IndexerID,
				}
			}
		}
	}

	for h := current; h >= int64(fromHeight); h-- {
		deltas, err := pgdb.ListIndexerAddrDeltasByHeight(ctx, uint32(h))
		if err != nil {
			return err
		}
		pipe := rdb.RdbBalanceClient.Pipeline()
		incrCmds := make(map[string]*redis.IntCmd, len(deltas))
		stakeIndexerTotals := make(map[string]int64)

		for _, item := range deltas {
			if item.Address == "" || item.Delta == 0 {
				continue
			}
			balanceKey := constant.GetRealtimeBalanceKey(item.Address)
			incrCmds[balanceKey] = pipe.IncrBy(ctx, balanceKey, -item.Delta)

			if info, ok := stakeAddrToInfo[item.Address]; ok {
				revDelta := -item.Delta
				pipe.ZIncrBy(ctx, constant.GetIndexerStakeZsetKey(info.IndexerID), float64(revDelta), info.Address)
				stakeIndexerTotals[info.IndexerID] += revDelta
			}
		}

		for indexerID, total := range stakeIndexerTotals {
			pipe.IncrBy(ctx, constant.GetIndexerStakeTotalKey(indexerID), total)
			pipe.ZRemRangeByScore(ctx, constant.GetIndexerStakeZsetKey(indexerID), "-inf", "0")
		}

		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			pipe.Close()
			return err
		}
		pipe.Close()

		zeroKeys := make([]string, 0, len(incrCmds))
		for key, cmd := range incrCmds {
			if cmd == nil {
				continue
			}
			v, err := cmd.Result()
			if err != nil {
				continue
			}
			if v == 0 {
				zeroKeys = append(zeroKeys, key)
			}
		}
		if len(zeroKeys) > 0 {
			if err := rdb.RdbBalanceClient.Del(ctx, zeroKeys...).Err(); err != nil {
				return err
			}
		}
	}

	rollbackTo := uint32(0)
	if fromHeight > 0 {
		rollbackTo = fromHeight - 1
	}
	pipe := rdb.RdbBalanceClient.Pipeline()
	pipe.HSet(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_BLOCK_HEIGHT, rollbackTo)
	if rollbackTo > 0 {
		syncBlock, syncErr := pgdb.GetSyncBlock(ctx, rollbackTo)
		if syncErr == nil && syncBlock != nil && syncBlock.BlockHash != "" {
			pipe.HSet(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_BLOCK, syncBlock.BlockHash)
		} else {
			pipe.HDel(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_BLOCK)
		}
	} else {
		pipe.HDel(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_BLOCK)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		pipe.Close()
		return err
	}
	pipe.Close()
	return nil
}

