package indexer

import (
	"stake_indexer/constant"
	"testing"
)

func TestQuantizeReward(t *testing.T) {
	if got := quantizeReward(1.5, false); got != 2 {
		t.Fatalf("rounding mode expected 2, got %d", got)
	}
	if got := quantizeReward(1.5, true); got != 1 {
		t.Fatalf("truncation mode expected 1, got %d", got)
	}
	if got := quantizeReward(0.9999999999, true); got != 1 {
		t.Fatalf("truncation with epsilon expected 1, got %d", got)
	}
}

func TestShouldUseRewardTruncation(t *testing.T) {
	checkpoint := constant.REWARD_ALLOCATION_STAGE2_CHECKPOINT_HEIGHT
	if shouldUseRewardTruncation(checkpoint - 1) {
		t.Fatalf("height before checkpoint should use rounding")
	}
	if !shouldUseRewardTruncation(checkpoint) {
		t.Fatalf("checkpoint height should use truncation")
	}
}
