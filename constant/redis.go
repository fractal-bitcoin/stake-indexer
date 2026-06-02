package constant

const (
	// Shared task progress hash in redis.
	TASK_INFO_KEYNAME = "info"

	// Realtime (syncIndexer) committed block hash/height cursor fields.
	// Keep legacy names for compatibility with existing readers.
	TASK_BLOCK        = "block"
	TASK_BLOCK_HEIGHT = "block_height"

	// Fast pipeline progress fields.
	TASK_UTXO                     = "block_height_utxo"
	TASK_SNAPSHOT_CONSUMER_HEIGHT = "snapshot:consumer_height"
	TASK_PENDING_CONSUMER_HEIGHT  = "pending:consumer_height"
	TASK_UTXO_TOTAL               = "utxo_total"

	// Realtime and snapshot balance key prefixes.
	BALANCE_REALTIME_PREFIX         = "b"
	BALANCE_SNAPSHOT_PREFIX         = "balance:snapshot:"
	BALANCE_PENDING_SNAPSHOT_PREFIX = "balance:pending:snapshot:"

	INDEXER_ADDR_DELTA_PREFIX = "si:addr_delta:"

	// Indexer stake amount zset: key=indexer_id, member=user_address, score=stake_amount
	INDEXER_STAKE_ZSET_PREFIX = "si:indexer_stake:"
	// Indexer total stake: key=indexer_id, value=total_amount
	INDEXER_STAKE_TOTAL_PREFIX = "si:indexer_stake_total:"
	INDEXER_UNDO_NEW_PREFIX    = "si:undo:new:"
	INDEXER_UNDO_SPENT_PREFIX  = "si:undo:spent:"
	INDEXER_BLOCK_HASH_PREFIX  = "si:block:hash:"

	// UTXO key prefix in redis.
	UTXO_KEY_PREFIX = "u"

	// Stake-related redis keys.
	REDIS_STAKE_INDEXER_INFO_PREFIX                                   = "s:indexer:info:"
	REDIS_STAKE_INDEXER_RATIO_SNAPSHOT_KEY                            = "s:indexer:ratio:snapshot"
	REDIS_STAKE_INDEXER_DELAYED_COMMISSION_KEY                        = "s:indexer:delayed_commission"
	REDIS_STAKE_INDEXER_RATIO_SNAPSHOT_DELAYED_COMMISSION_KEY         = "s:indexer:ratio:snapshot:delayed_commission"
	REDIS_STAKE_PENDING_INDEXER_INFO_PREFIX                           = "s:pending:indexer:info:"
	REDIS_STAKE_PENDING_INDEXER_DELAYED_COMMISSION_KEY                = "s:pending:indexer:delayed_commission"
	REDIS_STAKE_PENDING_INDEXER_RATIO_SNAPSHOT_KEY                    = "s:pending:indexer:ratio:snapshot"
	REDIS_STAKE_PENDING_INDEXER_RATIO_SNAPSHOT_DELAYED_COMMISSION_KEY = "s:pending:indexer:ratio:snapshot:delayed_commission"
	REDIS_STAKE_ADDRESS_REWARDS_PREFIX                                = "s:stake:rewards:"
	REDIS_STAKE_ADDRESS_REWARDS_TOTAL_PREFIX                          = "s:stake:rewards_total:"
	REDIS_INDEXER_ADDRESS_REWARDS_PREFIX                              = "s:indexer:rewards:"
	REDIS_INDEXER_ADDRESS_REWARDS_TOTAL_PREFIX                        = "s:indexer:rewards_total:"
	REDIS_STAKE_PENDING_ADDRESS_REWARDS_PREFIX                        = "s:pending:stake:rewards:"
	REDIS_STAKE_PENDING_ADDRESS_REWARDS_TOTAL_PREFIX                  = "s:pending:stake:rewards_total:"
	REDIS_INDEXER_PENDING_ADDRESS_REWARDS_PREFIX                      = "s:pending:indexer:rewards:"
	REDIS_INDEXER_PENDING_ADDRESS_REWARDS_TOTAL_PREFIX                = "s:pending:indexer:rewards_total:"
	REDIS_STAKE_MEMPOOL_BALANCE_DELTA_KEY                             = "s:stake:mempool:balance_delta"
	REDIS_STAKE_MEMPOOL_INDEXER_DELTA_KEY                             = "s:stake:mempool:indexer_delta"
	REDIS_STAKE_MEMPOOL_INDEXER_STAKER_DELTA_PREFIX                   = "s:stake:mempool:indexer_staker_delta:"
	REDIS_STAKE_MEMPOOL_INDEXER_STAKER_DELTA_INDEXERS_KEY             = "s:stake:mempool:indexer_staker_delta:indexers"
	REDIS_STAKE_MEMPOOL_INDEXER_STAKERS_PREFIX                        = "s:stake:mempool:indexer_stakers:"
	REDIS_STAKE_MEMPOOL_INDEXER_STAKERS_PENDING_PREFIX                = "s:stake:mempool:indexer_stakers:pending:"
	REDIS_STAKE_MEMPOOL_INDEXER_STAKERS_INDEXERS_KEY                  = "s:stake:mempool:indexer_stakers:indexers"
	REDIS_STAKE_INDEXER_STATUS_KEY                                    = "s:indexer:status"
)

func GetRealtimeBalanceKey(address string) string {
	return BALANCE_REALTIME_PREFIX + address
}

func GetSnapshotBalanceKey(address string) string {
	return BALANCE_SNAPSHOT_PREFIX + address
}

func GetPendingSnapshotBalanceKey(address string) string {
	return BALANCE_PENDING_SNAPSHOT_PREFIX + address
}

func GetIndexerStakeZsetKey(indexerID string) string {
	return INDEXER_STAKE_ZSET_PREFIX + indexerID
}

func GetIndexerStakeTotalKey(indexerID string) string {
	return INDEXER_STAKE_TOTAL_PREFIX + indexerID
}

// GetIndexerInfoKey returns the indexer info hash key.
func GetIndexerInfoKey(indexerID string) string {
	return REDIS_STAKE_INDEXER_INFO_PREFIX + indexerID
}

func GetPendingIndexerInfoKey(indexerID string) string {
	return REDIS_STAKE_PENDING_INDEXER_INFO_PREFIX + indexerID
}

// GetStakeRewardsKey returns the per-address reward zset key.
func GetStakeRewardsKey(userAddress string) string {
	return REDIS_STAKE_ADDRESS_REWARDS_PREFIX + userAddress
}

func GetPendingStakeRewardsKey(userAddress string) string {
	return REDIS_STAKE_PENDING_ADDRESS_REWARDS_PREFIX + userAddress
}

func GetStakeRewardsTotalKey(userAddress string) string {
	return REDIS_STAKE_ADDRESS_REWARDS_TOTAL_PREFIX + userAddress
}

func GetPendingStakeRewardsTotalKey(userAddress string) string {
	return REDIS_STAKE_PENDING_ADDRESS_REWARDS_TOTAL_PREFIX + userAddress
}

func GetIndexerRewardsKey(userAddress string) string {
	return REDIS_INDEXER_ADDRESS_REWARDS_PREFIX + userAddress
}

func GetPendingIndexerRewardsKey(userAddress string) string {
	return REDIS_INDEXER_PENDING_ADDRESS_REWARDS_PREFIX + userAddress
}

func GetIndexerRewardsTotalKey(userAddress string) string {
	return REDIS_INDEXER_ADDRESS_REWARDS_TOTAL_PREFIX + userAddress
}

func GetPendingIndexerRewardsTotalKey(userAddress string) string {
	return REDIS_INDEXER_PENDING_ADDRESS_REWARDS_TOTAL_PREFIX + userAddress
}

func GetStakeMempoolBalanceDeltaKey() string {
	return REDIS_STAKE_MEMPOOL_BALANCE_DELTA_KEY
}

func GetStakeMempoolIndexerDeltaKey() string {
	return REDIS_STAKE_MEMPOOL_INDEXER_DELTA_KEY
}

func GetStakeMempoolIndexerStakerDeltaKey(indexerID string) string {
	return REDIS_STAKE_MEMPOOL_INDEXER_STAKER_DELTA_PREFIX + indexerID
}

func GetStakeMempoolIndexerStakerDeltaIndexersKey() string {
	return REDIS_STAKE_MEMPOOL_INDEXER_STAKER_DELTA_INDEXERS_KEY
}

func GetStakeMempoolIndexerStakersKey(indexerID string) string {
	return REDIS_STAKE_MEMPOOL_INDEXER_STAKERS_PREFIX + indexerID
}

func GetStakeMempoolIndexerStakersPendingKey(indexerID string) string {
	return REDIS_STAKE_MEMPOOL_INDEXER_STAKERS_PENDING_PREFIX + indexerID
}

func GetStakeMempoolIndexerStakersIndexersKey() string {
	return REDIS_STAKE_MEMPOOL_INDEXER_STAKERS_INDEXERS_KEY
}

func GetStakeIndexerStatusKey() string {
	return REDIS_STAKE_INDEXER_STATUS_KEY
}

func GetUtxoKey(outpointKey string) string {
	return UTXO_KEY_PREFIX + outpointKey
}
