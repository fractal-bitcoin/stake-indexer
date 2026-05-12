package indexer

import (
	"testing"

	pgdb "stake_indexer/internal/component/pg"
	protocolparser "stake_indexer/internal/parser/protocol"
)

func TestMergeMempoolFirstBindIndexerDeltas_UsesAddressDeltasNet(t *testing.T) {
	confirmed := map[string]StakeAddressInfo{}
	addressDeltas := map[string]int64{
		"s2": 50,
		"s3": -20,
	}
	indexerDeltas := map[string]int64{}
	events := []pgdb.StakeMempoolEvent{
		{Op: protocolparser.TagStake, IndexerID: "i1", StakeAddress: "s1", Amount: 100, BizInvalidFlags: 0},
		{Op: protocolparser.TagStake, IndexerID: "i1", StakeAddress: "s2", Amount: 999, BizInvalidFlags: 0},
		{Op: protocolparser.TagStake, IndexerID: "i1", StakeAddress: "s3", Amount: 999, BizInvalidFlags: 0},
		{Op: protocolparser.TagStake, IndexerID: "i1", StakeAddress: "bad", Amount: 999, BizInvalidFlags: 1},
	}

	mergeMempoolFirstBindIndexerDeltas(confirmed, addressDeltas, indexerDeltas, events)

	if got := indexerDeltas["i1"]; got != 30 {
		t.Fatalf("expected indexer delta 30, got %d", got)
	}
}

func TestBuildMempoolFirstBindUserAmountsByIndexer_UsesNetPositiveOnly(t *testing.T) {
	confirmed := map[string]StakeAddressInfo{
		"confirmed": {Address: "u1", IndexerID: "i1"},
	}
	addressDeltas := map[string]int64{
		"s1": 0,
		"s2": 70,
		"s3": -10,
	}
	events := []pgdb.StakeMempoolEvent{
		{Op: protocolparser.TagStake, IndexerID: "i1", UserAddress: "u1", StakeAddress: "s1", Amount: 100, BizInvalidFlags: 0},
		{Op: protocolparser.TagStake, IndexerID: "i1", UserAddress: "u1", StakeAddress: "s2", Amount: 100, BizInvalidFlags: 0},
		{Op: protocolparser.TagStake, IndexerID: "i1", UserAddress: "u1", StakeAddress: "s3", Amount: 100, BizInvalidFlags: 0},
		{Op: protocolparser.TagStake, IndexerID: "i1", UserAddress: "u1", StakeAddress: "confirmed", Amount: 100, BizInvalidFlags: 0},
		{Op: protocolparser.TagStake, IndexerID: "i1", UserAddress: "u1", StakeAddress: "bad", Amount: 100, BizInvalidFlags: 2},
	}

	got := buildMempoolFirstBindUserAmountsByIndexer(confirmed, addressDeltas, events)
	userMap := got["i1"]
	if userMap == nil {
		t.Fatalf("expected indexer user map")
	}
	if amt := userMap["u1"]; amt != 70 {
		t.Fatalf("expected user amount 70, got %d", amt)
	}
}
