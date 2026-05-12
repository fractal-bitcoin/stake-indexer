package pgdb

import "testing"

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

	validProofs, updates, err := resolveStakeProofValidity(proofs, "aa", "bb", 0)
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

	validProofs, updates, err := resolveStakeProofValidity(proofs, "aa", "bb", 0)
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
