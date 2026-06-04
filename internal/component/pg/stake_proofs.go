package pgdb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
)

const (
	StakeProofVerifyValid            int16 = 1
	StakeProofVerifyInvalidHash      int16 = 2
	StakeProofVerifyInvalidDuplicate int16 = 3
	StakeProofVerifyValidDelayed     int16 = 4
	StakeProofVerifyExpired          int16 = 5
)

type StakeProof struct {
	IndexerID        string
	ProveBlockHeight uint32
	ProveDataHash    string
	TxID             string
	Height           uint32
	TxIdx            uint32
	VerifyStatus     int16
}

func UpsertStakeProof(ctx context.Context, proof StakeProof) error {
	if StakeDB == nil {
		return nil
	}

	return upsertStakeProof(ctx, StakeDB, proof)
}

func UpsertStakeProofTx(ctx context.Context, tx *sql.Tx, proof StakeProof) error {
	if tx == nil {
		return UpsertStakeProof(ctx, proof)
	}

	return upsertStakeProof(ctx, tx, proof)
}

func upsertStakeProof(ctx context.Context, execer stakeExecer, proof StakeProof) error {
	if execer == nil {
		return nil
	}

	const sqlText = `
INSERT INTO stake_proofs (
    indexer_id, prove_block_height, prove_data_hash, txid, height, tx_idx
) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (txid) DO UPDATE
SET
    indexer_id = EXCLUDED.indexer_id,
    prove_block_height = EXCLUDED.prove_block_height,
    prove_data_hash = EXCLUDED.prove_data_hash,
    height = EXCLUDED.height,
    tx_idx = EXCLUDED.tx_idx,
    verify_status = 0
`

	if _, err := execer.ExecContext(
		ctx, sqlText,
		proof.IndexerID,
		proof.ProveBlockHeight,
		proof.ProveDataHash,
		proof.TxID,
		proof.Height,
		proof.TxIdx,
	); err != nil {
		return fmt.Errorf("upsert stake proof failed: %w", err)
	}

	return nil
}

func ListStakeProofByProveHeight(ctx context.Context, proveBlockHeight, proofWindow uint32) ([]StakeProof, error) {
	if StakeDB == nil {
		return nil, nil
	}

	const sqlText = `
SELECT
    indexer_id, prove_block_height, prove_data_hash, txid, height, tx_idx, verify_status
FROM stake_proofs
WHERE prove_block_height = $1 and height > $2 and height <= $3
ORDER BY height DESC, tx_idx DESC, txid DESC
`

	rows, err := StakeDB.QueryContext(ctx, sqlText, proveBlockHeight, proveBlockHeight, proveBlockHeight+proofWindow)
	if err != nil {
		return nil, fmt.Errorf("query stake proofs failed: %w", err)
	}
	defer rows.Close()

	result := make([]StakeProof, 0, 16)
	for rows.Next() {
		var item StakeProof
		if err := rows.Scan(
			&item.IndexerID,
			&item.ProveBlockHeight,
			&item.ProveDataHash,
			&item.TxID,
			&item.Height,
			&item.TxIdx,
			&item.VerifyStatus,
		); err != nil {
			return nil, fmt.Errorf("scan stake proof failed: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake proofs failed: %w", err)
	}

	return result, nil
}

func ListStakeProofsByIndexerID(ctx context.Context, indexerID string, limit, offset int) ([]StakeProof, error) {
	if StakeDB == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	const sqlText = `
SELECT
    indexer_id, prove_block_height, prove_data_hash, txid, height, tx_idx, verify_status
FROM stake_proofs
WHERE indexer_id = $1
ORDER BY height DESC, tx_idx DESC
LIMIT $2 OFFSET $3
`
	rows, err := StakeDB.QueryContext(ctx, sqlText, indexerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query stake proofs by indexer id failed: %w", err)
	}
	defer rows.Close()

	result := make([]StakeProof, 0, limit)
	for rows.Next() {
		var item StakeProof
		if err := rows.Scan(
			&item.IndexerID,
			&item.ProveBlockHeight,
			&item.ProveDataHash,
			&item.TxID,
			&item.Height,
			&item.TxIdx,
			&item.VerifyStatus,
		); err != nil {
			return nil, fmt.Errorf("scan stake proof failed: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stake proofs failed: %w", err)
	}

	return result, nil
}

func CountStakeProofsByIndexerID(ctx context.Context, indexerID string) (int, error) {
	if StakeDB == nil {
		return 0, nil
	}
	const sqlText = `SELECT COUNT(*) FROM stake_proofs WHERE indexer_id = $1`
	var count int
	err := StakeDB.QueryRowContext(ctx, sqlText, indexerID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count stake proofs by indexer id failed: %w", err)
	}
	return count, nil
}

type StakeProofValidityRules struct {
	DelaySubmitTriggerBlocks     uint32
	DelaySubmitStage2StepBlocks  uint32
	DelaySubmitStage2StepPercent uint64
	Stage2StartHeight            uint32
}

func ResolveStakeProofValidityByProveHeightWithRules(ctx context.Context, proveBlockHeight, proofWindow uint32, blockHash, stateHash string, rules StakeProofValidityRules) ([]StakeProof, error) {
	return resolveStakeProofValidityByProveHeight(ctx, proveBlockHeight, proofWindow, blockHash, stateHash, rules, false)
}

func ResolveStakeProofValidityByProveHeightReadOnlyWithRules(ctx context.Context, proveBlockHeight, proofWindow uint32, blockHash, stateHash string, rules StakeProofValidityRules) ([]StakeProof, error) {
	return resolveStakeProofValidityByProveHeight(ctx, proveBlockHeight, proofWindow, blockHash, stateHash, rules, true)
}

func resolveStakeProofValidityByProveHeight(ctx context.Context, proveBlockHeight, proofWindow uint32, blockHash, stateHash string, rules StakeProofValidityRules, readOnly bool) ([]StakeProof, error) {
	if StakeDB == nil {
		return nil, nil
	}

	proofs, err := ListStakeProofByProveHeight(ctx, proveBlockHeight, proofWindow)
	if err != nil {
		return nil, err
	}
	if len(proofs) == 0 {
		return nil, nil
	}

	validProofs, updates, err := resolveStakeProofValidity(proofs, blockHash, stateHash, rules)
	if err != nil {
		return nil, err
	}
	if readOnly {
		return validProofs, nil
	}

	byTxID := make(map[string]*StakeProof, len(proofs))
	for i := range proofs {
		byTxID[proofs[i].TxID] = &proofs[i]
	}

	tx, err := StakeDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin resolve proof validity tx failed: %w", err)
	}
	defer tx.Rollback()

	const updateSQL = `
UPDATE stake_proofs
SET
    verify_status = $1
WHERE txid = $2
`
	for txID, update := range updates {
		current := byTxID[txID]
		if current != nil && current.VerifyStatus == update.status {
			continue
		}

		if _, err := tx.ExecContext(ctx, updateSQL, update.status, txID); err != nil {
			return nil, fmt.Errorf("update stake proof validity failed: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit resolve proof validity tx failed: %w", err)
	}
	return validProofs, nil
}

type hashGroup struct {
	latest     *StakeProof
	duplicates []*StakeProof
}

type verifyUpdate struct{ status int16 }

func resolveStakeProofValidity(proofs []StakeProof, blockHash, stateHash string, rules StakeProofValidityRules) ([]StakeProof, map[string]verifyUpdate, error) {
	blockHash = strings.ToLower(strings.TrimSpace(blockHash))
	stateHash = strings.ToLower(strings.TrimSpace(stateHash))
	if blockHash == "" {
		return nil, nil, fmt.Errorf("block hash is empty")
	}
	if stateHash == "" {
		return nil, nil, fmt.Errorf("state hash is empty")
	}

	groupsByIndexerHash := make(map[string]map[string]*hashGroup, len(proofs))
	updates := make(map[string]verifyUpdate, len(proofs))
	for i := range proofs {
		p := &proofs[i]

		byHash := groupsByIndexerHash[p.IndexerID]
		if byHash == nil {
			byHash = make(map[string]*hashGroup)
			groupsByIndexerHash[p.IndexerID] = byHash
		}

		group := byHash[p.ProveDataHash]
		if group == nil {
			group = &hashGroup{}
			byHash[p.ProveDataHash] = group
		}

		if group.latest == nil {
			group.latest = p
			continue
		}
		if isProofEarlier(*p, *group.latest) {
			group.duplicates = append(group.duplicates, group.latest)
			group.latest = p
			continue
		}
		group.duplicates = append(group.duplicates, p)
	}

	validProofs := make([]StakeProof, 0, len(proofs))
	for indexerID, byHash := range groupsByIndexerHash {
		expectedHash := computeStakeProofHash(indexerID, blockHash, stateHash)
		for hash, group := range byHash {
			if group == nil || group.latest == nil {
				continue
			}

			latestStatus := resolveValidStakeProofStatus(*group.latest, rules)

			if strings.EqualFold(hash, expectedHash) {
				updates[group.latest.TxID] = verifyUpdate{status: latestStatus}
				if latestStatus != StakeProofVerifyExpired {
					item := *group.latest
					item.VerifyStatus = latestStatus
					validProofs = append(validProofs, item)
				}
				for _, dup := range group.duplicates {
					if dup == nil {
						continue
					}
					updates[dup.TxID] = verifyUpdate{status: StakeProofVerifyInvalidDuplicate}
				}
				continue
			}

			updates[group.latest.TxID] = verifyUpdate{status: StakeProofVerifyInvalidHash}
			for _, dup := range group.duplicates {
				if dup == nil {
					continue
				}
				updates[dup.TxID] = verifyUpdate{status: StakeProofVerifyInvalidHash}
			}
		}
	}
	return validProofs, updates, nil
}

func computeStakeProofHash(indexerID, blockHash, stateHash string) string {
	payload := strings.ToLower(strings.TrimSpace(indexerID)) + ":" +
		strings.ToLower(strings.TrimSpace(blockHash)) + ":" +
		strings.ToLower(strings.TrimSpace(stateHash))
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", sum[:])
}

func resolveValidStakeProofStatus(proof StakeProof, rules StakeProofValidityRules) int16 {
	if rules.Stage2StartHeight > 0 && proof.ProveBlockHeight >= rules.Stage2StartHeight {
		return resolveStage2StakeProofStatus(proof, rules)
	}
	if shouldApplyDelaySubmitPenalty(proof, rules.DelaySubmitTriggerBlocks) {
		return StakeProofVerifyValidDelayed
	}
	return StakeProofVerifyValid
}

func resolveStage2StakeProofStatus(proof StakeProof, rules StakeProofValidityRules) int16 {
	stepBlocks := rules.DelaySubmitStage2StepBlocks
	stepPercent := rules.DelaySubmitStage2StepPercent
	if proof.Height <= proof.ProveBlockHeight || stepBlocks == 0 || stepPercent == 0 {
		return StakeProofVerifyValid
	}

	delayedBlocks := proof.Height - proof.ProveBlockHeight
	steps := uint64(delayedBlocks / stepBlocks)
	penaltyPercent := steps * stepPercent
	if penaltyPercent >= 100 {
		return StakeProofVerifyExpired
	}
	if delayedBlocks > stepBlocks {
		return StakeProofVerifyValidDelayed
	}
	return StakeProofVerifyValid
}

func shouldApplyDelaySubmitPenalty(proof StakeProof, delaySubmitTriggerBlocks uint32) bool {
	if delaySubmitTriggerBlocks == 0 {
		return false
	}
	if proof.Height <= proof.ProveBlockHeight {
		return false
	}
	delta := proof.Height - proof.ProveBlockHeight
	return delta > delaySubmitTriggerBlocks
}

func isProofLater(a, b StakeProof) bool {
	if a.Height != b.Height {
		return a.Height > b.Height
	}
	if a.TxIdx != b.TxIdx {
		return a.TxIdx > b.TxIdx
	}
	return a.TxID > b.TxID
}

func isProofEarlier(a, b StakeProof) bool {
	if a.Height != b.Height {
		return a.Height < b.Height
	}
	if a.TxIdx != b.TxIdx {
		return a.TxIdx < b.TxIdx
	}
	return a.TxID < b.TxID
}
