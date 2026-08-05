package api

import (
	"encoding/json"
	"testing"
)

func TestRewardPoolStatusRespOmitsOnchainBalanceFields(t *testing.T) {
	encoded, err := json.Marshal(RewardPoolStatusResp{})
	if err != nil {
		t.Fatalf("marshal reward pool status response: %v", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(encoded, &data); err != nil {
		t.Fatalf("decode reward pool status response: %v", err)
	}
	for _, field := range []string{
		"pool_addresses",
		"onchain_balance_amount",
		"balance_height",
		"balance_block_hash",
		"balance_observed_at",
	} {
		if _, ok := data[field]; ok {
			t.Fatalf("unexpected on-chain balance field %q", field)
		}
	}
}
