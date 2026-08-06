package pgdb

import (
	"context"
	"fmt"
)

const stakeMempoolClaimOp = "FIP_101_PLEDEGED_REWARD"

type StakeRewardBalance struct {
	AllocatedAmount      uint64
	ClaimedAmount        uint64
	ClaimedStakeReward   uint64
	ClaimedIndexerReward uint64
	PendingClaimedAmount uint64
}

// GetStakeRewardBalanceByUserAddress loads all reward-balance components in one database snapshot.
func GetStakeRewardBalanceByUserAddress(ctx context.Context, userAddress string) (StakeRewardBalance, error) {
	if StakeDB == nil {
		return StakeRewardBalance{}, nil
	}

	const sqlText = `
SELECT
    COALESCE((
        SELECT SUM(allocate_amount)
        FROM stake_allocated_rewards
        WHERE user_address = $1
    ), 0),
    COALESCE((
        SELECT SUM(amount)
        FROM stake_claimed_rewards
        WHERE user_address = $1
    ), 0),
    COALESCE((
        SELECT SUM(amount)
        FROM stake_claimed_rewards
        WHERE user_address = $1 AND reward_type = $2
    ), 0),
    COALESCE((
        SELECT SUM(amount)
        FROM stake_claimed_rewards
        WHERE user_address = $1 AND reward_type = $3
    ), 0),
    COALESCE((
        SELECT SUM(amount)
        FROM stake_mempool_events
        WHERE op = $4 AND user_address = $1 AND biz_invalid_flags = 0
    ), 0)
`

	var balance StakeRewardBalance
	if err := StakeDB.QueryRowContext(
		ctx,
		sqlText,
		userAddress,
		StakeRewardTypeStake,
		StakeRewardTypeIndexer,
		stakeMempoolClaimOp,
	).Scan(
		&balance.AllocatedAmount,
		&balance.ClaimedAmount,
		&balance.ClaimedStakeReward,
		&balance.ClaimedIndexerReward,
		&balance.PendingClaimedAmount,
	); err != nil {
		return StakeRewardBalance{}, fmt.Errorf("get stake reward balance by user address failed: %w", err)
	}
	return balance, nil
}
