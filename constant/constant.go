package constant

const (
	HEIGHT_MUTIPLY = 1000000000
	MEMPOOL_HEIGHT = ^uint32(0)

	// REWARD_ALLOCATION_STAGE2_CHECKPOINT_HEIGHT switches reward allocation
	// from rounding to decimal truncation to avoid over-allocation.
	REWARD_ALLOCATION_STAGE2_CHECKPOINT_HEIGHT uint32 = 1784160
)
