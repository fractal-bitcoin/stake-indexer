package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ResponseData is the unified response format (aligned with asset-relay).
type ResponseData struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg,omitempty"`
	Data interface{} `json:"data,omitempty"`
}

// wrapHandler standardizes handler execution and JSON response writing.
func wrapHandler(fn func(*gin.Context) (ResponseData, error)) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		rData, _ := fn(ctx)
		ctx.JSON(http.StatusOK, rData)
	}
}

var (
	GetIndexersHandler              = wrapHandler(GetIndexers)
	GetIndexerByAddressHandler      = wrapHandler(GetIndexerByAddress)
	GetIndexerStatusHandler         = wrapHandler(GetIndexerStatus)
	GetIndexerStakersHandler        = wrapHandler(GetIndexerStakers)
	GetIndexerProofsHandler         = wrapHandler(GetIndexerProofs)
	GetUserStakingsHandler          = wrapHandler(GetUserStakings)
	GetUserRewardRecordsHandler     = wrapHandler(GetUserRewardRecords)
	GetUserRewardSummaryHandler     = wrapHandler(GetUserRewardSummary)
	GetRewardPoolStatusHandler      = wrapHandler(GetRewardPoolStatus)
	GetStakeRewardSyncStatusHandler = wrapHandler(GetStakeRewardSyncStatus)
	GetMempoolProtocolTxsHandler    = wrapHandler(GetMempoolProtocolTxs)
)
