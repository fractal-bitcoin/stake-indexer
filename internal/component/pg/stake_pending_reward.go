package pgdb

import (
	"context"
	"database/sql"
	"fmt"
	"math"
)

type StakePendingReward struct {
	UserAddress          string
	IndexerID            string
	StakeAddress         string
	RewardType           string
	Height               uint32
	StakeAmountSnapshot  uint64
	StakeAmountEffective uint64
	TotalEffectiveStake  uint64
	ReleasePercent       float64
	BlockRewardAmount    uint64
	IndexerRatio         float64
	PendingAmount        uint64
}

func ListStakePendingRewardsByUserAddress(ctx context.Context, userAddress string, limit, offset int) ([]StakePendingReward, error) {
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
    height, stake_amount_snapshot, stake_amount_effective, total_effective_stake,
    release_percent, block_reward_amount, indexer_ratio, pending_amount
FROM stake_pending_rewards
WHERE user_address = $1
ORDER BY height DESC, id DESC
LIMIT $2 OFFSET $3
`
	rows, err := StakeDB.QueryContext(ctx, sqlText, userAddress, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query stake pending rewards by user address failed: %w", err)
	}
	defer rows.Close()

	result := make([]StakePendingReward, 0, limit)
	for rows.Next() {
		var item StakePendingReward
		var allocateHeight int64
		var stakeAmountSnapshot int64
		var stakeAmountEffective int64
		var totalEffectiveStake int64
		var releasePercent float64
		var blockRewardAmount int64
		var indexerRatio float64
		var pendingAmount int64
		if err := rows.Scan(
			&item.UserAddress,
			&item.IndexerID,
			&item.StakeAddress,
			&item.RewardType,
			&allocateHeight,
			&stakeAmountSnapshot,
			&stakeAmountEffective,
			&totalEffectiveStake,
			&releasePercent,
			&blockRewardAmount,
			&indexerRatio,
			&pendingAmount,
		); err != nil {
			return nil, fmt.Errorf("scan stake pending reward failed: %w", err)
		}
		if allocateHeight < 0 {
			allocateHeight = 0
		}
		if stakeAmountSnapshot < 0 {
			stakeAmountSnapshot = 0
		}
		if stakeAmountEffective < 0 {
			stakeAmountEffective = 0
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
		if pendingAmount < 0 {
			pendingAmount = 0
		}
		item.Height = uint32(allocateHeight)
		item.StakeAmountSnapshot = uint64(stakeAmountSnapshot)
		item.StakeAmountEffective = uint64(stakeAmountEffective)
		item.TotalEffectiveStake = uint64(totalEffectiveStake)
		item.ReleasePercent = releasePercent
		item.BlockRewardAmount = uint64(blockRewardAmount)
		item.IndexerRatio = indexerRatio
		item.PendingAmount = uint64(pendingAmount)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake pending rewards failed: %w", err)
	}
	return result, nil
}

func CountStakePendingRewardsByUserAddress(ctx context.Context, userAddress string) (int, error) {
	if StakeDB == nil {
		return 0, nil
	}
	const sqlText = `SELECT COUNT(*) FROM stake_pending_rewards WHERE user_address = $1`
	var count int
	if err := StakeDB.QueryRowContext(ctx, sqlText, userAddress).Scan(&count); err != nil {
		return 0, fmt.Errorf("count stake pending rewards by user address failed: %w", err)
	}
	return count, nil
}

func UpsertStakePendingRewardBatch(ctx context.Context, items []StakePendingReward) error {
	if StakeDB == nil || len(items) == 0 {
		return nil
	}

	tx, err := StakeDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert stake pending rewards tx failed: %w", err)
	}
	defer tx.Rollback()

	for _, item := range items {
		if err := upsertStakePendingReward(ctx, tx, item); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert stake pending rewards tx failed: %w", err)
	}
	return nil
}

func ListStakePendingRewardsByHeight(ctx context.Context, height uint32) ([]StakePendingReward, error) {
	if StakeDB == nil {
		return nil, nil
	}
	const sqlText = `
SELECT
    user_address, indexer_id, stake_address, reward_type,
    height, stake_amount_snapshot, stake_amount_effective, total_effective_stake,
    release_percent, block_reward_amount, indexer_ratio, pending_amount
FROM stake_pending_rewards
WHERE height = $1
ORDER BY id ASC
`
	rows, err := StakeDB.QueryContext(ctx, sqlText, height)
	if err != nil {
		return nil, fmt.Errorf("query stake pending rewards by height failed: %w", err)
	}
	defer rows.Close()

	result := make([]StakePendingReward, 0, 16)
	for rows.Next() {
		var item StakePendingReward
		var allocateHeight int64
		var stakeAmountSnapshot int64
		var stakeAmountEffective int64
		var totalEffectiveStake int64
		var releasePercent float64
		var blockRewardAmount int64
		var indexerRatio float64
		var pendingAmount int64
		if err := rows.Scan(
			&item.UserAddress,
			&item.IndexerID,
			&item.StakeAddress,
			&item.RewardType,
			&allocateHeight,
			&stakeAmountSnapshot,
			&stakeAmountEffective,
			&totalEffectiveStake,
			&releasePercent,
			&blockRewardAmount,
			&indexerRatio,
			&pendingAmount,
		); err != nil {
			return nil, fmt.Errorf("scan stake pending reward by height failed: %w", err)
		}
		if allocateHeight < 0 {
			allocateHeight = 0
		}
		if stakeAmountSnapshot < 0 {
			stakeAmountSnapshot = 0
		}
		if stakeAmountEffective < 0 {
			stakeAmountEffective = 0
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
		if pendingAmount < 0 {
			pendingAmount = 0
		}
		item.Height = uint32(allocateHeight)
		item.StakeAmountSnapshot = uint64(stakeAmountSnapshot)
		item.StakeAmountEffective = uint64(stakeAmountEffective)
		item.TotalEffectiveStake = uint64(totalEffectiveStake)
		item.ReleasePercent = releasePercent
		item.BlockRewardAmount = uint64(blockRewardAmount)
		item.IndexerRatio = indexerRatio
		item.PendingAmount = uint64(pendingAmount)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake pending rewards by height failed: %w", err)
	}
	return result, nil
}

func DeleteStakePendingRewardsByHeight(ctx context.Context, height uint32) error {
	if StakeDB == nil {
		return nil
	}
	const sqlText = `DELETE FROM stake_pending_rewards WHERE height = $1`
	if _, err := StakeDB.ExecContext(ctx, sqlText, height); err != nil {
		return fmt.Errorf("delete stake pending rewards by height failed: %w", err)
	}
	return nil
}

func upsertStakePendingReward(ctx context.Context, execer stakeExecer, item StakePendingReward) error {
	if execer == nil {
		return nil
	}

	const sqlText = `
INSERT INTO stake_pending_rewards (
    user_address, indexer_id, stake_address, reward_type, height,
    stake_amount_snapshot, stake_amount_effective, total_effective_stake,
    release_percent, block_reward_amount, indexer_ratio, pending_amount
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (height, indexer_id, stake_address) DO UPDATE
SET
    user_address = EXCLUDED.user_address,
    reward_type = EXCLUDED.reward_type,
    stake_amount_snapshot = EXCLUDED.stake_amount_snapshot,
    stake_amount_effective = EXCLUDED.stake_amount_effective,
    total_effective_stake = EXCLUDED.total_effective_stake,
    release_percent = EXCLUDED.release_percent,
    block_reward_amount = EXCLUDED.block_reward_amount,
    indexer_ratio = EXCLUDED.indexer_ratio,
    pending_amount = EXCLUDED.pending_amount
`
	if _, err := execer.ExecContext(
		ctx, sqlText,
		item.UserAddress,
		item.IndexerID,
		item.StakeAddress,
		item.RewardType,
		item.Height,
		item.StakeAmountSnapshot,
		item.StakeAmountEffective,
		item.TotalEffectiveStake,
		item.ReleasePercent,
		item.BlockRewardAmount,
		item.IndexerRatio,
		item.PendingAmount,
	); err != nil {
		return fmt.Errorf("upsert stake pending reward failed: %w", err)
	}
	return nil
}
