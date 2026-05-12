package indexer

import (
	"testing"

	"stake_indexer/conf"
	protocolparser "stake_indexer/internal/parser/protocol"
)

func TestBuildStakeProof_DoesNotRequireOwnerActor(t *testing.T) {
	cfg := conf.DefaultConfig()
	m := NewManager(cfg)
	appendRegisteredIndexer(m, "12:3")

	tx := &protocolparser.TxSnapshot{
		TxID:  "proof-tx-1",
		TxIdx: 1,
	}
	payload := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagProveStake,
		Fields: map[string]string{
			protocolparser.OpFieldIndexerID:   "12:3",
			protocolparser.OpFieldBlockHeight: "9999",
			protocolparser.OpFieldBlockHash:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			protocolparser.OpFieldActorAddr:   "not-owner-address",
		},
	}

	item, err := m.buildStakeProof(100, tx, payload)
	if err != nil {
		t.Fatalf("buildStakeProof returned error: %v", err)
	}
	if item == nil {
		t.Fatalf("buildStakeProof returned nil item")
	}
}

func TestResolveBusinessInvalidFlags_ProveAllowsFutureHeight(t *testing.T) {
	cfg := conf.DefaultConfig()
	m := NewManager(cfg)
	appendRegisteredIndexer(m, "12:3")

	tx := &protocolparser.TxSnapshot{
		TxID:  "proof-tx-2",
		TxIdx: 2,
	}
	payload := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagProveStake,
		Fields: map[string]string{
			protocolparser.OpFieldIndexerID:   "12:3",
			protocolparser.OpFieldBlockHeight: "500000",
			protocolparser.OpFieldBlockHash:   "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}

	flags := m.resolveBusinessInvalidFlags(100, tx, payload)
	if flags != BizInvalidNone {
		t.Fatalf("unexpected invalid flags: %d", flags)
	}
}

func TestResolveBusinessInvalidFlags_ProveRequiresRegisteredIndexer(t *testing.T) {
	cfg := conf.DefaultConfig()
	m := NewManager(cfg)

	tx := &protocolparser.TxSnapshot{
		TxID:  "proof-tx-3",
		TxIdx: 3,
	}
	payload := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagProveStake,
		Fields: map[string]string{
			protocolparser.OpFieldIndexerID:   "12:3",
			protocolparser.OpFieldBlockHeight: "500000",
			protocolparser.OpFieldBlockHash:   "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		},
	}

	flags := m.resolveBusinessInvalidFlags(100, tx, payload)
	if flags != BizInvalidProofRule {
		t.Fatalf("prove with unregistered indexer should be invalid, got flags=%d", flags)
	}
}
