package indexer

import (
	"stake_indexer/conf"
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

func TestResolveRewardProofWindow(t *testing.T) {
	origin := conf.StakeRewardCfg
	conf.StakeRewardCfg.ProofWindow = 20160
	t.Cleanup(func() {
		conf.StakeRewardCfg = origin
	})

	checkpoint := constant.REWARD_ALLOCATION_STAGE2_CHECKPOINT_HEIGHT
	if got := resolveRewardProofWindow(checkpoint - 1); got != 20160 {
		t.Fatalf("phase-1 proof window expected 20160, got %d", got)
	}
	if got := resolveRewardProofWindow(checkpoint); got != constant.REWARD_ALLOCATION_STAGE2_PROOF_WINDOW {
		t.Fatalf("phase-2 proof window expected %d, got %d", constant.REWARD_ALLOCATION_STAGE2_PROOF_WINDOW, got)
	}
}
