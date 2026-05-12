package slow

import (
	"stake_indexer/constant"
	"stake_indexer/internal/component/pg"

	redis "github.com/go-redis/redis/v8"
)

func runRewardSnapshotWriteFlow(height uint32, deltas []pgdb.IndexerAddrDelta) error {
	incrCmds := make(map[string]*redis.IntCmd, len(deltas))
	if err := rewardSyncManager.FlushBalanceChangesWithFinalizer(func(pipe redis.Pipeliner) {
		for _, item := range deltas {
			if item.Delta == 0 {
				continue
			}
			key := constant.GetSnapshotBalanceKey(item.Address)
			incrCmds[key] = pipe.IncrBy(ctx, key, item.Delta)
		}

		pipe.HSet(ctx, constant.TASK_INFO_KEYNAME,
			constant.TASK_SNAPSHOT_CONSUMER_HEIGHT, height,
		)
		pipe.HSet(ctx, constant.GetStakeIndexerStatusKey(), "stake_reward_sync_height", height)
	}); err != nil {
		return err
	}
	if err := cleanupZeroSnapshotBalances(incrCmds); err != nil {
		return err
	}
	return nil
}
