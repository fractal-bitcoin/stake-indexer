package api

import (
	pgdb "stake_indexer/internal/component/pg"

	"github.com/gin-gonic/gin"
)

// GetRewardPoolStatus returns reward-pool accounting data.
func GetRewardPoolStatus(c *gin.Context) (rData ResponseData, err error) {
	liability, err := pgdb.GetStakeRewardPoolLiability(ctx)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}
	requiredReserveAmount := calculateClaimableRewardAmount(liability.AllocatedAmount, liability.ClaimedAmount, 0)
	postPendingRequiredReserveAmount := calculateClaimableRewardAmount(liability.AllocatedAmount, liability.ClaimedAmount, liability.PendingClaimedAmount)
	rData.Data = RewardPoolStatusResp{
		AllocatedAmount:                  liability.AllocatedAmount,
		ClaimedAmount:                    liability.ClaimedAmount,
		PendingClaimedAmount:             liability.PendingClaimedAmount,
		RequiredReserveAmount:            requiredReserveAmount,
		PostPendingRequiredReserveAmount: postPendingRequiredReserveAmount,
	}
	return rData, nil
}
