package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"stake_indexer/constant"
	"stake_indexer/internal/component/pg"
	"stake_indexer/internal/component/redis"
)

func ResetMempoolSnapshotOnStartup(ctx context.Context) error {
	if err := pgdb.DeleteStakeMempoolEventsAll(ctx); err != nil {
		return err
	}

	deltaIndexersKey := constant.GetStakeMempoolIndexerStakerDeltaIndexersKey()
	stakersIndexersKey := constant.GetStakeMempoolIndexerStakersIndexersKey()

	deltaIndexerIDs, err := rdb.RdbBalanceClient.SMembers(ctx, deltaIndexersKey).Result()
	if err != nil {
		return fmt.Errorf("load stake mempool indexer delta keys failed: %w", err)
	}
	stakersIndexerIDs, err := rdb.RdbBalanceClient.SMembers(ctx, stakersIndexersKey).Result()
	if err != nil {
		return fmt.Errorf("load stake mempool indexer stakers keys failed: %w", err)
	}

	allIndexerIDs := make(map[string]struct{}, len(deltaIndexerIDs)+len(stakersIndexerIDs))
	for _, indexerID := range deltaIndexerIDs {
		indexerID = strings.TrimSpace(indexerID)
		if indexerID == "" {
			continue
		}
		allIndexerIDs[indexerID] = struct{}{}
	}
	for _, indexerID := range stakersIndexerIDs {
		indexerID = strings.TrimSpace(indexerID)
		if indexerID == "" {
			continue
		}
		allIndexerIDs[indexerID] = struct{}{}
	}

	delKeys := make([]string, 0, len(allIndexerIDs)*3+4)
	delKeys = append(delKeys,
		constant.GetStakeMempoolBalanceDeltaKey(),
		constant.GetStakeMempoolIndexerDeltaKey(),
		deltaIndexersKey,
		stakersIndexersKey,
	)
	for indexerID := range allIndexerIDs {
		delKeys = append(delKeys,
			constant.GetStakeMempoolIndexerStakerDeltaKey(indexerID),
			constant.GetStakeMempoolIndexerStakersKey(indexerID),
			constant.GetStakeMempoolIndexerStakersPendingKey(indexerID),
		)
	}
	if len(delKeys) > 0 {
		if err := rdb.RdbBalanceClient.Del(ctx, delKeys...).Err(); err != nil {
			return fmt.Errorf("delete stake mempool redis snapshot failed: %w", err)
		}
	}
	return nil
}

