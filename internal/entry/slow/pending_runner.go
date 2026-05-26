package slow

import (
	"fmt"
	"stake_indexer/conf"
	"stake_indexer/constant"
	logger "stake_indexer/internal/component/log"
	pgdb "stake_indexer/internal/component/pg"
	rdb "stake_indexer/internal/component/redis"
	protocolparser "stake_indexer/internal/parser/protocol"
	"stake_indexer/model"
	"strconv"
	"time"

	redis "github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

var pendingRewardSyncManager RewardSyncManager

func initPendingRewardSyncManager() {
	if pendingManagerFactory == nil {
		panic("entry/slow pending manager factory is not set")
	}
	pendingRewardSyncManager = pendingManagerFactory(conf.StakeRewardCfg)
	cursorHeight, cursorExists, err := GetPendingConsumerHeight()
	if err != nil {
		panic(err)
	}
	if !cursorExists {
		return
	}
	if err := pendingRewardSyncManager.LoadStakeBindingsToHeight(cursorHeight, true); err != nil {
		panic(err)
	}
}

func SyncPendingRewardIndexer() {
	retryInterval := time.Duration(conf.StakeRewardCfg.RetryInterval) * time.Second
	loopInterval := time.Duration(conf.StakeRewardCfg.LoopInterval) * time.Second
	batchBlockCount := conf.StakeRewardCfg.BatchBlockCount
	startHeight := conf.StakeRewardCfg.IndexStartHeight
	if startHeight < conf.StakeRewardCfg.Stage2StartHeight {
		startHeight = conf.StakeRewardCfg.Stage2StartHeight
	}

	initPendingRewardSyncManager()

	for {
		if model.NeedStop {
			break
		}

		cursorHeight, cursorExists, err := GetPendingConsumerHeight()
		if err != nil {
			logger.Log.Warn("syncPendingRewardIndexer reload pending cursor failed", zap.Error(err))
			time.Sleep(retryInterval)
			continue
		}

		nextHeight := uint32(0)
		if cursorExists {
			nextHeight = cursorHeight + 1
		}
		if nextHeight < startHeight {
			nextHeight = startHeight
		}

		indexedHeight, exists, err := pgdb.GetLatestCommittedSyncBlockHeight(ctx)
		if err != nil {
			logger.Log.Warn("syncPendingRewardIndexer load indexer committed height failed", zap.Error(err))
			time.Sleep(retryInterval)
			continue
		}
		if !exists || indexedHeight <= 1000 {
			time.Sleep(retryInterval)
			continue
		}

		targetHeight := indexedHeight - 1000
		if targetHeight < conf.StakeRewardCfg.Stage2StartHeight {
			time.Sleep(loopInterval)
			continue
		}
		if targetHeight < nextHeight {
			time.Sleep(loopInterval)
			continue
		}

		endHeight := targetHeight
		if batchBlockCount > 0 {
			batchEnd := nextHeight + batchBlockCount - 1
			if batchEnd < endHeight {
				endHeight = batchEnd
			}
		}

		progressed := false
		for h := nextHeight; h <= endHeight; h++ {
			if model.NeedStop {
				break
			}
			ok, err := consumePendingRewardSnapshotBlock(h)
			if err != nil {
				logger.Log.Error("syncPendingRewardIndexer consume block failed",
					zap.Error(err),
					zap.Uint32("height", h))
				time.Sleep(retryInterval)
				progressed = false
				break
			}
			if !ok {
				break
			}
			progressed = true
		}

		if model.NeedStop {
			break
		}
		if !progressed {
			if targetHeight < nextHeight {
				time.Sleep(loopInterval)
			} else {
				time.Sleep(retryInterval)
			}
		}
	}
	logger.Log.Info("syncPendingRewardIndexer stopped")
}

func GetPendingConsumerHeight() (uint32, bool, error) {
	res, err := rdb.RdbBalanceClient.HGet(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_PENDING_CONSUMER_HEIGHT).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	v, err := strconv.ParseUint(res, 10, 32)
	if err != nil {
		return 0, false, err
	}
	return uint32(v), true, nil
}

func consumePendingRewardSnapshotBlock(height uint32) (bool, error) {
	syncBlock, err := pgdb.GetSyncBlock(ctx, height)
	if err != nil {
		return false, err
	}
	if syncBlock == nil || syncBlock.State != "committed" {
		return false, nil
	}
	if height < conf.StakeRewardCfg.Stage2StartHeight {
		return false, fmt.Errorf("pending reward consumer reached pre-stage2 height %d", height)
	}

	if err := pendingRewardSyncManager.LoadStakeBindingsToHeight(height, true); err != nil {
		return false, err
	}

	deltas, err := pgdb.ListIndexerAddrDeltasByHeight(ctx, height)
	if err != nil {
		return false, err
	}
	deltaByAddr := make(map[string]int64, len(deltas))
	for _, item := range deltas {
		deltaByAddr[item.Address] = item.Delta
	}
	pendingRewardSyncManager.StageStakeBalanceDeltas(deltaByAddr)

	events, err := pgdb.ListFIP101InscriptionEventsByHeight(ctx, height)
	if err != nil {
		return false, err
	}
	if err := pendingRewardSyncManager.ApplyFIP101EventsForSnapshot(height, events); err != nil {
		return false, err
	}
	if err := pendingRewardSyncManager.SubmitBalanceBlock(protocolparser.BlockSnapshot{
		Height:         syncBlock.Height,
		HashHex:        syncBlock.BlockHash,
		Version:        syncBlock.Version,
		CoinbaseReward: syncBlock.CoinbaseReward,
	}); err != nil {
		return false, err
	}
	if err := runPendingRewardSnapshotWriteFlow(height, deltas); err != nil {
		return false, err
	}
	return true, nil
}

func runPendingRewardSnapshotWriteFlow(height uint32, deltas []pgdb.IndexerAddrDelta) error {
	incrCmds := make(map[string]*redis.IntCmd, len(deltas))
	if err := pendingRewardSyncManager.FlushBalanceChangesWithFinalizer(func(pipe redis.Pipeliner) {
		for _, item := range deltas {
			if item.Delta == 0 {
				continue
			}
			key := constant.GetPendingSnapshotBalanceKey(item.Address)
			incrCmds[key] = pipe.IncrBy(ctx, key, item.Delta)
		}
		pipe.HSet(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_PENDING_CONSUMER_HEIGHT, height)
		pipe.HSet(ctx, constant.GetStakeIndexerStatusKey(), "pending_reward_sync_height", height)
	}); err != nil {
		return err
	}
	if err := cleanupZeroSnapshotBalances(incrCmds); err != nil {
		return err
	}
	return nil
}
