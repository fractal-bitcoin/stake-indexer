package indexer

import (
	"testing"

	"stake_indexer/conf"
	protocolparser "stake_indexer/internal/parser/protocol"
)

func TestResolveBusinessInvalidFlags_RegisterOwnerOnlyOneIndexer(t *testing.T) {
	m := NewManager(conf.DefaultConfig())
	m.resetRegisterOwnerSeen()

	tx1 := &protocolparser.TxSnapshot{TxID: "tx-1", TxIdx: 1}
	tx2 := &protocolparser.TxSnapshot{TxID: "tx-2", TxIdx: 2}

	payload1 := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagRegister,
		Fields: map[string]string{
			protocolparser.OpFieldActorAddr:  "owner-address-1",
			protocolparser.OpFieldRewardAddr: "bc1qrewardaddr1111111111111111111111111111111111",
			protocolparser.OpFieldIndexRatio: "0.10000000",
		},
	}
	payload2 := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagRegister,
		Fields: map[string]string{
			protocolparser.OpFieldActorAddr:  "owner-address-1",
			protocolparser.OpFieldRewardAddr: "bc1qrewardaddr2222222222222222222222222222222222",
			protocolparser.OpFieldIndexRatio: "0.12000000",
		},
	}

	if flags := m.resolveBusinessInvalidFlags(100, tx1, payload1); flags != BizInvalidNone {
		t.Fatalf("first register should be valid, got flags=%d", flags)
	}
	if flags := m.resolveBusinessInvalidFlags(100, tx2, payload2); flags != BizInvalidRegisterRule {
		t.Fatalf("second register with same owner should be invalid, got flags=%d", flags)
	}
}

func TestResolveBusinessInvalidFlags_RegisterRejectsCommissionAboveLimit(t *testing.T) {
	m := NewManager(conf.DefaultConfig())
	m.resetRegisterOwnerSeen()

	payload := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagRegister,
		Fields: map[string]string{
			protocolparser.OpFieldActorAddr:  "owner-high-ratio",
			protocolparser.OpFieldRewardAddr: "bc1qrewardaddr8888888888888888888888888888888888",
			protocolparser.OpFieldIndexRatio: "0.16000000",
		},
	}
	flags := m.resolveBusinessInvalidFlags(100, &protocolparser.TxSnapshot{TxID: "tx-high-ratio", TxIdx: 8}, payload)
	if flags != BizInvalidRegisterRule {
		t.Fatalf("register with ratio above 15%% should be invalid, got flags=%d", flags)
	}
}

func TestResolveBusinessInvalidFlags_RegisterOwnerCaseSensitive(t *testing.T) {
	m := NewManager(conf.DefaultConfig())
	m.resetRegisterOwnerSeen()

	tx1 := &protocolparser.TxSnapshot{TxID: "tx-1", TxIdx: 1}
	tx2 := &protocolparser.TxSnapshot{TxID: "tx-2", TxIdx: 2}

	payload1 := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagRegister,
		Fields: map[string]string{
			protocolparser.OpFieldActorAddr:  "Owner-Address-Case",
			protocolparser.OpFieldRewardAddr: "bc1qrewardaddr3333333333333333333333333333333333",
			protocolparser.OpFieldIndexRatio: "0.13000000",
		},
	}
	payload2 := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagRegister,
		Fields: map[string]string{
			protocolparser.OpFieldActorAddr:  "owner-address-case",
			protocolparser.OpFieldRewardAddr: "bc1qrewardaddr4444444444444444444444444444444444",
			protocolparser.OpFieldIndexRatio: "0.14000000",
		},
	}

	if flags := m.resolveBusinessInvalidFlags(100, tx1, payload1); flags != BizInvalidNone {
		t.Fatalf("first register should be valid, got flags=%d", flags)
	}
	if flags := m.resolveBusinessInvalidFlags(100, tx2, payload2); flags != BizInvalidNone {
		t.Fatalf("second register with different-case owner should be valid in case-sensitive mode, got flags=%d", flags)
	}
}

func TestResolveBusinessInvalidFlags_RegisterIgnoresAllowlistWindow(t *testing.T) {
	m := NewManager(conf.DefaultConfig())
	m.resetRegisterOwnerSeen()

	backup := conf.StakeRewardCfg
	t.Cleanup(func() {
		conf.StakeRewardCfg = backup
	})

	cfg := conf.DefaultConfig()
	cfg.IndexerAllowlistWindows = []conf.IndexerAllowlistWindow{
		{StartHeight: 100, EndHeight: 200, IndexerIDs: []string{"100:1"}},
	}
	conf.StakeRewardCfg = cfg

	payload := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagRegister,
		Fields: map[string]string{
			protocolparser.OpFieldActorAddr:  "owner-unlisted",
			protocolparser.OpFieldRewardAddr: "bc1qrewardaddr5555555555555555555555555555555555",
			protocolparser.OpFieldIndexRatio: "0.15000000",
		},
	}
	if flags := m.resolveFastBlockBusinessInvalidFlags(100, &protocolparser.TxSnapshot{TxID: "tx-2", TxIdx: 2}, payload); flags != BizInvalidNone {
		t.Fatalf("register should ignore reward allowlist, got flags=%d", flags)
	}
}

func TestResolveBusinessInvalidFlags_RegisterIgnoresRewardAllowlist(t *testing.T) {
	m := NewManager(conf.DefaultConfig())
	m.resetRegisterOwnerSeen()

	backup := conf.StakeRewardCfg
	t.Cleanup(func() {
		conf.StakeRewardCfg = backup
	})

	cfg := conf.DefaultConfig()
	cfg.IndexerAllowlistWindows = []conf.IndexerAllowlistWindow{
		{StartHeight: 100, EndHeight: 200, IndexerIDs: []string{"100:1"}},
	}
	conf.StakeRewardCfg = cfg

	payload := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagRegister,
		Fields: map[string]string{
			protocolparser.OpFieldActorAddr:  "owner-mempool-like",
			protocolparser.OpFieldRewardAddr: "bc1qrewardaddr7777777777777777777777777777777777",
			protocolparser.OpFieldIndexRatio: "0.12000000",
		},
	}
	if flags := m.resolveBusinessInvalidFlags(100, &protocolparser.TxSnapshot{TxID: "tx-3", TxIdx: 2}, payload); flags != BizInvalidNone {
		t.Fatalf("register allowlist should be ignored without enforcement, got flags=%d", flags)
	}
}
