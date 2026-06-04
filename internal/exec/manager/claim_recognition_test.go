package indexer

import (
	"strings"
	"testing"

	"stake_indexer/conf"
	protocolparser "stake_indexer/internal/parser/protocol"
)

func TestBuildStakeClaimedReward_UsesFirstOutput(t *testing.T) {
	m := NewManager(conf.DefaultConfig())
	tx := &protocolparser.TxSnapshot{
		TxID:  "tx-1",
		TxIdx: 7,
		Outputs: []protocolparser.OutputSnapshot{
			{OutputIdx: 0, AddressKey: "user-receiver-address", Satoshi: 12345},
		},
	}
	payload := &protocolparser.OpReturnPayload{
		Tag:    protocolparser.TagPledgedReward,
		Fields: map[string]string{},
	}

	item, err := m.buildStakeClaimedReward(100, tx, payload)
	if err != nil {
		t.Fatalf("buildStakeClaimedReward returned error: %v", err)
	}
	if item == nil {
		t.Fatalf("buildStakeClaimedReward returned nil item")
	}
	if item.UserAddress != "user-receiver-address" {
		t.Fatalf("unexpected recipient address: %s", item.UserAddress)
	}
	if item.Amount != 12345 {
		t.Fatalf("unexpected claim amount: %d", item.Amount)
	}
}

func TestBuildStakeClaimedReward_RejectsNonClaimPayload(t *testing.T) {
	m := NewManager(conf.DefaultConfig())
	tx := &protocolparser.TxSnapshot{
		TxID:  "tx-2",
		TxIdx: 8,
		Outputs: []protocolparser.OutputSnapshot{
			{OutputIdx: 0, AddressKey: "user-receiver-address", Satoshi: 100},
		},
	}
	payload := &protocolparser.OpReturnPayload{
		Tag:    protocolparser.TagStake,
		Fields: map[string]string{},
	}

	item, err := m.buildStakeClaimedReward(100, tx, payload)
	if err != nil {
		t.Fatalf("buildStakeClaimedReward returned error: %v", err)
	}
	if item != nil {
		t.Fatalf("expected nil item for non-claim payload")
	}
}

func TestParseFIP101PayloadFromCSV_ClaimRejected(t *testing.T) {
	payload, _, err := protocolparser.ParseFIP101PayloadFromCSV([]byte("fip101,1,claim"), nil, "reward-claim-sender-address", "")
	if err == nil {
		t.Fatalf("expected error for inscription claim, got payload: %#v", payload)
	}
}

func TestParseFIP101PayloadFromCSV_ClaimWithIndexerIDRejected(t *testing.T) {
	payload, _, err := protocolparser.ParseFIP101PayloadFromCSV([]byte("fip101,1,claim,1:2"), nil, "reward-claim-sender-address", "")
	if err == nil {
		t.Fatalf("expected error for claim with indexer_id, got payload: %#v", payload)
	}
}

func TestTruncateRunes_NameTruncated(t *testing.T) {
	name := strings.Repeat("n", protocolparser.MaxIndexerNameLen+10)

	got := protocolparser.TruncateRunes(name, protocolparser.MaxIndexerNameLen)
	if len([]rune(got)) != protocolparser.MaxIndexerNameLen {
		t.Fatalf("expected truncated name length=%d, got=%d", protocolparser.MaxIndexerNameLen, len([]rune(got)))
	}
}

func TestParseFIP101PayloadFromCSV_RegisterQuotedCommaNameRejected(t *testing.T) {
	_, _, err := protocolparser.ParseFIP101PayloadFromCSV(
		[]byte(`fip101,1,register,100,bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kg3g4ty,"hello,world"`),
		nil,
		"reward-claim-sender-address",
		"",
	)
	if err == nil {
		t.Fatalf("expected error for quoted comma name in strict comma format")
	}
}
