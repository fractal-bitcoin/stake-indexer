package api

import (
	"encoding/json"
	"math"
	"testing"
)

func TestCalculateClaimableRewardAmount(t *testing.T) {
	tests := []struct {
		name                        string
		allocated, claimed, pending uint64
		want                        uint64
	}{
		{name: "unclaimed balance", allocated: 1000, claimed: 300, pending: 200, want: 500},
		{name: "fully claimed", allocated: 1000, claimed: 1000, pending: 0, want: 0},
		{name: "overclaimed balance is not negative", allocated: 1000, claimed: 1001, pending: 0, want: 0},
		{name: "pending claims can exhaust balance", allocated: 1000, claimed: 300, pending: 700, want: 0},
		{name: "overflowing claim total is not claimable", allocated: math.MaxUint64, claimed: math.MaxUint64, pending: 1, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateClaimableRewardAmount(tt.allocated, tt.claimed, tt.pending); got != tt.want {
				t.Fatalf("calculateClaimableRewardAmount(%d, %d, %d) = %d, want %d", tt.allocated, tt.claimed, tt.pending, got, tt.want)
			}
		})
	}
}

func TestUserRewardSummaryRespIncludesClaimTypeBreakdown(t *testing.T) {
	encoded, err := json.Marshal(UserRewardSummaryResp{
		ClaimedAmount:        100,
		ClaimedStakeReward:   40,
		ClaimedIndexerReward: 60,
	})
	if err != nil {
		t.Fatalf("marshal user reward summary response: %v", err)
	}

	var data map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &data); err != nil {
		t.Fatalf("decode user reward summary response: %v", err)
	}
	for field, want := range map[string]uint64{
		"claimed_amount":         100,
		"claimed_stake_reward":   40,
		"claimed_indexer_reward": 60,
	} {
		var got uint64
		if err := json.Unmarshal(data[field], &got); err != nil {
			t.Fatalf("decode %s: %v", field, err)
		}
		if got != want {
			t.Fatalf("%s = %d, want %d", field, got, want)
		}
	}
}
