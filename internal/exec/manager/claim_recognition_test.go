package indexer

import (
	"strings"
	"testing"

	"stake_indexer/conf"
	pgdb "stake_indexer/internal/component/pg"
	protocolparser "stake_indexer/internal/parser/protocol"
)

const claimSenderAddress = "reward-claim-sender-address"

func newClaimTestManager() *Manager {
	cfg := conf.DefaultConfig()
	cfg.RewardClaimSenderAddressKeys = []string{claimSenderAddress}
	return NewManager(cfg)
}

func claimTestInputs() []protocolparser.InputSnapshot {
	return []protocolparser.InputSnapshot{{InputIdx: 0, AddressKey: claimSenderAddress, Satoshi: 1000}}
}

func TestBuildStakeClaimedReward_UsesFirstOutput(t *testing.T) {
	m := newClaimTestManager()
	tx := &protocolparser.TxSnapshot{
		TxID:   "tx-1",
		TxIdx:  7,
		Inputs: claimTestInputs(),
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

func TestBuildStakeClaimedReward_UsesStakeRewardTypeByDefault(t *testing.T) {
	m := newClaimTestManager()
	tx := &protocolparser.TxSnapshot{
		TxID:   "tx-stake-claim",
		TxIdx:  7,
		Inputs: claimTestInputs(),
		Outputs: []protocolparser.OutputSnapshot{
			{OutputIdx: 0, AddressKey: "shared-receiver-address", Satoshi: 12345},
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
	if item.RewardType != pgdb.StakeRewardTypeStake {
		t.Fatalf("stake claim reward type expected %s, got %s", pgdb.StakeRewardTypeStake, item.RewardType)
	}
	if item.IndexerID != "" {
		t.Fatalf("stake claim indexer id expected empty, got %s", item.IndexerID)
	}
}

func TestBuildStakeClaimedReward_UsesIndexerRewardTypeWithIndexerID(t *testing.T) {
	m := newClaimTestManager()
	tx := &protocolparser.TxSnapshot{
		TxID:   "tx-indexer-claim",
		TxIdx:  7,
		Inputs: claimTestInputs(),
		Outputs: []protocolparser.OutputSnapshot{
			{OutputIdx: 0, AddressKey: "shared-receiver-address", Satoshi: 12345},
		},
	}
	payload := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagPledgedReward,
		Fields: map[string]string{
			protocolparser.OpFieldIndexerID: "12:34",
		},
	}

	item, err := m.buildStakeClaimedReward(100, tx, payload)
	if err != nil {
		t.Fatalf("buildStakeClaimedReward returned error: %v", err)
	}
	if item == nil {
		t.Fatalf("buildStakeClaimedReward returned nil item")
	}
	if item.UserAddress != "shared-receiver-address" {
		t.Fatalf("unexpected recipient address: %s", item.UserAddress)
	}
	if item.RewardType != pgdb.StakeRewardTypeIndexer {
		t.Fatalf("indexer claim reward type expected %s, got %s", pgdb.StakeRewardTypeIndexer, item.RewardType)
	}
	if item.IndexerID != "12:34" {
		t.Fatalf("indexer claim indexer id expected 12:34, got %s", item.IndexerID)
	}
}

func TestBuildStakeClaimedReward_UsesRegisteredIndexerUserAddress(t *testing.T) {
	m := newClaimTestManager()
	m.WaitForUpsert.StakeIndexerRegisterList = append(m.WaitForUpsert.StakeIndexerRegisterList, pgdb.StakeIndexerRegister{
		IndexerID:   "12:34",
		UserAddress: "indexer-owner-address",
	})
	tx := &protocolparser.TxSnapshot{
		TxID:   "tx-indexer-claim-owner",
		TxIdx:  7,
		Inputs: claimTestInputs(),
		Outputs: []protocolparser.OutputSnapshot{
			{OutputIdx: 0, AddressKey: "reward-receiver-address", Satoshi: 12345},
		},
	}
	payload := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagPledgedReward,
		Fields: map[string]string{
			protocolparser.OpFieldIndexerID: "12:34",
		},
	}

	item, err := m.buildStakeClaimedReward(100, tx, payload)
	if err != nil {
		t.Fatalf("buildStakeClaimedReward returned error: %v", err)
	}
	if item == nil {
		t.Fatalf("buildStakeClaimedReward returned nil item")
	}
	if item.UserAddress != "indexer-owner-address" {
		t.Fatalf("expected registered indexer user address, got %s", item.UserAddress)
	}
	if item.RewardType != pgdb.StakeRewardTypeIndexer {
		t.Fatalf("indexer claim reward type expected %s, got %s", pgdb.StakeRewardTypeIndexer, item.RewardType)
	}
	if item.IndexerID != "12:34" {
		t.Fatalf("indexer claim indexer id expected 12:34, got %s", item.IndexerID)
	}
}

func TestBuildStakeClaimedReward_FixesLegacyIndexerClaimByDefault(t *testing.T) {
	m := newClaimTestManager()
	tx := &protocolparser.TxSnapshot{
		TxID:   "a56daf10ee0ba7f121292806ff39907a281f8c6c2da9f970c0530180ad88d451",
		TxIdx:  7,
		Inputs: claimTestInputs(),
		Outputs: []protocolparser.OutputSnapshot{
			{OutputIdx: 0, AddressKey: "legacy-indexer-receiver", Satoshi: 12345},
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
	if item.RewardType != pgdb.StakeRewardTypeIndexer {
		t.Fatalf("legacy claim reward type expected %s, got %s", pgdb.StakeRewardTypeIndexer, item.RewardType)
	}
	if item.IndexerID != "1842346:1" {
		t.Fatalf("legacy claim indexer id expected 1842346:1, got %s", item.IndexerID)
	}
}

func TestBuildStakeClaimedReward_SkipsLegacyIndexerClaimFixWhenDisabled(t *testing.T) {
	cfg := conf.DefaultConfig()
	cfg.FixLegacyIndexerClaimRewards = false
	cfg.RewardClaimSenderAddressKeys = []string{claimSenderAddress}
	m := NewManager(cfg)
	tx := &protocolparser.TxSnapshot{
		TxID:   "a56daf10ee0ba7f121292806ff39907a281f8c6c2da9f970c0530180ad88d451",
		TxIdx:  7,
		Inputs: claimTestInputs(),
		Outputs: []protocolparser.OutputSnapshot{
			{OutputIdx: 0, AddressKey: "legacy-indexer-receiver", Satoshi: 12345},
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
	if item.RewardType != pgdb.StakeRewardTypeStake {
		t.Fatalf("legacy claim reward type expected %s when disabled, got %s", pgdb.StakeRewardTypeStake, item.RewardType)
	}
	if item.IndexerID != "" {
		t.Fatalf("legacy claim indexer id expected empty when disabled, got %s", item.IndexerID)
	}
}

func TestBuildStakeClaimedReward_PayloadIndexerIDOverridesLegacyFix(t *testing.T) {
	m := newClaimTestManager()
	tx := &protocolparser.TxSnapshot{
		TxID:   "a56daf10ee0ba7f121292806ff39907a281f8c6c2da9f970c0530180ad88d451",
		TxIdx:  7,
		Inputs: claimTestInputs(),
		Outputs: []protocolparser.OutputSnapshot{
			{OutputIdx: 0, AddressKey: "legacy-indexer-receiver", Satoshi: 12345},
		},
	}
	payload := &protocolparser.OpReturnPayload{
		Tag: protocolparser.TagPledgedReward,
		Fields: map[string]string{
			protocolparser.OpFieldIndexerID: "999:1",
		},
	}

	item, err := m.buildStakeClaimedReward(100, tx, payload)
	if err != nil {
		t.Fatalf("buildStakeClaimedReward returned error: %v", err)
	}
	if item == nil {
		t.Fatalf("buildStakeClaimedReward returned nil item")
	}
	if item.RewardType != pgdb.StakeRewardTypeIndexer {
		t.Fatalf("claim reward type expected %s, got %s", pgdb.StakeRewardTypeIndexer, item.RewardType)
	}
	if item.IndexerID != "999:1" {
		t.Fatalf("payload indexer id expected 999:1, got %s", item.IndexerID)
	}
}

func TestBuildStakeClaimedReward_RejectsNonClaimPayload(t *testing.T) {
	m := newClaimTestManager()
	tx := &protocolparser.TxSnapshot{
		TxID:   "tx-2",
		TxIdx:  8,
		Inputs: claimTestInputs(),
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

func TestShouldTrackStakeAddress_IncludesRewardClaimSenderAddress(t *testing.T) {
	m := newClaimTestManager()
	if !m.shouldTrackStakeAddress(claimSenderAddress) {
		t.Fatalf("expected reward claim sender address to be tracked")
	}
	if m.shouldTrackStakeAddress("untracked-address") {
		t.Fatalf("expected unrelated address to be untracked")
	}
}

func TestBuildStakeClaimedReward_RejectsUnconfiguredSenderInput(t *testing.T) {
	m := newClaimTestManager()
	tx := &protocolparser.TxSnapshot{
		TxID:  "tx-unconfigured-sender",
		TxIdx: 7,
		Inputs: []protocolparser.InputSnapshot{
			{InputIdx: 0, AddressKey: "other-input-address", Satoshi: 1000},
		},
		Outputs: []protocolparser.OutputSnapshot{
			{OutputIdx: 0, AddressKey: "user-receiver-address", Satoshi: 100},
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
	if item != nil {
		t.Fatalf("expected nil item for claim with unconfigured sender input")
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
