package pgdb

import (
	"context"
	"database/sql"
	"fmt"
	"math"
)

type StakeAllocatedReward struct {
	UserAddress          string
	IndexerID            string
	StakeAddress         string
	RewardType           string
	Height               uint32
	StakeAmountSnapshot  uint64
	IndexerTotalStake    uint64
	IndexerEffectivePct  float64
	StakeAmountEffective uint64
	PlatformTotalStake   uint64
	TotalEffectiveStake  uint64
	ReleasePercent       float64
	BlockRewardAmount    uint64
	IndexerRatio         float64
	AllocateAmount       uint64
}

const (
	StakeRewardTypeStake   = "stake"
	StakeRewardTypeIndexer = "indexer"
)

func ListStakeAllocatedRewardsByUserAddress(ctx context.Context, userAddress string, limit, offset int) ([]StakeAllocatedReward, error) {
	if StakeDB == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	const sqlText = `
SELECT
    user_address, indexer_id, stake_address, reward_type,
    height, stake_amount_snapshot, indexer_total_stake, indexer_effective_percent,
    stake_amount_effective, platform_total_stake, total_effective_stake,
    release_percent, block_reward_amount, indexer_ratio, allocate_amount
FROM stake_allocated_rewards
WHERE user_address = $1
ORDER BY height DESC, id DESC
LIMIT $2 OFFSET $3
`
	rows, err := StakeDB.QueryContext(ctx, sqlText, userAddress, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query stake allocated rewards by user address failed: %w", err)
	}
	defer rows.Close()

	result := make([]StakeAllocatedReward, 0, limit)
	for rows.Next() {
		var item StakeAllocatedReward
		var allocateHeight int64
		var stakeAmountSnapshot int64
		var indexerTotalStake int64
		var indexerEffectivePercent float64
		var stakeAmountEffective int64
		var platformTotalStake int64
		var totalEffectiveStake int64
		var releasePercent float64
		var blockRewardAmount int64
		var indexerRatio float64
		var allocateAmount int64
		if err := rows.Scan(
			&item.UserAddress,
			&item.IndexerID,
			&item.StakeAddress,
			&item.RewardType,
			&allocateHeight,
			&stakeAmountSnapshot,
			&indexerTotalStake,
			&indexerEffectivePercent,
			&stakeAmountEffective,
			&platformTotalStake,
			&totalEffectiveStake,
			&releasePercent,
			&blockRewardAmount,
			&indexerRatio,
			&allocateAmount,
		); err != nil {
			return nil, fmt.Errorf("scan stake allocated reward failed: %w", err)
		}
		if allocateHeight < 0 {
			allocateHeight = 0
		}
		if stakeAmountSnapshot < 0 {
			stakeAmountSnapshot = 0
		}
		if indexerTotalStake < 0 {
			indexerTotalStake = 0
		}
		if indexerEffectivePercent < 0 || math.IsNaN(indexerEffectivePercent) || math.IsInf(indexerEffectivePercent, 0) {
			indexerEffectivePercent = 0
		}
		if indexerEffectivePercent > 100 {
			indexerEffectivePercent = 100
		}
		if stakeAmountEffective < 0 {
			stakeAmountEffective = 0
		}
		if platformTotalStake < 0 {
			platformTotalStake = 0
		}
		if totalEffectiveStake < 0 {
			totalEffectiveStake = 0
		}
		if releasePercent < 0 || math.IsNaN(releasePercent) || math.IsInf(releasePercent, 0) {
			releasePercent = 0
		}
		if releasePercent > 100 {
			releasePercent = 100
		}
		if blockRewardAmount < 0 {
			blockRewardAmount = 0
		}
		if indexerRatio < 0 {
			indexerRatio = 0
		}
		if allocateAmount < 0 {
			allocateAmount = 0
		}
		item.Height = uint32(allocateHeight)
		item.StakeAmountSnapshot = uint64(stakeAmountSnapshot)
		item.IndexerTotalStake = uint64(indexerTotalStake)
		item.IndexerEffectivePct = indexerEffectivePercent
		item.StakeAmountEffective = uint64(stakeAmountEffective)
		item.PlatformTotalStake = uint64(platformTotalStake)
		item.TotalEffectiveStake = uint64(totalEffectiveStake)
		item.ReleasePercent = releasePercent
		item.BlockRewardAmount = uint64(blockRewardAmount)
		item.IndexerRatio = indexerRatio
		item.AllocateAmount = uint64(allocateAmount)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake allocated rewards failed: %w", err)
	}
	return result, nil
}

func CountStakeAllocatedRewardsByUserAddress(ctx context.Context, userAddress string) (int, error) {
	if StakeDB == nil {
		return 0, nil
	}

	const sqlText = `SELECT COUNT(*) FROM stake_allocated_rewards WHERE user_address = $1`
	var count int
	if err := StakeDB.QueryRowContext(ctx, sqlText, userAddress).Scan(&count); err != nil {
		return 0, fmt.Errorf("count stake allocated rewards by user address failed: %w", err)
	}
	return count, nil
}

func GetLatestStakeAllocatedRewardHeight(ctx context.Context) (uint32, bool, error) {
	if StakeDB == nil {
		return 0, false, nil
	}

	const sqlText = `SELECT height FROM stake_allocated_rewards ORDER BY height DESC, id DESC LIMIT 1`
	var height uint32
	err := StakeDB.QueryRowContext(ctx, sqlText).Scan(&height)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("query latest stake allocated reward height failed: %w", err)
	}
	return height, true, nil
}

func UpsertStakeAllocatedReward(ctx context.Context, item StakeAllocatedReward) error {
	if StakeDB == nil {
		return nil
	}
	return upsertStakeAllocatedReward(ctx, StakeDB, item)
}

func UpsertStakeAllocatedRewardTx(ctx context.Context, tx *sql.Tx, item StakeAllocatedReward) error {
	if tx == nil {
		return UpsertStakeAllocatedReward(ctx, item)
	}
	return upsertStakeAllocatedReward(ctx, tx, item)
}

func UpsertStakeAllocatedRewardBatch(ctx context.Context, items []StakeAllocatedReward) error {
	if StakeDB == nil || len(items) == 0 {
		return nil
	}

	tx, err := StakeDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert stake allocated rewards tx failed: %w", err)
	}
	defer tx.Rollback()

	for _, item := range items {
		if err := upsertStakeAllocatedReward(ctx, tx, item); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert stake allocated rewards tx failed: %w", err)
	}
	return nil
}

func upsertStakeAllocatedReward(ctx context.Context, execer stakeExecer, item StakeAllocatedReward) error {
	if execer == nil {
		return nil
	}

	const sqlText = `
INSERT INTO stake_allocated_rewards (
    user_address, indexer_id, stake_address, reward_type, height,
    stake_amount_snapshot, indexer_total_stake, indexer_effective_percent,
    stake_amount_effective, platform_total_stake, total_effective_stake,
    release_percent, block_reward_amount, indexer_ratio, allocate_amount
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
ON CONFLICT (height, indexer_id, stake_address) DO UPDATE
SET
    user_address = EXCLUDED.user_address,
    reward_type = EXCLUDED.reward_type,
    stake_amount_snapshot = EXCLUDED.stake_amount_snapshot,
    indexer_total_stake = EXCLUDED.indexer_total_stake,
    indexer_effective_percent = EXCLUDED.indexer_effective_percent,
    stake_amount_effective = EXCLUDED.stake_amount_effective,
    platform_total_stake = EXCLUDED.platform_total_stake,
    total_effective_stake = EXCLUDED.total_effective_stake,
    release_percent = EXCLUDED.release_percent,
    block_reward_amount = EXCLUDED.block_reward_amount,
    indexer_ratio = EXCLUDED.indexer_ratio,
    allocate_amount = EXCLUDED.allocate_amount
`
	if _, err := execer.ExecContext(
		ctx, sqlText,
		item.UserAddress,
		item.IndexerID,
		item.StakeAddress,
		item.RewardType,
		item.Height,
		item.StakeAmountSnapshot,
		item.IndexerTotalStake,
		item.IndexerEffectivePct,
		item.StakeAmountEffective,
		item.PlatformTotalStake,
		item.TotalEffectiveStake,
		item.ReleasePercent,
		item.BlockRewardAmount,
		item.IndexerRatio,
		item.AllocateAmount,
	); err != nil {
		return fmt.Errorf("upsert stake allocated reward failed: %w", err)
	}
	return nil
}
