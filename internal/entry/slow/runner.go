package slow

import (
	"context"
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

type RewardSyncManager interface {
	LoadStakeBindingsToHeight(height uint32, withBalance bool) error
	StageStakeBalanceDeltas(addressDeltas map[string]int64)
	ApplyFIP101EventsForSnapshot(height uint32, events []pgdb.FIP101InscriptionEvent) error
	SubmitBalanceBlock(block protocolparser.BlockSnapshot) error
	FlushBalanceChangesWithFinalizer(finalizer func(redis.Pipeliner)) error
}

var (
	ctx                   = context.Background()
	rewardSyncManager     RewardSyncManager
	managerFactory        func(conf.StakeRewardConfigInfo) RewardSyncManager
	pendingManagerFactory func(conf.StakeRewardConfigInfo) RewardSyncManager
)

func SetManagerFactory(factory func(conf.StakeRewardConfigInfo) RewardSyncManager) {
	managerFactory = factory
}

func SetPendingManagerFactory(factory func(conf.StakeRewardConfigInfo) RewardSyncManager) {
	pendingManagerFactory = factory
}

func initRewardSyncManager() {
	if managerFactory == nil {
		panic("entry/slow manager factory is not set")
	}
	rewardSyncManager = managerFactory(conf.StakeRewardCfg)
	cursorHeight, cursorExists, err := GetSnapshotConsumerHeight()
	if err != nil {
		panic(err)
	}
	if !cursorExists {
		return
	}
	if err := rewardSyncManager.LoadStakeBindingsToHeight(cursorHeight, true); err != nil {
		panic(err)
	}
}

func SyncStakeRewardIndexer() {
	slowLagBlocks := conf.StakeRewardCfg.SlowLagBlocks
	retryInterval := time.Duration(conf.StakeRewardCfg.RetryInterval) * time.Second
	loopInterval := time.Duration(conf.StakeRewardCfg.LoopInterval) * time.Second
	batchBlockCount := conf.StakeRewardCfg.BatchBlockCount
	indexStartHeight := conf.StakeRewardCfg.IndexStartHeight

	initRewardSyncManager()

	for {
		if model.NeedStop {
			break
		}

		cursorHeight, cursorExists, err := GetSnapshotConsumerHeight()
		if err != nil {
			logger.Log.Warn("syncStakeRewardIndexer reload stake reward cursor failed", zap.Error(err))
			time.Sleep(retryInterval)
			continue
		}
		nextHeight := uint32(0)
		if cursorExists {
			nextHeight = cursorHeight + 1
		}
		if nextHeight < indexStartHeight {
			nextHeight = indexStartHeight
		}

		indexedHeight, exists, err := pgdb.GetLatestCommittedSyncBlockHeight(ctx)
		if err != nil {
			logger.Log.Warn("syncStakeRewardIndexer load indexer committed height failed", zap.Error(err))
			time.Sleep(retryInterval)
			continue
		}
		if !exists || indexedHeight <= slowLagBlocks {
			logger.Log.Debug("syncStakeRewardIndexer waiting indexer height",
				zap.Bool("exists", exists),
				zap.Uint32("indexed_height", indexedHeight),
				zap.Uint32("slow_lag_blocks", slowLagBlocks))
			time.Sleep(retryInterval)
			continue
		}

		targetHeight := indexedHeight - slowLagBlocks
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
			ok, err := consumeRewardSnapshotBlock(h)
			if err != nil {
				logger.Log.Error("syncStakeRewardIndexer consume block failed",
					zap.Error(err),
					zap.Uint32("height", h))
				time.Sleep(retryInterval)
				progressed = false
				break
			}
			if !ok {
				logger.Log.Debug("syncStakeRewardIndexer waiting committed block", zap.Uint32("height", h))
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

	logger.Log.Info("syncStakeRewardIndexer stopped")
}

func EnsureManagerFactoryConfigured() error {
	if managerFactory == nil {
		return fmt.Errorf("entry/slow manager factory is not set")
	}
	if pendingManagerFactory == nil {
		return fmt.Errorf("entry/slow pending manager factory is not set")
	}
	return nil
}

func GetSnapshotConsumerHeight() (uint32, bool, error) {
	res, err := rdb.RdbBalanceClient.HGet(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_SNAPSHOT_CONSUMER_HEIGHT).Result()
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

func consumeRewardSnapshotBlock(height uint32) (bool, error) {
	syncBlock, err := pgdb.GetSyncBlock(ctx, height)
	if err != nil {
		return false, err
	}
	if syncBlock == nil || syncBlock.State != "committed" {
		return false, nil
	}

	if err := rewardSyncManager.LoadStakeBindingsToHeight(height, true); err != nil {
		return false, err
	}

	deltas, err := pgdb.ListIndexerAddrDeltasByHeight(ctx, height)
	if err != nil {
		logger.Log.Error("load indexer addr deltas failed", zap.Error(err), zap.Uint32("height", height))
		return false, err
	}

	deltaByAddr := make(map[string]int64, len(deltas))
	for _, item := range deltas {
		deltaByAddr[item.Address] = item.Delta
	}
	rewardSyncManager.StageStakeBalanceDeltas(deltaByAddr)

	events, err := pgdb.ListFIP101InscriptionEventsByHeight(ctx, height)
	if err != nil {
		return false, err
	}
	if err := rewardSyncManager.ApplyFIP101EventsForSnapshot(height, events); err != nil {
		return false, err
	}
	if err := rewardSyncManager.SubmitBalanceBlock(protocolparser.BlockSnapshot{
		Height:         syncBlock.Height,
		HashHex:        syncBlock.BlockHash,
		Version:        syncBlock.Version,
		CoinbaseReward: syncBlock.CoinbaseReward,
	}); err != nil {
		return false, err
	}
	if err := runRewardSnapshotWriteFlow(height, deltas); err != nil {
		return false, err
	}
	return true, nil
}

func cleanupZeroSnapshotBalances(incrCmds map[string]*redis.IntCmd) error {
	if len(incrCmds) == 0 {
		return nil
	}
	zeroKeys := make([]string, 0, len(incrCmds))
	for key, cmd := range incrCmds {
		if cmd == nil {
			continue
		}
		v, err := cmd.Result()
		if err != nil {
			logger.Log.Warn("load balance incr result failed", zap.Error(err), zap.String("key", key))
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
	return nil
}
