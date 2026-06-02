package indexer

import (
	"fmt"
	"strings"

	"stake_indexer/conf"
	"stake_indexer/constant"
	pgdb "stake_indexer/internal/component/pg"
	protocolparser "stake_indexer/internal/parser/protocol"
)

func (m *Manager) handleRegisterTx(height uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) error {
	indexRatio, ok := protocolparser.ParseRatio(payload.Get(protocolparser.OpFieldIndexRatio))
	if !ok {
		return nil
	}
	rewardAddress := strings.TrimSpace(payload.Get(protocolparser.OpFieldRewardAddr))
	if rewardAddress == "" {
		return nil
	}

	indexerID := protocolparser.BuildIndexerID(height, tx.TxIdx)
	name := payload.Get(protocolparser.OpFieldIndexerName)
	if name == "" {
		name = indexerID
	}

	m.WaitForUpsert.StakeIndexerRegisterList = append(m.WaitForUpsert.StakeIndexerRegisterList, pgdb.StakeIndexerRegister{
		IndexerID:        indexerID,
		Name:             name,
		RewardAddress:    rewardAddress,
		UserAddress:      normalizeUserAddress(payload.Get(protocolparser.OpFieldActorAddr)),
		IndexRatio:       indexRatio,
		LastUpdateHeight: height,
		RegisterTxID:     tx.TxID,
		Height:           height,
		TxIdx:            tx.TxIdx,
	})

	return nil
}

func (m *Manager) buildStakeProof(txHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (*pgdb.StakeProof, error) {
	return m.buildStakeProofAtHeight(txHeight, txHeight, tx, payload)
}

func (m *Manager) buildStakeProofAtHeight(validationHeight uint32, txHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (*pgdb.StakeProof, error) {
	if tx == nil || payload == nil {
		return nil, nil
	}

	indexerID := payload.Get(protocolparser.OpFieldIndexerID)
	var err error
	indexerID, err = m.normalizeIndexerIDAtHeight(indexerID, validationHeight)
	if err != nil {
		return nil, err
	}
	if indexerID == "" {
		return nil, nil
	}
	registered, err := m.isIndexerRegistered(indexerID)
	if err != nil {
		return nil, err
	}
	if !registered {
		return nil, nil
	}

	proveHeight := protocolparser.ParseUint32(payload.Get(protocolparser.OpFieldBlockHeight))
	if proveHeight == 0 {
		return nil, nil
	}

	dataHash := payload.Get(protocolparser.OpFieldBlockHash)
	if dataHash == "" {
		return nil, nil
	}

	return &pgdb.StakeProof{
		IndexerID:        indexerID,
		ProveBlockHeight: proveHeight,
		ProveDataHash:    dataHash,
		TxID:             tx.TxID,
		Height:           txHeight,
		TxIdx:            tx.TxIdx,
	}, nil
}

func (m *Manager) handleProofTx(txHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) error {
	proof, err := m.buildStakeProof(txHeight, tx, payload)
	if err != nil || proof == nil {
		return err
	}

	m.WaitForUpsert.StakeProofList = append(m.WaitForUpsert.StakeProofList, *proof)
	return nil
}

func (m *Manager) buildStakeClaimedReward(txHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (*pgdb.StakeClaimedReward, error) {
	if m == nil || tx == nil || payload == nil {
		return nil, nil
	}

	actorAddress := strings.TrimSpace(payload.Get(protocolparser.OpFieldActorAddr))
	if !isSystemClaimActorAddress(actorAddress) {
		return nil, nil
	}

	out, ok := firstSpendableOutput(tx)
	if !ok || strings.TrimSpace(out.AddressKey) == "" || out.Satoshi == 0 {
		return nil, nil
	}

	return &pgdb.StakeClaimedReward{
		UserAddress: strings.TrimSpace(out.AddressKey),
		Amount:      out.Satoshi,
		TxID:        tx.TxID,
		Height:      txHeight,
		TxIdx:       tx.TxIdx,
	}, nil
}

func (m *Manager) handleClaimedRewardTx(txHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) error {
	item, err := m.buildStakeClaimedReward(txHeight, tx, payload)
	if err != nil || item == nil {
		return err
	}

	m.WaitForUpsert.StakeClaimedRewardList = append(m.WaitForUpsert.StakeClaimedRewardList, *item)
	return nil
}

func isSystemClaimActorAddress(actorAddress string) bool {
	actorAddress = strings.TrimSpace(actorAddress)
	if actorAddress == "" {
		return false
	}

	for _, configured := range conf.StakeRewardCfg.RewardClaimSenderAddressKeys {
		if strings.EqualFold(actorAddress, strings.TrimSpace(configured)) {
			return true
		}
	}
	return false
}

func (m *Manager) handleUpdateRatioTxLatest(height uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) error {
	return m.handleUpdateRatioTx(height, tx, payload, true, false)
}

func (m *Manager) handleUpdateRatioTxSnapshot(height uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) error {
	return m.handleUpdateRatioTx(height, tx, payload, false, true)
}

func (m *Manager) validateUpdateRatioTx(height uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (string, float64, bool, error) {
	if tx == nil || payload == nil {
		return "", 0, false, nil
	}

	indexerID := payload.Get(protocolparser.OpFieldIndexerID)
	if indexerID == "" {
		return "", 0, false, nil
	}
	var err error
	indexerID, err = m.normalizeIndexerIDAtHeight(indexerID, height)
	if err != nil || indexerID == "" {
		return "", 0, false, err
	}

	ratio, ok := protocolparser.ParseRatio(payload.Get(protocolparser.OpFieldIndexRatio))
	if !ok {
		return "", 0, false, nil
	}
	userAddress, err := m.getIndexerUserAddressForAuth(indexerID)
	if err != nil || userAddress == "" {
		return "", 0, false, err
	}
	actorAddress := strings.TrimSpace(payload.Get(protocolparser.OpFieldActorAddr))
	if actorAddress == "" || actorAddress != userAddress {
		return "", 0, false, nil
	}

	return indexerID, ratio, true, nil
}

func (m *Manager) handleUpdateRatioTx(height uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload, updateLatest bool, updateSnapshot bool) error {
	indexerID, ratio, ok, err := m.validateUpdateRatioTx(height, tx, payload)
	if err != nil || !ok {
		return err
	}

	if updateLatest {
		m.stageHSet(m.getIndexerInfoKey(indexerID), map[string]interface{}{
			"index_ratio":        protocolparser.FormatRatio(ratio),
			"last_update_height": height,
		})
	}
	if updateSnapshot {
		snapshotKey := constant.REDIS_STAKE_INDEXER_RATIO_SNAPSHOT_KEY
		if m != nil && m.pendingRewardMode {
			snapshotKey = constant.REDIS_STAKE_PENDING_INDEXER_RATIO_SNAPSHOT_KEY
		}
		m.stageHSet(snapshotKey, map[string]interface{}{
			indexerID: protocolparser.FormatRatio(ratio),
		})
	}

	return nil
}
func (m *Manager) isIndexerRegistered(indexerID string) (bool, error) {
	indexerID = strings.TrimSpace(indexerID)
	if indexerID == "" {
		return false, nil
	}

	for i := len(m.WaitForUpsert.StakeIndexerRegisterList) - 1; i >= 0; i-- {
		item := m.WaitForUpsert.StakeIndexerRegisterList[i]
		if strings.EqualFold(strings.TrimSpace(item.IndexerID), indexerID) {
			return true, nil
		}
	}

	reg, err := pgdb.GetStakeIndexerRegisterByID(m.ctx, indexerID)
	if err != nil {
		return false, fmt.Errorf("load indexer register from pg failed: %w", err)
	}
	if reg == nil {
		return false, nil
	}

	return true, nil
}

func (m *Manager) getIndexerUserAddressForAuth(indexerID string) (string, error) {
	indexerID = strings.TrimSpace(indexerID)
	if indexerID == "" {
		return "", nil
	}

	for i := len(m.WaitForUpsert.StakeIndexerRegisterList) - 1; i >= 0; i-- {
		item := m.WaitForUpsert.StakeIndexerRegisterList[i]
		if item.IndexerID != indexerID {
			continue
		}
		for j := len(m.WaitForUpsert.FIP101InscriptionEvents) - 1; j >= 0; j-- {
			event := m.WaitForUpsert.FIP101InscriptionEvents[j]
			if !strings.EqualFold(event.TxID, item.RegisterTxID) {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(event.Op), "register_indexer") {
				continue
			}
			actorAddress := strings.TrimSpace(event.UserAddress)
			if actorAddress != "" {
				return actorAddress, nil
			}
		}
	}

	reg, err := pgdb.GetStakeIndexerRegisterByID(m.ctx, indexerID)
	if err != nil {
		return "", fmt.Errorf("load indexer owner address from pg failed: %w", err)
	}
	if reg == nil {
		return "", nil
	}
	if userAddress := strings.TrimSpace(reg.UserAddress); userAddress != "" {
		return userAddress, nil
	}
	return "", nil
}
