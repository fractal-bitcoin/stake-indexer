package indexer

import (
	"context"
	"fmt"

	"stake_indexer/constant"
	"stake_indexer/internal/component/redis"
)

func UpdateLatestBlockHeightStatus(height uint32) error {
	if err := rdb.RdbBalanceClient.HSet(context.Background(), constant.GetStakeIndexerStatusKey(), "latest_block_height", height).Err(); err != nil {
		return fmt.Errorf("update latest_block_height failed: %w", err)
	}
	return nil
}
