package bootstrap

import (
	"context"
	"time"

	"stake_indexer/conf"
	logger "stake_indexer/internal/component/log"
	pgdb "stake_indexer/internal/component/pg"

	"go.uber.org/zap"
)

const legacyIndexerClaimBackfillInterval = time.Minute

type LegacyIndexerClaimRewardFix struct {
	TxID      string
	IndexerID string
}

func StartLegacyIndexerClaimRewardBackfill(ctx context.Context, fixes map[string]string) {
	if !conf.StakeRewardCfg.FixLegacyIndexerClaimRewards {
		return
	}
	if len(fixes) == 0 {
		return
	}

	pending := make(map[string]string, len(fixes))
	for txid, indexerID := range fixes {
		if txid == "" || indexerID == "" {
			continue
		}
		pending[txid] = indexerID
	}
	if len(pending) == 0 {
		return
	}

	go runLegacyIndexerClaimRewardBackfill(ctx, pending, legacyIndexerClaimBackfillInterval)
}

func runLegacyIndexerClaimRewardBackfill(ctx context.Context, pending map[string]string, interval time.Duration) {
	if interval <= 0 {
		interval = legacyIndexerClaimBackfillInterval
	}

	logger.Log.Info("legacy indexer claim reward backfill started", zap.Int("pending", len(pending)))
	defer logger.Log.Info("legacy indexer claim reward backfill stopped")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastStatusLog := time.Time{}
	for {
		updated, fixed, errCount := backfillLegacyIndexerClaimRewardOnce(ctx, pending)
		if len(pending) == 0 {
			logger.Log.Info("legacy indexer claim reward backfill completed", zap.Int("fixed", fixed), zap.Int("updated", updated))
			return
		}
		if updated > 0 || time.Since(lastStatusLog) >= 5*time.Minute {
			logger.Log.Info("legacy indexer claim reward backfill progress",
				zap.Int("pending", len(pending)),
				zap.Int("fixed", fixed),
				zap.Int("updated", updated),
				zap.Int("errors", errCount))
			lastStatusLog = time.Now()
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func backfillLegacyIndexerClaimRewardOnce(ctx context.Context, pending map[string]string) (updated int, fixed int, errCount int) {
	for txid, indexerID := range pending {
		result, err := pgdb.EnsureLegacyIndexerClaimedRewardFixed(ctx, txid, indexerID)
		if err != nil {
			errCount++
			logger.Log.Warn("legacy indexer claim reward backfill failed",
				zap.String("txid", txid),
				zap.String("indexer_id", indexerID),
				zap.Error(err))
			continue
		}
		if result.Updated {
			updated++
			logger.Log.Info("legacy indexer claim reward fixed",
				zap.String("txid", txid),
				zap.String("indexer_id", indexerID))
		}
		if result.Fixed {
			fixed++
			delete(pending, txid)
		}
	}
	return updated, fixed, errCount
}
