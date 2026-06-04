package pgdb

import (
	"testing"
)

func TestResolveStakeProofValidityUsesComputedHash(t *testing.T) {
	expectedHash := computeStakeProofHash("42664:1", "aa", "bb")
	proofs := []StakeProof{
		{
			IndexerID:        "42664:1",
			ProveBlockHeight: 100,
			ProveDataHash:    expectedHash,
			TxID:             "tx-valid",
			Height:           102,
			TxIdx:            1,
		},
		{
			IndexerID:        "42664:1",
			ProveBlockHeight: 100,
			ProveDataHash:    "deadbeef",
			TxID:             "tx-invalid",
			Height:           101,
			TxIdx:            0,
		},
	}

	validProofs, updates, err := resolveStakeProofValidity(proofs, "aa", "bb", StakeProofValidityRules{})
	if err != nil {
		t.Fatalf("resolveStakeProofValidity() error = %v", err)
	}
	if len(validProofs) != 1 {
		t.Fatalf("valid proofs len = %d, want 1", len(validProofs))
	}
	if validProofs[0].TxID != "tx-valid" {
		t.Fatalf("valid txid = %q, want tx-valid", validProofs[0].TxID)
	}
	if updates["tx-valid"].status != StakeProofVerifyValid {
		t.Fatalf("tx-valid status = %d, want %d", updates["tx-valid"].status, StakeProofVerifyValid)
	}
	if updates["tx-invalid"].status != StakeProofVerifyInvalidHash {
		t.Fatalf("tx-invalid status = %d, want %d", updates["tx-invalid"].status, StakeProofVerifyInvalidHash)
	}
}

func TestResolveStakeProofValidityMarksDuplicates(t *testing.T) {
	expectedHash := computeStakeProofHash("42664:1", "aa", "bb")
	proofs := []StakeProof{
		{
			IndexerID:        "42664:1",
			ProveBlockHeight: 100,
			ProveDataHash:    expectedHash,
			TxID:             "tx-earliest",
			Height:           101,
			TxIdx:            0,
		},
		{
			IndexerID:        "42664:1",
			ProveBlockHeight: 100,
			ProveDataHash:    expectedHash,
			TxID:             "tx-duplicate",
			Height:           102,
			TxIdx:            0,
		},
	}

	validProofs, updates, err := resolveStakeProofValidity(proofs, "aa", "bb", StakeProofValidityRules{})
	if err != nil {
		t.Fatalf("resolveStakeProofValidity() error = %v", err)
	}
	if len(validProofs) != 1 || validProofs[0].TxID != "tx-earliest" {
		t.Fatalf("valid proofs = %#v, want only tx-earliest", validProofs)
	}
	if updates["tx-earliest"].status != StakeProofVerifyValid {
		t.Fatalf("tx-earliest status = %d, want %d", updates["tx-earliest"].status, StakeProofVerifyValid)
	}
	if updates["tx-duplicate"].status != StakeProofVerifyInvalidDuplicate {
		t.Fatalf("tx-duplicate status = %d, want %d", updates["tx-duplicate"].status, StakeProofVerifyInvalidDuplicate)
	}
}

func TestResolveStakeProofValidityUsesStage2DelayRules(t *testing.T) {
	rules := StakeProofValidityRules{
		Stage2StartHeight:            1000,
		DelaySubmitStage2StepBlocks:  100,
		DelaySubmitStage2StepPercent: 10,
	}

	tests := []struct {
		name   string
		height uint32
		want   int16
	}{
		{
			name:   "within step remains valid",
			height: 1100,
			want:   StakeProofVerifyValid,
		},
		{
			name:   "over one step is delayed",
			height: 1101,
			want:   StakeProofVerifyValidDelayed,
		},
		{
			name:   "one hundred percent penalty is delayed",
			height: 2000,
			want:   StakeProofVerifyValidDelayed,
		},
		{
			name:   "over one hundred percent penalty expires",
			height: 2100,
			want:   StakeProofVerifyExpired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expectedHash := computeStakeProofHash("42664:1", "aa", "bb")
			proofs := []StakeProof{
				{
					IndexerID:        "42664:1",
					ProveBlockHeight: 1000,
					ProveDataHash:    expectedHash,
					TxID:             "tx-valid",
					Height:           tc.height,
					TxIdx:            0,
				},
			}

			validProofs, updates, err := resolveStakeProofValidity(proofs, "aa", "bb", rules)
			if err != nil {
				t.Fatalf("resolveStakeProofValidity() error = %v", err)
			}
			if updates["tx-valid"].status != tc.want {
				t.Fatalf("tx-valid status = %d, want %d", updates["tx-valid"].status, tc.want)
			}
			if tc.want == StakeProofVerifyExpired {
				if len(validProofs) != 0 {
					t.Fatalf("valid proofs len = %d, want 0", len(validProofs))
				}
				return
			}
			if len(validProofs) != 1 {
				t.Fatalf("valid proofs len = %d, want 1", len(validProofs))
			}
			if validProofs[0].VerifyStatus != tc.want {
				t.Fatalf("valid proof status = %d, want %d", validProofs[0].VerifyStatus, tc.want)
			}
		})
	}
}

func TestResolveStakeProofValidityStage1KeepsTriggerRule(t *testing.T) {
	rules := StakeProofValidityRules{
		DelaySubmitTriggerBlocks:     120,
		Stage2StartHeight:            1000,
		DelaySubmitStage2StepBlocks:  100,
		DelaySubmitStage2StepPercent: 10,
	}

	expectedHash := computeStakeProofHash("42664:1", "aa", "bb")
	proofs := []StakeProof{
		{
			IndexerID:        "42664:1",
			ProveBlockHeight: 999,
			ProveDataHash:    expectedHash,
			TxID:             "tx-valid",
			Height:           1100,
			TxIdx:            0,
		},
	}

	_, updates, err := resolveStakeProofValidity(proofs, "aa", "bb", rules)
	if err != nil {
		t.Fatalf("resolveStakeProofValidity() error = %v", err)
	}
	if updates["tx-valid"].status != StakeProofVerifyValid {
		t.Fatalf("tx-valid status = %d, want %d", updates["tx-valid"].status, StakeProofVerifyValid)
	}
}
