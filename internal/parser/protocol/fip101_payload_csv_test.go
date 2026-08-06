package protocolparser

import (
	"strings"
	"testing"

	"stake_indexer/model"
)

func TestParseFIP101PayloadFromCSV_NewProtocolOpNames(t *testing.T) {
	actorPubKey := make([]byte, 32)
	actorAddr := "bc1qactoraddressxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	actorType := "2"

	tests := []struct {
		name    string
		csv     string
		wantTag string
	}{
		{
			name:    "register_indexer",
			csv:     "fip101,1,register_indexer,1000,bc1qnlvzhv535uzq2t0jtfkfjfhs4jtaycrln2tetn,tetn-indexer",
			wantTag: TagRegister,
		},
		{
			name:    "commission_rate",
			csv:     "fip101,1,commission_rate,100:2,900",
			wantTag: TagAllocatRatio,
		},
		{
			name:    "submit_proof",
			csv:     "fip101,1,submit_proof,100:2,123,0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			wantTag: TagProveStake,
		},
		{
			name:    "stake",
			csv:     "fip101,1,stake,100:2",
			wantTag: TagStake,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload, _, err := ParseFIP101PayloadFromCSV([]byte(tc.csv), actorPubKey, actorAddr, actorType)
			if err != nil {
				t.Fatalf("parse payload failed: %v", err)
			}
			if payload == nil {
				t.Fatalf("payload is nil")
			}
			if payload.Tag != tc.wantTag {
				t.Fatalf("unexpected tag: got=%s want=%s", payload.Tag, tc.wantTag)
			}
		})
	}
}

func TestIsValidIndexerName(t *testing.T) {
	valid64 := "A" + strings.Repeat("z", 63)
	tests := []struct {
		name string
		want bool
	}{
		{name: "Indexer01", want: true},
		{name: "node.alpha_01-main", want: true},
		{name: "my indexer", want: true},
		{name: valid64, want: true},
		{name: "", want: false},
		{name: "_indexer", want: false},
		{name: "-indexer", want: false},
		{name: ".indexer", want: false},
		{name: " indexer", want: false},
		{name: "indexer@1", want: false},
		{name: "中文节点", want: false},
		{name: "A" + strings.Repeat("z", 64), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidIndexerName(tc.name); got != tc.want {
				t.Fatalf("IsValidIndexerName(%q)=%v want=%v", tc.name, got, tc.want)
			}
		})
	}
}

func TestParseFIP101PayloadFromCSV_RegisterRejectsInvalidIndexerName(t *testing.T) {
	actorPubKey := make([]byte, 32)
	actorAddr := "bc1qactoraddressxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	actorType := "2"
	csv := "fip101,1,register_indexer,1000,bc1qnlvzhv535uzq2t0jtfkfjfhs4jtaycrln2tetn,indexer@1"

	payload, _, err := ParseFIP101PayloadFromCSV([]byte(csv), actorPubKey, actorAddr, actorType)
	if err == nil {
		t.Fatalf("expected invalid indexer name to be rejected, got payload: %#v", payload)
	}
}

func TestParseProtocolTxFromModelTx_ClaimRewardFromOpReturn(t *testing.T) {
	tx := &model.Tx{
		TxIdHex: "tx-claim",
		TxOuts: []*model.TxOut{
			{Satoshi: 1000, PkScript: p2wpkhScript(0x01)},
			{PkScript: opReturnScript("FIP-101:claim_reward")},
		},
	}

	parsed, err := ParseProtocolTxFromModelTx(tx, 3, 100)
	if err != nil {
		t.Fatalf("parse protocol tx failed: %v", err)
	}
	if parsed == nil || parsed.Payload == nil {
		t.Fatalf("parsed claim payload is nil")
	}
	if parsed.Payload.Tag != TagPledgedReward {
		t.Fatalf("unexpected tag: got=%s want=%s", parsed.Payload.Tag, TagPledgedReward)
	}
	if parsed.Event == nil {
		t.Fatalf("event is nil")
	}
	if parsed.Event.Op != "claim_reward" {
		t.Fatalf("unexpected event op: %s", parsed.Event.Op)
	}
	if parsed.Event.UserAddress == "" {
		t.Fatalf("expected claim receiver address")
	}
	if parsed.Event.Amount != 1000 {
		t.Fatalf("unexpected claim amount: %d", parsed.Event.Amount)
	}
}

func TestParseProtocolTxFromModelTx_IndexerClaimRewardFromOpReturn(t *testing.T) {
	tx := &model.Tx{
		TxIdHex: "tx-indexer-claim",
		TxOuts: []*model.TxOut{
			{Satoshi: 2000, PkScript: p2wpkhScript(0x01)},
			{PkScript: opReturnScript("FIP-101:claim_reward:12:34")},
		},
	}

	parsed, err := ParseProtocolTxFromModelTx(tx, 3, 100)
	if err != nil {
		t.Fatalf("parse protocol tx failed: %v", err)
	}
	if parsed == nil || parsed.Payload == nil {
		t.Fatalf("parsed claim payload is nil")
	}
	if got := parsed.Payload.Get(OpFieldIndexerID); got != "12:34" {
		t.Fatalf("unexpected claim indexer id: %s", got)
	}
	if parsed.Event == nil || parsed.Event.IndexerID != "12:34" {
		t.Fatalf("expected event indexer_id 12:34, got %#v", parsed.Event)
	}
	if parsed.Event.Amount != 2000 {
		t.Fatalf("unexpected claim amount: %d", parsed.Event.Amount)
	}
}

func TestParseProtocolTxFromModelTx_EarlySupporterClaimRewardFromOpReturn(t *testing.T) {
	tx := &model.Tx{
		TxIdHex: "tx-early-supporter-claim",
		TxOuts: []*model.TxOut{
			{Satoshi: 3000, PkScript: p2wpkhScript(0x01)},
			{PkScript: opReturnScript("FIP-101:claim_reward:early_supporter_reward")},
		},
	}

	parsed, err := ParseProtocolTxFromModelTx(tx, 3, 100)
	if err != nil {
		t.Fatalf("parse protocol tx failed: %v", err)
	}
	if parsed == nil || parsed.Payload == nil {
		t.Fatalf("parsed claim payload is nil")
	}
	if got := parsed.Payload.Get(OpFieldRewardClaimType); got != OpValueEarlySupporterReward {
		t.Fatalf("unexpected reward claim type: %s", got)
	}
	if got := parsed.Payload.Get(OpFieldIndexerID); got != "" {
		t.Fatalf("early supporter stake claim indexer id expected empty, got %s", got)
	}
}

func TestParseProtocolTxFromModelTx_EarlySupporterIndexerClaimRewardFromOpReturn(t *testing.T) {
	tx := &model.Tx{
		TxIdHex: "tx-early-supporter-indexer-claim",
		TxOuts: []*model.TxOut{
			{Satoshi: 4000, PkScript: p2wpkhScript(0x01)},
			{PkScript: opReturnScript("FIP-101:claim_reward:early_supporter_reward:12:34")},
		},
	}

	parsed, err := ParseProtocolTxFromModelTx(tx, 3, 100)
	if err != nil {
		t.Fatalf("parse protocol tx failed: %v", err)
	}
	if parsed == nil || parsed.Payload == nil {
		t.Fatalf("parsed claim payload is nil")
	}
	if got := parsed.Payload.Get(OpFieldRewardClaimType); got != OpValueEarlySupporterReward {
		t.Fatalf("unexpected reward claim type: %s", got)
	}
	if got := parsed.Payload.Get(OpFieldIndexerID); got != "12:34" {
		t.Fatalf("unexpected claim indexer id: %s", got)
	}
	if parsed.Event == nil || parsed.Event.IndexerID != "12:34" {
		t.Fatalf("expected event indexer_id 12:34, got %#v", parsed.Event)
	}
}

func TestParseFIP101PayloadFromCSV_ClaimRewardRejected(t *testing.T) {
	payload, _, err := ParseFIP101PayloadFromCSV([]byte("fip101,1,claim_reward"), nil, "", "")
	if err == nil {
		t.Fatalf("expected csv claim_reward to be rejected, got payload: %#v", payload)
	}
}

func opReturnScript(text string) []byte {
	data := []byte(text)
	return append([]byte{0x6a, byte(len(data))}, data...)
}

func p2wpkhScript(fill byte) []byte {
	script := []byte{0x00, 0x14}
	for i := 0; i < 20; i++ {
		script = append(script, fill)
	}
	return script
}
