package api

type PageReqParams struct {
	Start int `form:"start"`
	Limit int `form:"limit"`
}

type ListIndexersResp struct {
	Total       int               `json:"total"`
	TotalStaked uint64            `json:"total_staked"`
	Start       int               `json:"start"`
	Detail      []IndexerListItem `json:"detail"`
}

type IndexerListItem struct {
	IndexerID                        string   `json:"indexer_id"`
	Name                             string   `json:"name"`
	RewardAddress                    string   `json:"reward_address"`
	UserAddress                      string   `json:"user_address"`
	IndexRatio                       float64  `json:"index_ratio"`
	PendingEffectiveIndexRatio       *float64 `json:"pending_effective_index_ratio,omitempty"`
	PendingEffectiveIndexRatioHeight *uint32  `json:"pending_effective_index_ratio_height,omitempty"`
	TotalStaked                      uint64   `json:"total_staked"`
	StakeRatio                       float64  `json:"stake_ratio"`
	AllocatedReward                  uint64   `json:"allocated_reward"`
	Pending                          bool     `json:"pending,omitempty"`
}

type StakerItem struct {
	UserAddress string `json:"user_address"`
	Amount      uint64 `json:"amount"`
	Pending     bool   `json:"pending,omitempty"`
}

type ListIndexerStakersResp struct {
	IndexerID string       `json:"indexer_id"`
	Name      string       `json:"name"`
	Total     int          `json:"total"`
	Start     int          `json:"start"`
	Detail    []StakerItem `json:"detail"`
}

type ProofItem struct {
	IndexerID        string `json:"indexer_id"`
	ProveBlockHeight uint32 `json:"prove_block_height"`
	ProveDataHash    string `json:"prove_data_hash"`
	TxID             string `json:"txid"`
	Height           uint32 `json:"height"`
	TxIdx            uint32 `json:"tx_idx"`
	VerifyStatus     int16  `json:"verify_status"`
	Pending          bool   `json:"pending,omitempty"`
}

type ListIndexerProofsResp struct {
	IndexerID string      `json:"indexer_id"`
	Name      string      `json:"name"`
	Total     int         `json:"total"`
	Start     int         `json:"start"`
	Detail    []ProofItem `json:"detail"`
}

type StakeRewardSyncStatusResp struct {
	Height      uint32 `json:"height"`
	BlockReward uint64 `json:"block_reward"`
	BlockHash   string `json:"block_hash"`
}

type UserStakingItem struct {
	IndexerID    string `json:"indexer_id"`
	Name         string `json:"name"`
	StakeAddress string `json:"stake_address"`
	Amount       uint64 `json:"amount"`
	Rewards      uint64 `json:"rewards"`
	Pending      bool   `json:"pending,omitempty"`
}

type ListUserStakingsResp struct {
	Total        int               `json:"total"`
	Start        int               `json:"start"`
	TotalRewards uint64            `json:"total_rewards"`
	Detail       []UserStakingItem `json:"detail"`
}

type UserRewardRecordItem struct {
	UserAddress          string  `json:"user_address"`
	IndexerID            string  `json:"indexer_id"`
	StakeAddress         string  `json:"stake_address"`
	RewardType           string  `json:"reward_type"`
	Height               uint32  `json:"height"`
	StakeAmountSnapshot  uint64  `json:"stake_amount_snapshot"`
	IndexerTotalStake    uint64  `json:"indexer_total_stake"`
	IndexerEffectivePct  float64 `json:"indexer_effective_percent"`
	StakeAmountEffective uint64  `json:"stake_amount_effective"`
	PlatformTotalStake   uint64  `json:"platform_total_stake"`
	TotalEffectiveStake  uint64  `json:"total_effective_stake"`
	ReleasePercent       float64 `json:"release_percent"`
	BlockRewardAmount    uint64  `json:"block_reward_amount"`
	IndexerRatio         float64 `json:"indexer_ratio"`
	AllocateAmount       uint64  `json:"allocate_amount"`
}
type ListUserRewardRecordsResp struct {
	Total  int                    `json:"total"`
	Start  int                    `json:"start"`
	Detail []UserRewardRecordItem `json:"detail"`
}

type UserRewardSummaryResp struct {
	UserAddress          string `json:"user_address"`
	AllocatedAmount      uint64 `json:"allocated_amount"`
	ClaimedAmount        uint64 `json:"claimed_amount"`
	PendingClaimedAmount uint64 `json:"pending_claimed_amount"`
	ClaimableAmount      uint64 `json:"claimable_amount"`
	RequestedAmount      uint64 `json:"requested_amount"`
	CanClaim             bool   `json:"can_claim"`
}

type IndexerStatusResp struct {
	TotalIndexers               int    `json:"total_indexers"`
	TotalStaked                 uint64 `json:"total_staked"`
	LatestBlockHeight           uint32 `json:"latest_block_height"`
	StakeRewardSyncHeight       uint32 `json:"stake_reward_sync_height"`
	LatestAllocatedRewardHeight uint32 `json:"latest_allocated_reward_height"`
	LatestAllocatedRewardAmount uint64 `json:"latest_allocated_reward_amount"`
	PendingRewardSyncHeight     uint32 `json:"pending_reward_sync_height"`
	PendingRewardTotalAmount    uint64 `json:"pending_reward_total_amount"`
}

type MempoolProtocolTxItem struct {
	TxID                             string   `json:"txid"`
	Op                               string   `json:"op"`
	Height                           int64    `json:"height"`
	InscriptionContent               string   `json:"inscription_content"`
	IndexerID                        string   `json:"indexer_id"`
	UserAddress                      string   `json:"user_address"`
	RewardAddress                    string   `json:"reward_address"`
	StakeAddress                     string   `json:"stake_address"`
	Amount                           uint64   `json:"amount"`
	IndexRatio                       float64  `json:"index_ratio"`
	PendingEffectiveIndexRatio       *float64 `json:"pending_effective_index_ratio,omitempty"`
	PendingEffectiveIndexRatioHeight *uint32  `json:"pending_effective_index_ratio_height,omitempty"`
	IndexerName                      string   `json:"indexer_name"`
	ProveBlockHeight                 uint32   `json:"prove_block_height"`
	ProveDataHash                    string   `json:"prove_data_hash"`
	TxIdx                            uint32   `json:"tx_idx"`
}

type ListMempoolProtocolTxsResp struct {
	Total  int                     `json:"total"`
	Start  int                     `json:"start"`
	Detail []MempoolProtocolTxItem `json:"detail"`
}
