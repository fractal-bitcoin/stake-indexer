package indexer

import (
	"testing"

	"stake_indexer/conf"
	pgdb "stake_indexer/internal/component/pg"
	protocolparser "stake_indexer/internal/parser/protocol"
)

func TestResolveBusinessInvalidFlags_StakeRequiresRegisteredIndexer(t *testing.T) {
	m := NewManager(conf.DefaultConfig())
	tx, payload := buildValidStakeTxAndPayload(t, "12:3")

	flags := m.resolveBusinessInvalidFlags(100, tx, payload)
	if flags != BizInvalidStakeRule {
		t.Fatalf("stake with unregistered indexer should be invalid, got flags=%d", flags)
	}
}

func TestResolveBusinessInvalidFlags_StakeAcceptsRegisteredIndexer(t *testing.T) {
	m := NewManager(conf.DefaultConfig())
	appendRegisteredIndexer(m, "12:3")
	tx, payload := buildValidStakeTxAndPayload(t, "12:3")

	flags := m.resolveBusinessInvalidFlags(100, tx, payload)
	if flags != BizInvalidNone {
		t.Fatalf("stake with registered indexer should be valid, got flags=%d", flags)
	}
}

func appendRegisteredIndexer(m *Manager, indexerID string) {
	m.WaitForUpsert.StakeIndexerRegisterList = append(m.WaitForUpsert.StakeIndexerRegisterList, pgdb.StakeIndexerRegister{
		IndexerID:   indexerID,
		UserAddress: "owner-address",
	})
}

func buildValidStakeTxAndPayload(t *testing.T, indexerID string) (*protocolparser.TxSnapshot, *protocolparser.OpReturnPayload) {
	t.Helper()

	const pubKeyHex = "79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798"
	const addressType = "2"

	stakeAddress, err := protocolparser.DeriveStakeAddress(indexerID, pubKeyHex, addressType, nil)
	if err != nil {
		t.Fatalf("derive stake address failed: %v", err)
	}

	tx := &protocolparser.TxSnapshot{
		TxID:  "stake-tx-1",
		TxIdx: 1,
		Outputs: []protocolparser.OutputSnapshot{
			{OutputIdx: 0, Satoshi: 1000, AddressKey: stakeAddress},
		},
	}
	payload := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagStake,
		Fields: map[string]string{
			protocolparser.OpFieldIndexerID:   indexerID,
			protocolparser.OpFieldActorPubKey: pubKeyHex,
			protocolparser.OpFieldAddressType: addressType,
		},
	}

	return tx, payload
}
