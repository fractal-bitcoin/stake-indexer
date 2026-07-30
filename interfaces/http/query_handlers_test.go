package api

import (
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
