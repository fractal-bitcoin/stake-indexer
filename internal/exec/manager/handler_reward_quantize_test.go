package indexer

import (
	"stake_indexer/conf"
	pgdb "stake_indexer/internal/component/pg"
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
	checkpoint := conf.StakeRewardCfg.Stage2StartHeight
	if shouldUseRewardTruncation(checkpoint - 1) {
		t.Fatalf("height before checkpoint should use rounding")
	}
	if !shouldUseRewardTruncation(checkpoint) {
		t.Fatalf("checkpoint height should use truncation")
	}
}

func TestResolveRewardProofWindow(t *testing.T) {
	origin := conf.StakeRewardCfg
	cfg := conf.DefaultConfig()
	cfg.ProofWindow = 20160
	cfg.DelaySubmitStage2StepBlocks = 50
	cfg.DelaySubmitStage2StepPercent = 20
	cfg.DelaySubmitStage3Blocks = []uint32{10, 20, 30, 40, 50, 60, 70}
	conf.StakeRewardCfg = cfg
	t.Cleanup(func() {
		conf.StakeRewardCfg = origin
	})

	stage2Checkpoint := conf.StakeRewardCfg.Stage2StartHeight
	stage3Checkpoint := conf.StakeRewardCfg.Stage3StartHeight
	if got := resolveRewardProofWindow(stage2Checkpoint - 1); got != 20160 {
		t.Fatalf("phase-1 proof window expected 20160, got %d", got)
	}
	if got := resolveRewardProofWindow(stage2Checkpoint); got != 250 {
		t.Fatalf("phase-2 proof window expected 250, got %d", got)
	}
	if got := resolveRewardProofWindow(stage3Checkpoint); got != 70 {
		t.Fatalf("phase-3 proof window expected 70, got %d", got)
	}
}

func TestResolveDelaySubmitStakePercent(t *testing.T) {
	checkpoint := conf.StakeRewardCfg.Stage2StartHeight
	proof := pgdb.StakeProof{
		ProveBlockHeight: 1000,
		Height:           1000,
		VerifyStatus:     pgdb.StakeProofVerifyValidDelayed,
	}

	if got := resolveDelaySubmitStakePercent(checkpoint-1, proof); got != 95 {
		t.Fatalf("phase-1 delayed stake percent expected 95, got %d", got)
	}

	cases := []struct {
		delay uint32
		want  uint64
	}{
		{99, 100},
		{100, 100},
		{101, 90},
		{199, 90},
		{200, 90},
		{201, 80},
		{900, 20},
		{901, 10},
		{1000, 10},
		{1001, 0},
		{1100, 0},
	}
	for _, tc := range cases {
		proof.Height = proof.ProveBlockHeight + tc.delay
		if got := resolveDelaySubmitStakePercent(checkpoint, proof); got != tc.want {
			t.Fatalf("stage-2 delay %d stake percent expected %d, got %d", tc.delay, tc.want, got)
		}
	}

	proof.VerifyStatus = pgdb.StakeProofVerifyValid
	proof.Height = proof.ProveBlockHeight + 1000
	if got := resolveDelaySubmitStakePercent(checkpoint, proof); got != 100 {
		t.Fatalf("non-delayed stake percent expected 100, got %d", got)
	}
}

func TestResolveDelaySubmitStakePercent_Stage3Tiers(t *testing.T) {
	origin := conf.StakeRewardCfg
	cfg := conf.DefaultConfig()
	cfg.Stage3StartHeight = cfg.Stage2StartHeight + 1000
	conf.StakeRewardCfg = cfg
	t.Cleanup(func() {
		conf.StakeRewardCfg = origin
	})

	proof := pgdb.StakeProof{
		ProveBlockHeight: 1000,
		VerifyStatus:     pgdb.StakeProofVerifyValidDelayed,
	}
	cases := []struct {
		delay uint32
		want  uint64
	}{
		{0, 100},
		{1, 100},
		{720, 100},
		{721, 95},
		{840, 95},
		{841, 87},
		{960, 87},
		{961, 75},
		{1080, 75},
		{1081, 59},
		{1200, 59},
		{1201, 35},
		{1320, 35},
		{1321, 10},
		{1439, 10},
		{1440, 10},
		{1441, 0},
	}
	for _, tc := range cases {
		proof.Height = proof.ProveBlockHeight + tc.delay
		if got := resolveDelaySubmitStakePercent(cfg.Stage3StartHeight, proof); got != tc.want {
			t.Fatalf("stage-3 delay %d stake percent expected %d, got %d", tc.delay, tc.want, got)
		}
	}
}

func TestResolveDelaySubmitStakePercent_Stage3BlocksConfigurable(t *testing.T) {
	origin := conf.StakeRewardCfg
	cfg := conf.DefaultConfig()
	cfg.Stage3StartHeight = cfg.Stage2StartHeight + 1000
	cfg.DelaySubmitStage3Blocks = []uint32{10, 20, 30, 40, 50, 60, 70}
	conf.StakeRewardCfg = cfg
	t.Cleanup(func() {
		conf.StakeRewardCfg = origin
	})

	proof := pgdb.StakeProof{
		ProveBlockHeight: 1000,
		VerifyStatus:     pgdb.StakeProofVerifyValidDelayed,
	}
	cases := []struct {
		delay uint32
		want  uint64
	}{
		{10, 100},
		{11, 95},
		{20, 95},
		{21, 87},
		{60, 35},
		{61, 10},
		{70, 10},
		{71, 0},
	}
	for _, tc := range cases {
		proof.Height = proof.ProveBlockHeight + tc.delay
		if got := resolveDelaySubmitStakePercent(cfg.Stage3StartHeight, proof); got != tc.want {
			t.Fatalf("stage-3 configurable delay %d stake percent expected %d, got %d", tc.delay, tc.want, got)
		}
	}
}

func TestResolveDelaySubmitStakePercent_Configurable(t *testing.T) {
	origin := conf.StakeRewardCfg
	conf.StakeRewardCfg.DelaySubmitStage2StepBlocks = 50
	conf.StakeRewardCfg.DelaySubmitStage2StepPercent = 20
	t.Cleanup(func() {
		conf.StakeRewardCfg = origin
	})

	checkpoint := conf.StakeRewardCfg.Stage2StartHeight
	proof := pgdb.StakeProof{
		ProveBlockHeight: 1000,
		Height:           1100,
		VerifyStatus:     pgdb.StakeProofVerifyValidDelayed,
	}
	if got := resolveDelaySubmitStakePercent(checkpoint, proof); got != 80 {
		t.Fatalf("expected configurable stage-2 percent 80, got %d", got)
	}
}

func TestResolveDelaySubmitStakePercent_EachBlockPenaltyAfterFirstBlock(t *testing.T) {
	origin := conf.StakeRewardCfg
	conf.StakeRewardCfg.DelaySubmitStage2StepBlocks = 1
	conf.StakeRewardCfg.DelaySubmitStage2StepPercent = 10
	t.Cleanup(func() {
		conf.StakeRewardCfg = origin
	})

	checkpoint := conf.StakeRewardCfg.Stage2StartHeight
	proof := pgdb.StakeProof{
		ProveBlockHeight: 1000,
		VerifyStatus:     pgdb.StakeProofVerifyValidDelayed,
	}

	cases := []struct {
		delay uint32
		want  uint64
	}{
		{1, 100},
		{2, 90},
		{3, 80},
		{10, 10},
		{11, 0},
	}
	for _, tc := range cases {
		proof.Height = proof.ProveBlockHeight + tc.delay
		if got := resolveDelaySubmitStakePercent(checkpoint, proof); got != tc.want {
			t.Fatalf("stage-2 delay %d stake percent expected %d, got %d", tc.delay, tc.want, got)
		}
	}
}

func TestResolveRewardStakePercentByIndexer_UsesRewardAllowlistWindow(t *testing.T) {
	origin := conf.StakeRewardCfg
	cfg := conf.DefaultConfig()
	cfg.IndexerAllowlistWindows = []conf.IndexerAllowlistWindow{
		{StartHeight: 100, EndHeight: 200, IndexerIDs: []string{"12:3"}},
	}
	conf.StakeRewardCfg = cfg
	t.Cleanup(func() {
		conf.StakeRewardCfg = origin
	})

	proofs := []pgdb.StakeProof{
		{IndexerID: "12:3", ProveBlockHeight: 100, Height: 100, VerifyStatus: pgdb.StakeProofVerifyValid},
		{IndexerID: "12:4", ProveBlockHeight: 100, Height: 100, VerifyStatus: pgdb.StakeProofVerifyValid},
	}

	inside := resolveRewardStakePercentByIndexer(100, proofs)
	if len(inside) != 1 || inside["12:3"] != 100 {
		t.Fatalf("inside allowlist window expected only listed indexer, got %#v", inside)
	}

	outside := resolveRewardStakePercentByIndexer(200, proofs)
	if len(outside) != 2 || outside["12:3"] != 100 || outside["12:4"] != 100 {
		t.Fatalf("outside allowlist window expected both indexers, got %#v", outside)
	}
}

func TestResolveRewardStakePercentByIndexer_UsesBestProof(t *testing.T) {
	origin := conf.StakeRewardCfg
	cfg := conf.DefaultConfig()
	conf.StakeRewardCfg = cfg
	t.Cleanup(func() {
		conf.StakeRewardCfg = origin
	})

	checkpoint := conf.StakeRewardCfg.Stage2StartHeight
	proofs := []pgdb.StakeProof{
		{IndexerID: "12:3", ProveBlockHeight: 1000, Height: 1200, VerifyStatus: pgdb.StakeProofVerifyValidDelayed},
		{IndexerID: "12:3", ProveBlockHeight: 1000, Height: 1000, VerifyStatus: pgdb.StakeProofVerifyValid},
		{IndexerID: "12:4", ProveBlockHeight: 1000, Height: 1400, VerifyStatus: pgdb.StakeProofVerifyValidDelayed},
		{IndexerID: "12:4", ProveBlockHeight: 1000, Height: 1200, VerifyStatus: pgdb.StakeProofVerifyValidDelayed},
	}

	got := resolveRewardStakePercentByIndexer(checkpoint, proofs)
	if got["12:3"] != 100 {
		t.Fatalf("indexer with valid proof should use 100 percent, got %#v", got)
	}
	if got["12:4"] != 90 {
		t.Fatalf("indexer with multiple delayed proofs should use best percent 90, got %#v", got)
	}
}
