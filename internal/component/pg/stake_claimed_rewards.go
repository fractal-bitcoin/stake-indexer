package pgdb

import (
	"context"
	"database/sql"
	"fmt"
)

type StakeClaimedReward struct {
	UserAddress string
	IndexerID   string
	RewardType  string
	Amount      uint64
	TxID        string
	Height      uint32
	TxIdx       uint32
}

func SumClaimedRewards(ctx context.Context) (uint64, error) {
	if StakeDB == nil {
		return 0, nil
	}
	const sqlText = `SELECT COALESCE(SUM(amount), 0) FROM stake_claimed_rewards`
	var sum uint64
	err := StakeDB.QueryRowContext(ctx, sqlText).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("sum claimed rewards failed: %w", err)
	}
	return sum, nil
}

func SumClaimedRewardsByUserAddress(ctx context.Context, userAddress string) (uint64, error) {
	if StakeDB == nil {
		return 0, nil
	}
	const sqlText = `SELECT COALESCE(SUM(amount), 0) FROM stake_claimed_rewards WHERE user_address = $1 AND reward_type = $2`
	var sum uint64
	err := StakeDB.QueryRowContext(ctx, sqlText, userAddress, StakeRewardTypeStake).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("sum claimed rewards by user address failed: %w", err)
	}
	return sum, nil
}

func SumClaimedIndexerRewards(ctx context.Context, userAddress, indexerID string) (uint64, error) {
	if StakeDB == nil {
		return 0, nil
	}
	const sqlText = `SELECT COALESCE(SUM(amount), 0) FROM stake_claimed_rewards WHERE user_address = $1 AND indexer_id = $2 AND reward_type = $3`
	var sum uint64
	err := StakeDB.QueryRowContext(ctx, sqlText, userAddress, indexerID, StakeRewardTypeIndexer).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("sum claimed indexer rewards failed: %w", err)
	}
	return sum, nil
}

func UpsertStakeClaimedReward(ctx context.Context, item StakeClaimedReward) error {
	if StakeDB == nil {
		return nil
	}

	return upsertStakeClaimedReward(ctx, StakeDB, item)
}

func UpsertStakeClaimedRewardTx(ctx context.Context, tx *sql.Tx, item StakeClaimedReward) error {
	if tx == nil {
		return UpsertStakeClaimedReward(ctx, item)
	}

	return upsertStakeClaimedReward(ctx, tx, item)
}

func upsertStakeClaimedReward(ctx context.Context, execer stakeExecer, item StakeClaimedReward) error {
	if execer == nil {
		return nil
	}

	if item.RewardType == "" {
		item.RewardType = StakeRewardTypeStake
	}
	const sqlText = `
INSERT INTO stake_claimed_rewards (
    txid, user_address, indexer_id, reward_type, amount, height, tx_idx
) VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (txid) DO UPDATE
SET
    user_address = EXCLUDED.user_address,
    indexer_id = EXCLUDED.indexer_id,
    reward_type = EXCLUDED.reward_type,
    amount = EXCLUDED.amount,
    height = EXCLUDED.height,
    tx_idx = EXCLUDED.tx_idx
`
	_, err := execer.ExecContext(
		ctx, sqlText,
		item.TxID,
		item.UserAddress,
		item.IndexerID,
		item.RewardType,
		item.Amount,
		item.Height,
		item.TxIdx,
	)
	if err != nil {
		return fmt.Errorf("upsert stake claimed reward failed: %w", err)
	}
	return nil
}
