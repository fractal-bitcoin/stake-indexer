package indexer

import (
	"fmt"
	"strconv"
	"strings"

	"stake_indexer/conf"
	"stake_indexer/constant"
	pgdb "stake_indexer/internal/component/pg"
	rdb "stake_indexer/internal/component/redis"
	protocolparser "stake_indexer/internal/parser/protocol"

	redis "github.com/go-redis/redis/v8"
)

const (
	maxCommissionRatio = 0.15
)

type delayedCommissionRatio struct {
	Ratio           float64
	EventHeight     uint32
	EffectiveHeight uint32
}

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
	if !ok || !isValidCommissionRatio(ratio) {
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
	delayed := delayedCommissionRatio{
		Ratio:           ratio,
		EventHeight:     height,
		EffectiveHeight: height + conf.StakeRewardCfg.CommissionActivationBlocks,
	}

	if updateLatest {
		delayedKey := m.getIndexerInfoDelayedCommissionKey()
		infoKey := m.getIndexerInfoKey(indexerID)
		if err := m.stageCommissionRatioForHeight(
			height,
			indexerID,
			delayedKey,
			infoKey,
			"index_ratio",
		); err != nil {
			return err
		}
		m.stageDelayedCommissionRatio(delayedKey, indexerID, delayed)
		m.stageHSet(infoKey, map[string]interface{}{"last_update_height": height})
	}
	if updateSnapshot {
		delayedKey := constant.REDIS_STAKE_INDEXER_RATIO_SNAPSHOT_DELAYED_COMMISSION_KEY
		snapshotKey := constant.REDIS_STAKE_INDEXER_RATIO_SNAPSHOT_KEY
		if m != nil && m.pendingRewardMode {
			delayedKey = constant.REDIS_STAKE_PENDING_INDEXER_RATIO_SNAPSHOT_DELAYED_COMMISSION_KEY
			snapshotKey = constant.REDIS_STAKE_PENDING_INDEXER_RATIO_SNAPSHOT_KEY
		}
		if err := m.stageCommissionRatioForHeight(height, indexerID, delayedKey, snapshotKey, indexerID); err != nil {
			return err
		}
		m.stageDelayedCommissionRatio(delayedKey, indexerID, delayed)
	}

	return nil
}

func isValidCommissionRatio(ratio float64) bool {
	return ratio >= 0 && ratio <= maxCommissionRatio
}

func formatDelayedCommissionRatio(item delayedCommissionRatio) string {
	return strings.Join([]string{
		protocolparser.FormatRatio(item.Ratio),
		strconv.FormatUint(uint64(item.EventHeight), 10),
		strconv.FormatUint(uint64(item.EffectiveHeight), 10),
	}, "|")
}

func parseDelayedCommissionRatio(raw string) (delayedCommissionRatio, bool) {
	parts := strings.Split(strings.TrimSpace(raw), "|")
	if len(parts) != 3 {
		return delayedCommissionRatio{}, false
	}
	ratio, ok := protocolparser.ParseRatio(parts[0])
	if !ok {
		return delayedCommissionRatio{}, false
	}
	eventHeight, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 32)
	if err != nil {
		return delayedCommissionRatio{}, false
	}
	effectiveHeight, err := strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 32)
	if err != nil {
		return delayedCommissionRatio{}, false
	}
	return delayedCommissionRatio{
		Ratio:           ratio,
		EventHeight:     uint32(eventHeight),
		EffectiveHeight: uint32(effectiveHeight),
	}, true
}

func (m *Manager) getStagedDelayedCommissionRatio(key, indexerID string) (delayedCommissionRatio, bool) {
	if m == nil || m.slowState == nil || key == "" || indexerID == "" {
		return delayedCommissionRatio{}, false
	}
	values := m.slowState.hset[key]
	if len(values) == 0 {
		return delayedCommissionRatio{}, false
	}
	raw, ok := values[indexerID]
	if !ok {
		return delayedCommissionRatio{}, false
	}
	return parseDelayedCommissionRatio(fmt.Sprint(raw))
}

func (m *Manager) loadDelayedCommissionRatio(key, indexerID string) (delayedCommissionRatio, bool, error) {
	if item, ok := m.getStagedDelayedCommissionRatio(key, indexerID); ok {
		return item, true, nil
	}
	raw, err := rdb.RdbBalanceClient.HGet(m.ctx, key, indexerID).Result()
	if err == redis.Nil {
		return delayedCommissionRatio{}, false, nil
	}
	if err != nil {
		return delayedCommissionRatio{}, false, fmt.Errorf("load delayed commission ratio failed: %w", err)
	}
	item, ok := parseDelayedCommissionRatio(raw)
	return item, ok, nil
}

func (m *Manager) stageDelayedCommissionRatio(key, indexerID string, item delayedCommissionRatio) {
	m.stageHSet(key, map[string]interface{}{
		indexerID: formatDelayedCommissionRatio(item),
	})
}

func (m *Manager) hasUneffectiveDelayedCommissionRatio(indexerID string, height uint32) (bool, error) {
	item, ok, err := m.loadDelayedCommissionRatio(constant.REDIS_STAKE_INDEXER_DELAYED_COMMISSION_KEY, indexerID)
	if err != nil || !ok {
		return false, err
	}
	return item.EffectiveHeight > height, nil
}

func (m *Manager) stageCommissionRatioForHeight(height uint32, indexerID, delayedKey, targetKey, targetField string) error {
	item, ok, err := m.loadDelayedCommissionRatio(delayedKey, indexerID)
	if err != nil || !ok {
		return err
	}
	if item.EffectiveHeight > height {
		return nil
	}
	m.stageHSet(targetKey, map[string]interface{}{
		targetField: protocolparser.FormatRatio(item.Ratio),
	})
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
