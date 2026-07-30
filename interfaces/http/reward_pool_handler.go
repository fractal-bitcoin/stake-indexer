package api

import (
	"fmt"

	"stake_indexer/conf"
	"stake_indexer/internal/component/node"
	pgdb "stake_indexer/internal/component/pg"

	"github.com/gin-gonic/gin"
)

// GetRewardPoolStatus returns reward-pool accounting data and its confirmed UTXO balance.
func GetRewardPoolStatus(c *gin.Context) (rData ResponseData, err error) {
	poolAddresses := append([]string(nil), conf.StakeRewardCfg.RewardClaimSenderAddressKeys...)
	if len(poolAddresses) == 0 {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "reward claim sender address keys not configured"
		return rData, fmt.Errorf("reward claim sender address keys not configured")
	}

	liability, err := pgdb.GetStakeRewardPoolLiability(ctx)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}
	balance, err := node.GetAddressBalanceRPC(poolAddresses, conf.StakeRewardCfg.RewardPoolBalanceCacheTTL)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	requiredReserveAmount := calculateClaimableRewardAmount(liability.AllocatedAmount, liability.ClaimedAmount, 0)
	postPendingRequiredReserveAmount := calculateClaimableRewardAmount(liability.AllocatedAmount, liability.ClaimedAmount, liability.PendingClaimedAmount)
	rData.Data = RewardPoolStatusResp{
		PoolAddresses:                    poolAddresses,
		AllocatedAmount:                  liability.AllocatedAmount,
		ClaimedAmount:                    liability.ClaimedAmount,
		PendingClaimedAmount:             liability.PendingClaimedAmount,
		RequiredReserveAmount:            requiredReserveAmount,
		PostPendingRequiredReserveAmount: postPendingRequiredReserveAmount,
		OnchainBalanceAmount:             balance.Satoshi,
		BalanceHeight:                    balance.Height,
		BalanceBlockHash:                 balance.BlockHash,
		BalanceObservedAt:                balance.ObservedAt.Unix(),
	}
	return rData, nil
}
