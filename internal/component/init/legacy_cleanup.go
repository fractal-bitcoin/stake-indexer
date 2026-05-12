package bootstrap

import (
	"context"
	"fmt"
	"stake_indexer/constant"
	"stake_indexer/internal/component/redis"

	redis "github.com/go-redis/redis/v8"
)

const legacyIndexerArtifactCleanupMarker = "cleanup:indexer_artifact:v1"

func CleanupLegacyIndexerArtifactsOnStartup(ctx context.Context) error {
	done, err := rdb.RdbBalanceClient.HGet(ctx, constant.TASK_INFO_KEYNAME, legacyIndexerArtifactCleanupMarker).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("load legacy indexer artifact cleanup marker failed: %w", err)
	}
	if done == "1" {
		return nil
	}

	patterns := []string{
		constant.INDEXER_UNDO_NEW_PREFIX + "*",
		constant.INDEXER_UNDO_SPENT_PREFIX + "*",
		constant.INDEXER_BLOCK_HASH_PREFIX + "*",
	}
	for _, pattern := range patterns {
		if err := scanDeleteRedisKeys(ctx, pattern, 2048); err != nil {
			return err
		}
	}

	pipe := rdb.RdbBalanceClient.Pipeline()
	pipe.HSet(ctx, constant.TASK_INFO_KEYNAME, legacyIndexerArtifactCleanupMarker, 1)
	if _, err := pipe.Exec(ctx); err != nil {
		pipe.Close()
		return fmt.Errorf("record legacy indexer artifact cleanup marker failed: %w", err)
	}
	pipe.Close()
	return nil
}

func scanDeleteRedisKeys(ctx context.Context, pattern string, scanCount int64) error {
	if scanCount <= 0 {
		scanCount = 1024
	}
	cursor := uint64(0)
	for {
		keys, nextCursor, err := rdb.RdbBalanceClient.Scan(ctx, cursor, pattern, scanCount).Result()
		if err != nil {
			return fmt.Errorf("scan redis keys failed for pattern %s: %w", pattern, err)
		}
		if len(keys) > 0 {
			if err := rdb.RdbBalanceClient.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("delete redis keys failed for pattern %s: %w", pattern, err)
			}
		}
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}
	return nil
}

