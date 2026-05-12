package protocolparser

import "testing"

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
