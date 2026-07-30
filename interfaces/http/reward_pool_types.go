package api

type RewardPoolStatusResp struct {
	PoolAddresses                    []string `json:"pool_addresses"`
	AllocatedAmount                  uint64   `json:"allocated_amount"`
	ClaimedAmount                    uint64   `json:"claimed_amount"`
	PendingClaimedAmount             uint64   `json:"pending_claimed_amount"`
	RequiredReserveAmount            uint64   `json:"required_reserve_amount"`
	PostPendingRequiredReserveAmount uint64   `json:"post_pending_required_reserve_amount"`
	OnchainBalanceAmount             uint64   `json:"onchain_balance_amount"`
	BalanceHeight                    uint32   `json:"balance_height"`
	BalanceBlockHash                 string   `json:"balance_block_hash"`
	BalanceObservedAt                int64    `json:"balance_observed_at"`
}
