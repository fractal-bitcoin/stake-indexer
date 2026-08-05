package api

type RewardPoolStatusResp struct {
	AllocatedAmount                  uint64 `json:"allocated_amount"`
	ClaimedAmount                    uint64 `json:"claimed_amount"`
	PendingClaimedAmount             uint64 `json:"pending_claimed_amount"`
	RequiredReserveAmount            uint64 `json:"required_reserve_amount"`
	PostPendingRequiredReserveAmount uint64 `json:"post_pending_required_reserve_amount"`
}
