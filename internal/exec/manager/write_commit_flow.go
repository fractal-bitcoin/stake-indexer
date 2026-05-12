package indexer

import (
	"fmt"
	"stake_indexer/constant"
	"stake_indexer/internal/component/log"
	"stake_indexer/internal/component/pg"
	"stake_indexer/internal/component/redis"
	"stake_indexer/model"

	redis "github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

func runSubmitBlocksFlow(m *Manager, startHeight, stageBlockHeight uint32) error {
	if err := FlushIndexerBlockArtifacts(startHeight, stageBlockHeight); err != nil {
		return err
	}
	if err := persistPreparedStakeWrites(m, stageBlockHeight); err != nil {
		return err
	}
	if err := commitUtxoWritesAndCursor(stageBlockHeight); err != nil {
		return err
	}
	if ok := applyRealtimeBalances(m, stageBlockHeight); !ok {
		return fmt.Errorf("commit realtime balances failed at %d", stageBlockHeight)
	}
	if m != nil {
		if err := m.flushBalanceChanges(nil); err != nil {
			return fmt.Errorf("flush staged indexer updates failed: %w", err)
		}
	}
	if err := MarkIndexerBlocksCommitted(startHeight, stageBlockHeight); err != nil {
		return fmt.Errorf("mark committed range failed [%d,%d]: %w", startHeight, stageBlockHeight, err)
	}

	model.CleanUtxoMap()
	logger.Log.Info("block submit done")
	return nil
}

func persistPreparedStakeWrites(m *Manager, stageBlockHeight uint32) error {
	if m == nil {
		return nil
	}

	for _, reg := range m.WaitForUpsert.StakeIndexerRegisterList {
		if err := pgdb.UpsertStakeIndexerRegister(ctx, reg); err != nil {
			return fmt.Errorf("upsert stake indexer register failed: %w", err)
		}
	}
	if totalIndexers, err := pgdb.CountStakeIndexerRegisters(ctx); err != nil {
		return fmt.Errorf("count stake indexer registers failed: %w", err)
	} else if err := rdb.RdbBalanceClient.HSet(ctx, constant.GetStakeIndexerStatusKey(), "total_indexers", totalIndexers).Err(); err != nil {
		return fmt.Errorf("set status total_indexers failed: %w", err)
	}

	for _, p := range m.WaitForUpsert.StakeProofList {
		if err := pgdb.UpsertStakeProof(ctx, p); err != nil {
			return fmt.Errorf("upsert stake proof failed: %w", err)
		}
	}

	for _, r := range m.WaitForUpsert.StakeClaimedRewardList {
		if err := pgdb.UpsertStakeClaimedReward(ctx, r); err != nil {
			return fmt.Errorf("upsert stake claimed reward failed: %w", err)
		}
	}

	for _, b := range m.WaitForUpsert.StakeBindingList {
		if err := pgdb.InsertStakeBinding(ctx, b); err != nil {
			return fmt.Errorf("insert stake binding failed: %w", err)
		}
	}

	if len(m.WaitForUpsert.FIP101InscriptionEvents) > 0 {
		items := make([]*pgdb.FIP101InscriptionEvent, 0, len(m.WaitForUpsert.FIP101InscriptionEvents))
		for i := range m.WaitForUpsert.FIP101InscriptionEvents {
			items = append(items, &m.WaitForUpsert.FIP101InscriptionEvents[i])
		}
		if err := pgdb.UpsertFIP101InscriptionEventsBatch(ctx, items); err != nil {
			return fmt.Errorf("upsert fip101 inscription events batch failed: %w", err)
		}
	}

	m.WaitForUpsert = WaitForUpsert{}
	if err := m.LoadStakeBindingsToHeight(stageBlockHeight, false); err != nil {
		return fmt.Errorf("load stake bindings to height failed: %w", err)
	}
	return nil
}

func commitUtxoWritesAndCursor(stageBlockHeight uint32) error {
	if ok := updateUtxoInPikaAdd(model.GlobalNewUtxoDataMap); !ok {
		return fmt.Errorf("commit utxo add failed")
	}
	if ok := updateUtxoInPikaDel(model.GlobalDeleteUtxoKeysMap); !ok {
		return fmt.Errorf("commit utxo delete failed")
	}

	pipe := rdb.RdbUtxoClient.Pipeline()
	pipe.HSet(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_UTXO, stageBlockHeight)
	pipe.HIncrBy(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_UTXO_TOTAL, int64(len(model.GlobalNewUtxoDataMap)-len(model.GlobalSpentUtxoDataMap)))
	if _, err := pipe.Exec(ctx); err != nil {
		pipe.Close()
		return fmt.Errorf("commit utxo cursor failed: %w", err)
	}
	pipe.Close()
	return nil
}

func applyRealtimeBalances(m *Manager, stageBlockHeight uint32) bool {

	pipe := rdb.RdbBalanceClient.Pipeline()
	incrCmds := make(map[string]*redis.IntCmd, len(model.GlobalAddressBalanceDeltaMap))
	stakeIndexerTotals := make(map[string]int64)
	globalStakeTotalDelta := int64(0)

	for address, delta := range model.GlobalAddressBalanceDeltaMap {
		if delta == 0 {
			continue
		}
		key := constant.GetRealtimeBalanceKey(address)
		incrCmds[key] = pipe.IncrBy(ctx, key, delta)

		if m != nil {
			if info, ok := m.stakeAddrToIndexer[address]; ok {
				pipe.ZIncrBy(ctx, constant.GetIndexerStakeZsetKey(info.IndexerID), float64(delta), info.Address)
				stakeIndexerTotals[info.IndexerID] += delta
			}
		}
	}

	for indexerID, total := range stakeIndexerTotals {
		pipe.IncrBy(ctx, constant.GetIndexerStakeTotalKey(indexerID), total)
		pipe.ZRemRangeByScore(ctx, constant.GetIndexerStakeZsetKey(indexerID), "-inf", "0")
		globalStakeTotalDelta += total
	}
	if globalStakeTotalDelta != 0 {
		pipe.HIncrBy(ctx, constant.GetStakeIndexerStatusKey(), "confirmed_total_staked", globalStakeTotalDelta)
	}

	pipe.HSet(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_BLOCK_HEIGHT, stageBlockHeight)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		logger.Log.Error("commit realtime balances failed", zap.Error(err), zap.Uint32("height", stageBlockHeight))
		pipe.Close()
		return false
	}
	pipe.Close()

	zeroKeys := make([]string, 0, len(incrCmds))
	for key, cmd := range incrCmds {
		if cmd == nil {
			continue
		}
		v, err := cmd.Result()
		if err != nil {
			logger.Log.Warn("load realtime balance incr result failed", zap.Error(err), zap.String("key", key))
			continue
		}
		if v == 0 {
			zeroKeys = append(zeroKeys, key)
		}
	}
	if len(zeroKeys) > 0 {
		if err := rdb.RdbBalanceClient.Del(ctx, zeroKeys...).Err(); err != nil {
			logger.Log.Error("cleanup zero realtime balances failed", zap.Error(err))
			return false
		}
	}
	return true
}
