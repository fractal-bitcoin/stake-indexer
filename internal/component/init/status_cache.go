package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"stake_indexer/conf"
	"stake_indexer/constant"
	"stake_indexer/internal/component/node"
	"stake_indexer/internal/component/pg"
	rdb "stake_indexer/internal/component/redis"

	redis "github.com/go-redis/redis/v8"
)

func InitIndexerStatusCache(ctx context.Context) error {
	registers, err := pgdb.ListStakeIndexerRegisters(ctx)
	if err != nil {
		return err
	}

	confirmedTotalStaked := uint64(0)
	if len(registers) > 0 {
		pipe := rdb.RdbBalanceClient.Pipeline()
		defer pipe.Close()

		totalCmds := make(map[string]*redis.StringCmd, len(registers))
		for _, reg := range registers {
			if reg.IndexerID == "" {
				continue
			}
			totalCmds[reg.IndexerID] = pipe.Get(ctx, constant.GetIndexerStakeTotalKey(reg.IndexerID))
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return err
		}
		for _, cmd := range totalCmds {
			if cmd == nil {
				continue
			}
			val, err := cmd.Result()
			if err != nil || val == "" {
				continue
			}
			amount, parseErr := strconv.ParseUint(val, 10, 64)
			if parseErr != nil {
				continue
			}
			confirmedTotalStaked += amount
		}
	}

	latestBlockHeight := uint32(0)
	if h, err := node.GetBlockCountRPC(); err != nil {
		return fmt.Errorf("load latest block height failed: %w", err)
	} else {
		latestBlockHeight = h
	}

	stakeRewardSyncHeight := uint32(0)
	rawSyncHeight, err := rdb.RdbBalanceClient.HGet(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_SNAPSHOT_CONSUMER_HEIGHT).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("load stake reward sync height failed: %w", err)
	}
	if strings.TrimSpace(rawSyncHeight) != "" {
		if v, parseErr := strconv.ParseUint(strings.TrimSpace(rawSyncHeight), 10, 32); parseErr == nil {
			stakeRewardSyncHeight = uint32(v)
		}
	}

	pendingRewardSyncHeight := uint32(0)
	if rawPendingSyncHeight, pendingSyncErr := rdb.RdbBalanceClient.HGet(ctx, constant.GetStakeIndexerStatusKey(), "pending_reward_sync_height").Result(); pendingSyncErr == nil {
		if v, parseErr := strconv.ParseUint(strings.TrimSpace(rawPendingSyncHeight), 10, 32); parseErr == nil {
			pendingRewardSyncHeight = uint32(v)
		}
	} else if pendingSyncErr != redis.Nil {
		return fmt.Errorf("load pending reward sync height failed: %w", pendingSyncErr)
	}
	pendingRewardTotalAmount := uint64(0)
	if rawPendingTotal, pendingTotalErr := rdb.RdbBalanceClient.HGet(ctx, constant.GetStakeIndexerStatusKey(), "pending_reward_total_amount").Result(); pendingTotalErr == nil {
		if v, parseErr := strconv.ParseUint(strings.TrimSpace(rawPendingTotal), 10, 64); parseErr == nil {
			pendingRewardTotalAmount = v
		}
	} else if pendingTotalErr != redis.Nil {
		return fmt.Errorf("load pending reward total amount failed: %w", pendingTotalErr)
	}

	latestAllocatedRewardHeight := uint32(0)
	latestAllocatedRewardAmount := uint64(0)
	if h, exists, err := pgdb.GetLatestStakeAllocatedRewardHeight(ctx); err != nil {
		return fmt.Errorf("load latest allocated reward height failed: %w", err)
	} else if exists {
		latestAllocatedRewardHeight = h
		if syncBlock, blockErr := pgdb.GetSyncBlock(ctx, h); blockErr != nil {
			return fmt.Errorf("load latest allocated reward block failed: %w", blockErr)
		} else if syncBlock != nil {
			latestAllocatedRewardAmount = syncBlock.CoinbaseReward
		}
	}

	statusKey := constant.GetStakeIndexerStatusKey()
	releasePercent := conf.StakeRewardCfg.RewardReleasePercentByHeight(latestBlockHeight)
	if persistedRaw, persistedErr := rdb.RdbBalanceClient.HGet(ctx, statusKey, "reward_release_percent").Result(); persistedErr == nil {
		if persistedVal, parseErr := strconv.ParseFloat(strings.TrimSpace(persistedRaw), 64); parseErr == nil && persistedVal > releasePercent {
			releasePercent = persistedVal
		}
	} else if persistedErr != redis.Nil {
		return fmt.Errorf("load reward release percent failed: %w", persistedErr)
	}
	if releasePercent > 100 {
		releasePercent = 100
	}
	if releasePercent < 0 {
		releasePercent = 0
	}

	fields := map[string]interface{}{
		"total_indexers":                 len(registers),
		"confirmed_total_staked":         confirmedTotalStaked,
		"mempool_total_staked_delta":     0,
		"latest_block_height":            latestBlockHeight,
		"stake_reward_sync_height":       stakeRewardSyncHeight,
		"latest_allocated_reward_height": latestAllocatedRewardHeight,
		"latest_allocated_reward_amount": latestAllocatedRewardAmount,
		"reward_release_percent":         releasePercent,
		"pending_reward_sync_height":     pendingRewardSyncHeight,
		"pending_reward_total_amount":    pendingRewardTotalAmount,
	}
	if err := rdb.RdbBalanceClient.HSet(ctx, statusKey, fields).Err(); err != nil {
		return fmt.Errorf("init indexer status cache failed: %w", err)
	}
	return nil
}
