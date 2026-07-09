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

type LegacyIndexerClaimRewardFixResult struct {
	Exists  bool
	Fixed   bool
	Updated bool
}

type IndexerClaimRewardAddressFixResult struct {
	Updated int64
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

func EnsureLegacyIndexerClaimedRewardFixed(ctx context.Context, txid, indexerID string) (LegacyIndexerClaimRewardFixResult, error) {
	return ensureLegacyIndexerClaimedRewardFixed(ctx, StakeDB, txid, indexerID)
}

func FixIndexerClaimRewardUserAddresses(ctx context.Context) (IndexerClaimRewardAddressFixResult, error) {
	return fixIndexerClaimRewardUserAddresses(ctx, StakeDB)
}

func ensureLegacyIndexerClaimedRewardFixed(ctx context.Context, execer interface {
	stakeExecer
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}, txid, indexerID string) (LegacyIndexerClaimRewardFixResult, error) {
	result := LegacyIndexerClaimRewardFixResult{}
	if execer == nil || txid == "" || indexerID == "" {
		return result, nil
	}

	const updateSQL = `
UPDATE stake_claimed_rewards
SET reward_type = $2, indexer_id = $3
WHERE txid = $1
  AND (COALESCE(reward_type, 'stake') <> $2 OR COALESCE(indexer_id, '') <> $3)`
	updateResult, err := execer.ExecContext(ctx, updateSQL, txid, StakeRewardTypeIndexer, indexerID)
	if err != nil {
		return result, fmt.Errorf("fix legacy indexer claimed reward failed: %w", err)
	}
	if updateResult != nil {
		rowsAffected, err := updateResult.RowsAffected()
		if err == nil && rowsAffected > 0 {
			result.Updated = true
		}
	}

	const selectSQL = `SELECT reward_type, indexer_id FROM stake_claimed_rewards WHERE txid = $1`
	var rewardType string
	var actualIndexerID string
	err = execer.QueryRowContext(ctx, selectSQL, txid).Scan(&rewardType, &actualIndexerID)
	if err == sql.ErrNoRows {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("query legacy indexer claimed reward failed: %w", err)
	}
	result.Exists = true
	result.Fixed = rewardType == StakeRewardTypeIndexer && actualIndexerID == indexerID
	return result, nil
}

func fixIndexerClaimRewardUserAddresses(ctx context.Context, execer stakeExecer) (IndexerClaimRewardAddressFixResult, error) {
	result := IndexerClaimRewardAddressFixResult{}
	if execer == nil {
		return result, nil
	}

	const sqlText = `
UPDATE stake_claimed_rewards scr
SET user_address = TRIM(reg.user_address)
FROM fip101_inscription_events fie
JOIN stake_indexer_registers reg ON reg.indexer_id = fie.indexer_id
WHERE scr.txid = fie.txid
  AND scr.indexer_id = fie.indexer_id
  AND scr.reward_type = $1
  AND fie.op = 'claim_reward'
  AND fie.indexer_id <> ''
  AND TRIM(reg.user_address) <> ''
  AND scr.user_address = fie.user_address
  AND scr.user_address <> TRIM(reg.user_address)`
	updateResult, err := execer.ExecContext(ctx, sqlText, StakeRewardTypeIndexer)
	if err != nil {
		return result, fmt.Errorf("fix indexer claim reward user addresses failed: %w", err)
	}
	if updateResult != nil {
		rowsAffected, err := updateResult.RowsAffected()
		if err != nil {
			return result, fmt.Errorf("read fixed indexer claim reward user address rows failed: %w", err)
		}
		result.Updated = rowsAffected
	}
	return result, nil
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
