package indexer

import (
	"strings"
	"testing"

	"stake_indexer/conf"
	protocolparser "stake_indexer/internal/parser/protocol"
)

func TestBuildStakeClaimedReward_UsesSystemActorAndFirstOutput(t *testing.T) {
	original := conf.StakeRewardCfg
	t.Cleanup(func() { conf.StakeRewardCfg = original })

	cfg := conf.DefaultConfig()
	cfg.ColdWalletAddressKey = []string{"system-wallet-address"}
	conf.StakeRewardCfg = cfg

	m := NewManager(cfg)
	tx := &protocolparser.TxSnapshot{
		TxID:  "tx-1",
		TxIdx: 7,
		Outputs: []protocolparser.OutputSnapshot{
			{OutputIdx: 0, AddressKey: "user-receiver-address", Satoshi: 12345},
		},
	}
	payload := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagPledgedReward,
		Fields: map[string]string{
			protocolparser.OpFieldActorAddr: "system-wallet-address",
		},
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

func TestBuildStakeClaimedReward_RejectsNonSystemActor(t *testing.T) {
	original := conf.StakeRewardCfg
	t.Cleanup(func() { conf.StakeRewardCfg = original })

	cfg := conf.DefaultConfig()
	cfg.ColdWalletAddressKey = []string{"system-wallet-address"}
	conf.StakeRewardCfg = cfg

	m := NewManager(cfg)
	tx := &protocolparser.TxSnapshot{
		TxID:  "tx-2",
		TxIdx: 8,
		Outputs: []protocolparser.OutputSnapshot{
			{OutputIdx: 0, AddressKey: "user-receiver-address", Satoshi: 100},
		},
	}
	payload := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagPledgedReward,
		Fields: map[string]string{
			protocolparser.OpFieldActorAddr: "unknown-wallet-address",
		},
	}

	item, err := m.buildStakeClaimedReward(100, tx, payload)
	if err != nil {
		t.Fatalf("buildStakeClaimedReward returned error: %v", err)
	}
	if item != nil {
		t.Fatalf("expected nil item for non-system actor")
	}
}

func TestParseFIP101PayloadFromCSV_ClaimWithoutIndexerID(t *testing.T) {
	payload, _, err := protocolparser.ParseFIP101PayloadFromCSV([]byte("fip101,1,claim"), nil, "system-wallet-address", "")
	if err != nil {
		t.Fatalf("parseFIP101PayloadFromCSV returned error: %v", err)
	}
	if payload == nil {
		t.Fatalf("parseFIP101PayloadFromCSV returned nil payload")
	}
	if payload.Tag != protocolparser.TagPledgedReward {
		t.Fatalf("unexpected payload tag: %s", payload.Tag)
	}
	if got := payload.Get(protocolparser.OpFieldIndexerID); got != "" {
		t.Fatalf("unexpected indexer id: %s", got)
	}
}

func TestParseFIP101PayloadFromCSV_ClaimWithIndexerIDRejected(t *testing.T) {
	payload, _, err := protocolparser.ParseFIP101PayloadFromCSV([]byte("fip101,1,claim,1:2"), nil, "system-wallet-address", "")
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
		"system-wallet-address",
		"",
	)
	if err == nil {
		t.Fatalf("expected error for quoted comma name in strict comma format")
	}
}
