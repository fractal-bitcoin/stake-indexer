package pgdb

import (
	"context"
	"fmt"
)

type StakeRewardPoolLiability struct {
	AllocatedAmount      uint64
	ClaimedAmount        uint64
	PendingClaimedAmount uint64
}

// GetStakeRewardPoolLiability loads all pool-liability components in one database snapshot.
func GetStakeRewardPoolLiability(ctx context.Context) (StakeRewardPoolLiability, error) {
	if StakeDB == nil {
		return StakeRewardPoolLiability{}, nil
	}

	const sqlText = `
SELECT
    COALESCE((SELECT SUM(allocate_amount) FROM stake_allocated_rewards), 0),
    COALESCE((SELECT SUM(amount) FROM stake_claimed_rewards), 0),
    COALESCE((
        SELECT SUM(amount)
        FROM stake_mempool_events
        WHERE op = $1 AND biz_invalid_flags = 0
    ), 0)
`

	var liability StakeRewardPoolLiability
	if err := StakeDB.QueryRowContext(ctx, sqlText, stakeMempoolClaimOp).Scan(
		&liability.AllocatedAmount,
		&liability.ClaimedAmount,
		&liability.PendingClaimedAmount,
	); err != nil {
		return StakeRewardPoolLiability{}, fmt.Errorf("get stake reward pool liability failed: %w", err)
	}
	return liability, nil
}
