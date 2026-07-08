package indexer

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"stake_indexer/conf"
	"stake_indexer/constant"
	logger "stake_indexer/internal/component/log"
	"stake_indexer/internal/component/node"
	pgdb "stake_indexer/internal/component/pg"
	rdb "stake_indexer/internal/component/redis"
	"stake_indexer/internal/component/stateapi"
	protocolparser "stake_indexer/internal/parser/protocol"

	redis "github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

const (
	stakeStatusRewardReleasePercentField = "reward_release_percent"
	stakeStatusPendingRewardSyncHeight   = "pending_reward_sync_height"
	stakeStatusPendingRewardTotalAmount  = "pending_reward_total_amount"
)
const (
	rewardTruncationEpsilon              = 1e-9
	delaySubmitStage1StakePercent uint64 = 95
)

func payloadFromRatioEvent(event pgdb.FIP101InscriptionEvent) (*protocolparser.OpReturnPayload, bool) {
	content := strings.TrimSpace(event.InscriptionContent)
	if content == "" {
		return nil, false
	}

	payload, _, err := protocolparser.ParseFIP101PayloadFromCSV(
		[]byte(content),
		nil,
		strings.TrimSpace(event.UserAddress),
		"",
	)
	if err != nil || payload == nil {
		return nil, false
	}
	if payload.Tag != protocolparser.TagAllocatRatio {
		return nil, false
	}
	return payload, true
}

func (m *Manager) ApplyFIP101EventsForSnapshot(height uint32, events []pgdb.FIP101InscriptionEvent) error {
	if m == nil {
		return nil
	}
	for _, event := range events {
		if event.BizInvalidFlags != 0 {
			continue
		}
		if strings.TrimSpace(strings.ToLower(event.Op)) != "commission_rate" {
			continue
		}

		payload, ok := payloadFromRatioEvent(event)
		if !ok {
			continue
		}

		tx := &protocolparser.TxSnapshot{
			TxID:  event.TxID,
			TxIdx: event.TxIdx,
		}
		if err := m.handleUpdateRatioTxSnapshot(height, tx, payload); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) handleBlockReward(block *protocolparser.BlockSnapshot) error {
	if block.Height < conf.StakeRewardCfg.StartRewardHeight {
		return nil
	}
	if !isRewardBlockVersion(block.Version) {
		return nil
	}

	rewardAmount := block.CoinbaseReward
	if rewardAmount == 0 {
		return nil
	}

	logger.Log.Info("handleBlockReward", zap.Uint32("block_height", block.Height))

	blockHash, stateHash, err := m.resolveProofHashInputs(block.Height)
	if err != nil {
		return fmt.Errorf("resolve proof hash inputs failed: %w", err)
	}
	validProofs, err := m.resolveStakeProofValidityForReward(block.Height, blockHash, stateHash)
	if err != nil {
		return fmt.Errorf("resolve proof validity failed: %w", err)
	}
	if len(validProofs) == 0 {
		return nil
	}

	stakePercentByIndexer := resolveRewardStakePercentByIndexer(block.Height, validProofs)
	if len(stakePercentByIndexer) < len(validProofs) {
		for _, item := range validProofs {
			if strings.TrimSpace(item.IndexerID) == "" || conf.StakeRewardCfg.IsIndexerRewardAllowedAtHeight(item.IndexerID, block.Height) {
				continue
			}
			logger.Log.Info("handleBlockReward skip indexer outside reward allowlist",
				zap.Uint32("block_height", block.Height),
				zap.String("indexer_id", item.IndexerID),
			)
		}
	}
	if len(stakePercentByIndexer) == 0 {
		return nil
	}

	type indexerStakeWeight struct {
		raw              uint64
		effective        uint64
		effectivePercent uint64
		penalized        bool
	}

	indexerStakeTotal := make(map[string]indexerStakeWeight, len(stakePercentByIndexer))
	totalEffectiveStake := uint64(0)
	totalRawStake := uint64(0)

	for indexerID, stakePercent := range stakePercentByIndexer {
		penalized := stakePercent < 100
		logger.Log.Info("handleBlockReward", zap.String("indexer_id", indexerID), zap.Bool("delayed_valid_status", penalized), zap.Uint64("effective_stake_percent", stakePercent))
		addrStakeAmount, err := m.getIndexerStakeAmount(indexerID)
		if err != nil {
			return err
		}
		if len(addrStakeAmount) == 0 {
			continue
		}

		rawStake := uint64(0)
		for _, stakeAmount := range addrStakeAmount {
			rawStake += uint64(stakeAmount)
		}
		if rawStake == 0 {
			continue
		}

		effectiveStake := rawStake * stakePercent / 100
		if effectiveStake == 0 {
			continue
		}

		indexerStakeTotal[indexerID] = indexerStakeWeight{raw: rawStake, effective: effectiveStake, effectivePercent: stakePercent, penalized: penalized}
		totalRawStake += rawStake
		totalEffectiveStake += effectiveStake
	}

	if totalEffectiveStake == 0 {
		return nil
	}

	releasePercent, err := m.resolveRewardReleasePercent(block.Height)
	if err != nil {
		return err
	}
	useTruncation := shouldUseRewardTruncation(block.Height)
	m.stageHSet(constant.GetStakeIndexerStatusKey(), map[string]interface{}{
		stakeStatusRewardReleasePercentField: releasePercent,
	})

	unlockedRewardAmount := quantizeReward(float64(rewardAmount)*releasePercent/100.0, useTruncation)
	if unlockedRewardAmount == 0 {
		return nil
	}

	allocatedRewards := make([]pgdb.StakeAllocatedReward, 0, len(indexerStakeTotal)*8)

	logger.Log.Info("handleBlockReward",
		zap.Uint64("reward_amount", rewardAmount),
		zap.Float64("release_percent", releasePercent),
		zap.Uint64("unlocked_reward_amount", unlockedRewardAmount),
		zap.Uint64("total_effective_stake", totalEffectiveStake),
		zap.Uint64("total_raw_stake", totalRawStake),
	)

	for indexerID, weights := range indexerStakeTotal {
		firstLayerRewardPercent := float64(weights.effective) / float64(totalEffectiveStake)
		firstLayerReward := quantizeReward(float64(unlockedRewardAmount)*firstLayerRewardPercent, useTruncation)

		indexerRatio, err := m.getIndexerSnapshotRatio(block.Height, indexerID)
		if err != nil {
			return err
		}

		indexerReward := quantizeReward(float64(firstLayerReward)*indexerRatio, useTruncation)
		if indexerReward > firstLayerReward {
			indexerReward = firstLayerReward
		}
		addressRewardPool := firstLayerReward - indexerReward

		logger.Log.Info("handleBlockReward",
			zap.String("indexer_id", indexerID),
			zap.Uint64("effective_stake", weights.effective),
			zap.Uint64("total_effective_stake", totalEffectiveStake),
			zap.Uint64("first_layer_reward", firstLayerReward),
			zap.Uint64("indexer_reward", indexerReward),
			zap.Uint64("address_reward_pool", addressRewardPool),
		)

		m.stageHIncrByFloat(m.getIndexerInfoKey(indexerID), m.indexerRewardTotalField(), float64(firstLayerReward))
		m.stageHIncrByFloat(m.getIndexerInfoKey(indexerID), m.indexerSelfRewardTotalField(), float64(indexerReward))
		if indexerReward > 0 {
			userAddress, err := m.getIndexerUserAddress(indexerID)
			if err != nil {
				return err
			}
			if userAddress != "" {
				m.stageZIncrBy(m.getIndexerRewardsKey(userAddress), float64(indexerReward), indexerID)
				m.stageIncrBy(m.getIndexerRewardsTotalKey(userAddress), int64(indexerReward))

				allocatedRewards = append(allocatedRewards, pgdb.StakeAllocatedReward{
					UserAddress:          userAddress,
					IndexerID:            indexerID,
					StakeAddress:         indexerID,
					RewardType:           pgdb.StakeRewardTypeIndexer,
					Height:               block.Height,
					StakeAmountSnapshot:  weights.raw,
					IndexerTotalStake:    weights.raw,
					IndexerEffectivePct:  float64(weights.effectivePercent),
					StakeAmountEffective: weights.effective,
					PlatformTotalStake:   totalRawStake,
					TotalEffectiveStake:  totalEffectiveStake,
					ReleasePercent:       releasePercent,
					BlockRewardAmount:    rewardAmount,
					IndexerRatio:         indexerRatio,
					AllocateAmount:       indexerReward,
				})
			}
		}

		for address, stakeAmount := range m.indexerToAddrStakeAmount[indexerID] {
			if stakeAmount == 0 || addressRewardPool == 0 {
				continue
			}
			addressRewardPercent := float64(stakeAmount) / float64(weights.raw)
			reward := quantizeReward(float64(addressRewardPool)*addressRewardPercent, useTruncation)
			if reward == 0 {
				continue
			}
			m.stageZIncrBy(m.getStakeRewardsKey(address), float64(reward), indexerID)
			m.stageIncrBy(m.getStakeRewardsTotalKey(address), int64(reward))

			stakeAddress := m.getStakeAddressByIndexerAndUser(indexerID, address)
			if stakeAddress == "" {
				continue
			}
			allocatedRewards = append(allocatedRewards, pgdb.StakeAllocatedReward{
				UserAddress:          address,
				IndexerID:            indexerID,
				StakeAddress:         stakeAddress,
				RewardType:           pgdb.StakeRewardTypeStake,
				Height:               block.Height,
				StakeAmountSnapshot:  stakeAmount,
				IndexerTotalStake:    weights.raw,
				IndexerEffectivePct:  float64(weights.effectivePercent),
				StakeAmountEffective: weights.effective,
				PlatformTotalStake:   totalRawStake,
				TotalEffectiveStake:  totalEffectiveStake,
				ReleasePercent:       releasePercent,
				BlockRewardAmount:    rewardAmount,
				IndexerRatio:         indexerRatio,
				AllocateAmount:       reward,
			})
		}
	}

	if len(allocatedRewards) > 0 {
		if m.pendingRewardMode {
			existingPendingItems, err := pgdb.ListStakePendingRewardsByHeight(m.ctx, block.Height)
			if err != nil {
				return fmt.Errorf("list existing stake pending rewards by height failed: %w", err)
			}
			existingTotalPendingAmount := uint64(0)
			for _, item := range existingPendingItems {
				existingTotalPendingAmount += item.PendingAmount
			}

			pendingRewards := make([]pgdb.StakePendingReward, 0, len(allocatedRewards))
			totalPendingAmount := uint64(0)
			for _, item := range allocatedRewards {
				pendingRewards = append(pendingRewards, pgdb.StakePendingReward{
					UserAddress:          item.UserAddress,
					IndexerID:            item.IndexerID,
					StakeAddress:         item.StakeAddress,
					RewardType:           item.RewardType,
					Height:               item.Height,
					StakeAmountSnapshot:  item.StakeAmountSnapshot,
					IndexerTotalStake:    item.IndexerTotalStake,
					IndexerEffectivePct:  item.IndexerEffectivePct,
					StakeAmountEffective: item.StakeAmountEffective,
					PlatformTotalStake:   item.PlatformTotalStake,
					TotalEffectiveStake:  item.TotalEffectiveStake,
					ReleasePercent:       item.ReleasePercent,
					BlockRewardAmount:    item.BlockRewardAmount,
					IndexerRatio:         item.IndexerRatio,
					PendingAmount:        item.AllocateAmount,
				})
				totalPendingAmount += item.AllocateAmount
			}
			if err := pgdb.DeleteStakePendingRewardsByHeight(m.ctx, block.Height); err != nil {
				return fmt.Errorf("delete existing stake pending rewards by height failed: %w", err)
			}
			if err := pgdb.UpsertStakePendingRewardBatch(m.ctx, pendingRewards); err != nil {
				return fmt.Errorf("upsert stake pending rewards failed: %w", err)
			}
			currentPendingTotal := m.resolvePendingRewardTotalAmount()
			if existingTotalPendingAmount >= currentPendingTotal {
				currentPendingTotal = 0
			} else {
				currentPendingTotal -= existingTotalPendingAmount
			}
			m.stageHSet(constant.GetStakeIndexerStatusKey(), map[string]interface{}{
				stakeStatusPendingRewardSyncHeight:  block.Height,
				stakeStatusPendingRewardTotalAmount: currentPendingTotal + totalPendingAmount,
			})
			return nil
		}

		m.stageHSet(constant.GetStakeIndexerStatusKey(), map[string]interface{}{
			"latest_allocated_reward_height": block.Height,
			"latest_allocated_reward_amount": unlockedRewardAmount,
		})
		if err := pgdb.UpsertStakeAllocatedRewardBatch(m.ctx, allocatedRewards); err != nil {
			return fmt.Errorf("upsert stake allocated rewards failed: %w", err)
		}
		if err := m.subtractPendingRewardByHeight(block.Height); err != nil {
			return fmt.Errorf("subtract pending rewards by allocated height failed: %w", err)
		}
	}
	return nil
}

func (m *Manager) resolveStakeProofValidityForReward(height uint32, blockHash, stateHash string) ([]pgdb.StakeProof, error) {
	rules := pgdb.StakeProofValidityRules{
		DelaySubmitTriggerBlocks:     conf.StakeRewardCfg.DelaySubmitTriggerBlocks,
		DelaySubmitStage2StepBlocks:  conf.StakeRewardCfg.DelaySubmitStage2StepBlocks,
		DelaySubmitStage2StepPercent: conf.StakeRewardCfg.DelaySubmitStage2StepPercent,
		Stage2StartHeight:            conf.StakeRewardCfg.Stage2StartHeight,
	}
	if m != nil && m.pendingRewardMode {
		return pgdb.ResolveStakeProofValidityByProveHeightReadOnlyWithRules(
			m.ctx,
			height,
			resolveRewardProofWindow(height),
			blockHash,
			stateHash,
			rules,
		)
	}
	return pgdb.ResolveStakeProofValidityByProveHeightWithRules(
		m.ctx,
		height,
		resolveRewardProofWindow(height),
		blockHash,
		stateHash,
		rules,
	)
}

func resolveRewardStakePercentByIndexer(rewardHeight uint32, proofs []pgdb.StakeProof) map[string]uint64 {
	stakePercentByIndexer := make(map[string]uint64, len(proofs))
	for _, item := range proofs {
		indexerID := strings.TrimSpace(item.IndexerID)
		if indexerID == "" {
			continue
		}
		if !conf.StakeRewardCfg.IsIndexerRewardAllowedAtHeight(indexerID, rewardHeight) {
			continue
		}
		stakePercent := resolveDelaySubmitStakePercent(rewardHeight, item)
		currentPercent, exists := stakePercentByIndexer[indexerID]
		if !exists || stakePercent > currentPercent {
			stakePercentByIndexer[indexerID] = stakePercent
		}
	}
	return stakePercentByIndexer
}

func (m *Manager) getIndexerInfoKey(indexerID string) string {
	if m != nil && m.pendingRewardMode {
		return constant.GetPendingIndexerInfoKey(indexerID)
	}
	return constant.GetIndexerInfoKey(indexerID)
}

func (m *Manager) getIndexerInfoDelayedCommissionKey() string {
	if m != nil && m.pendingRewardMode {
		return constant.REDIS_STAKE_PENDING_INDEXER_DELAYED_COMMISSION_KEY
	}
	return constant.REDIS_STAKE_INDEXER_DELAYED_COMMISSION_KEY
}

func (m *Manager) getStakeRewardsKey(address string) string {
	if m != nil && m.pendingRewardMode {
		return constant.GetPendingStakeRewardsKey(address)
	}
	return constant.GetStakeRewardsKey(address)
}

func (m *Manager) getStakeRewardsTotalKey(address string) string {
	if m != nil && m.pendingRewardMode {
		return constant.GetPendingStakeRewardsTotalKey(address)
	}
	return constant.GetStakeRewardsTotalKey(address)
}

func (m *Manager) getIndexerRewardsKey(address string) string {
	if m != nil && m.pendingRewardMode {
		return constant.GetPendingIndexerRewardsKey(address)
	}
	return constant.GetIndexerRewardsKey(address)
}

func (m *Manager) getIndexerRewardsTotalKey(address string) string {
	if m != nil && m.pendingRewardMode {
		return constant.GetPendingIndexerRewardsTotalKey(address)
	}
	return constant.GetIndexerRewardsTotalKey(address)
}

func (m *Manager) indexerRewardTotalField() string {
	if m != nil && m.pendingRewardMode {
		return "pending_reward_total"
	}
	return "reward_total"
}

func (m *Manager) indexerSelfRewardTotalField() string {
	if m != nil && m.pendingRewardMode {
		return "pending_self_reward_total"
	}
	return "self_reward_total"
}

func (m *Manager) resolvePendingRewardTotalAmount() uint64 {
	if m == nil {
		return 0
	}
	raw, err := rdb.RdbBalanceClient.HGet(m.ctx, constant.GetStakeIndexerStatusKey(), stakeStatusPendingRewardTotalAmount).Result()
	if err != nil {
		return 0
	}
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func (m *Manager) subtractPendingRewardByHeight(height uint32) error {
	if m == nil {
		return nil
	}
	pendingItems, err := pgdb.ListStakePendingRewardsByHeight(m.ctx, height)
	if err != nil {
		return err
	}
	if len(pendingItems) == 0 {
		return nil
	}

	totalPendingAmount := uint64(0)
	for _, item := range pendingItems {
		if item.PendingAmount == 0 {
			continue
		}
		totalPendingAmount += item.PendingAmount
		m.stageHIncrByFloat(constant.GetPendingIndexerInfoKey(item.IndexerID), "pending_reward_total", -float64(item.PendingAmount))
		if item.RewardType == pgdb.StakeRewardTypeIndexer {
			m.stageHIncrByFloat(constant.GetPendingIndexerInfoKey(item.IndexerID), "pending_self_reward_total", -float64(item.PendingAmount))
			m.stageZIncrBy(constant.GetPendingIndexerRewardsKey(item.UserAddress), -float64(item.PendingAmount), item.IndexerID)
			m.stageIncrBy(constant.GetPendingIndexerRewardsTotalKey(item.UserAddress), -int64(item.PendingAmount))
			continue
		}
		m.stageZIncrBy(constant.GetPendingStakeRewardsKey(item.UserAddress), -float64(item.PendingAmount), item.IndexerID)
		m.stageIncrBy(constant.GetPendingStakeRewardsTotalKey(item.UserAddress), -int64(item.PendingAmount))
	}

	currentPendingTotal := m.resolvePendingRewardTotalAmount()
	if totalPendingAmount >= currentPendingTotal {
		currentPendingTotal = 0
	} else {
		currentPendingTotal -= totalPendingAmount
	}
	m.stageHSet(constant.GetStakeIndexerStatusKey(), map[string]interface{}{
		stakeStatusPendingRewardTotalAmount: currentPendingTotal,
	})

	if err := pgdb.DeleteStakePendingRewardsByHeight(m.ctx, height); err != nil {
		return err
	}
	return nil
}

func (m *Manager) getStakeAddressByIndexerAndUser(indexerID, userAddress string) string {
	if m == nil || indexerID == "" || userAddress == "" {
		return ""
	}
	if userMap := m.indexerToUserStakeAddress[indexerID]; userMap != nil {
		if stakeAddress := strings.TrimSpace(userMap[userAddress]); stakeAddress != "" {
			return stakeAddress
		}
	}
	for stakeAddress, info := range m.stakeAddrToIndexer {
		if info.IndexerID != indexerID || info.Address != userAddress {
			continue
		}
		userMap := m.indexerToUserStakeAddress[indexerID]
		if userMap == nil {
			userMap = make(map[string]string)
			m.indexerToUserStakeAddress[indexerID] = userMap
		}
		userMap[userAddress] = stakeAddress
		return stakeAddress
	}
	return ""
}

func (m *Manager) getIndexerLatestRatio(height uint32, indexerID string) (float64, error) {
	infoKey := m.getIndexerInfoKey(indexerID)
	delayedKey := m.getIndexerInfoDelayedCommissionKey()
	if height > 0 {
		if err := m.stageCommissionRatioForHeight(
			height,
			indexerID,
			delayedKey,
			infoKey,
			"index_ratio",
		); err != nil {
			return 0, err
		}
	}
	if ratio, ok := m.getCachedIndexerRatio(indexerID); ok {
		return ratio, nil
	}

	values, err := rdb.RdbBalanceClient.HMGet(m.ctx, infoKey, "index_ratio").Result()
	if err != nil {
		return 0, fmt.Errorf("load indexer ratio failed: %w", err)
	}
	for _, value := range values {
		strValue, ok := value.(string)
		if !ok {
			continue
		}
		ratio, ok := protocolparser.ParseRatio(strValue)
		if ok {
			return ratio, nil
		}
	}

	reg, err := pgdb.GetStakeIndexerRegisterByID(m.ctx, indexerID)
	if err != nil {
		return 0, fmt.Errorf("load indexer ratio from pg failed: %w", err)
	}
	if reg != nil {
		return reg.IndexRatio, nil
	}
	return 0, nil
}

func (m *Manager) getIndexerSnapshotRatio(height uint32, indexerID string) (float64, error) {
	snapshotKey := constant.REDIS_STAKE_INDEXER_RATIO_SNAPSHOT_KEY
	delayedKey := constant.REDIS_STAKE_INDEXER_RATIO_SNAPSHOT_DELAYED_COMMISSION_KEY
	if m != nil && m.pendingRewardMode {
		snapshotKey = constant.REDIS_STAKE_PENDING_INDEXER_RATIO_SNAPSHOT_KEY
		delayedKey = constant.REDIS_STAKE_PENDING_INDEXER_RATIO_SNAPSHOT_DELAYED_COMMISSION_KEY
	}
	if err := m.stageCommissionRatioForHeight(height, indexerID, delayedKey, snapshotKey, indexerID); err != nil {
		return 0, err
	}

	if ratio, ok := m.getCachedIndexerSnapshotRatio(indexerID); ok {
		return ratio, nil
	}

	raw, err := rdb.RdbBalanceClient.HGet(m.ctx, snapshotKey, indexerID).Result()
	if err != nil && err != redis.Nil {
		return 0, fmt.Errorf("load indexer snapshot ratio failed: %w", err)
	}
	if ratio, ok := protocolparser.ParseRatio(raw); ok {
		return ratio, nil
	}

	return m.getIndexerLatestRatio(height, indexerID)
}

func (m *Manager) getIndexerRewardAddress(indexerID string) (string, error) {
	if indexerID == "" {
		return "", nil
	}

	reg, err := pgdb.GetStakeIndexerRegisterByID(m.ctx, indexerID)
	if err != nil {
		return "", fmt.Errorf("load indexer reward address from pg failed: %w", err)
	}
	if reg == nil {
		return "", nil
	}
	return strings.TrimSpace(reg.RewardAddress), nil
}

func (m *Manager) getIndexerUserAddress(indexerID string) (string, error) {
	indexerID = strings.TrimSpace(indexerID)
	if indexerID == "" {
		return "", nil
	}

	for i := len(m.WaitForUpsert.StakeIndexerRegisterList) - 1; i >= 0; i-- {
		item := m.WaitForUpsert.StakeIndexerRegisterList[i]
		if strings.TrimSpace(item.IndexerID) != indexerID {
			continue
		}
		if userAddress := strings.TrimSpace(item.UserAddress); userAddress != "" {
			return userAddress, nil
		}
	}

	reg, err := pgdb.GetStakeIndexerRegisterByID(m.ctx, indexerID)
	if err != nil {
		return "", fmt.Errorf("load indexer user address from pg failed: %w", err)
	}
	if reg == nil {
		return "", nil
	}
	return strings.TrimSpace(reg.UserAddress), nil
}

func (m *Manager) resolveRewardReleasePercent(height uint32) (float64, error) {
	releasePercent := conf.StakeRewardCfg.RewardReleasePercentByHeight(height)
	if releasePercent > 100 {
		releasePercent = 100
	}
	return releasePercent, nil
}

func isRewardBlockVersion(version uint32) bool {
	hexVersion := fmt.Sprintf("%08x", version)
	return strings.HasPrefix(hexVersion, "2026") && len(hexVersion) > 5 && hexVersion[5] == '1'
}

func shouldUseRewardTruncation(height uint32) bool {
	return height >= conf.StakeRewardCfg.Stage2StartHeight
}

func resolveRewardProofWindow(height uint32) uint32 {
	if shouldUseRewardTruncation(height) {
		return constant.REWARD_ALLOCATION_STAGE2_PROOF_WINDOW
	}
	return conf.StakeRewardCfg.ProofWindow
}

func resolveDelaySubmitStakePercent(rewardHeight uint32, proof pgdb.StakeProof) uint64 {
	if proof.VerifyStatus != pgdb.StakeProofVerifyValidDelayed {
		return 100
	}
	if !shouldUseRewardTruncation(rewardHeight) {
		return delaySubmitStage1StakePercent
	}
	stepBlocks := conf.StakeRewardCfg.DelaySubmitStage2StepBlocks
	stepPercent := conf.StakeRewardCfg.DelaySubmitStage2StepPercent
	if proof.Height <= proof.ProveBlockHeight || stepBlocks == 0 || stepPercent == 0 {
		return 100
	}
	delayedBlocks := proof.Height - proof.ProveBlockHeight
	steps := uint64((delayedBlocks - 1) / stepBlocks)
	penaltyPercent := steps * stepPercent
	if penaltyPercent >= 100 {
		return 0
	}
	return 100 - penaltyPercent
}

func quantizeReward(value float64, useTruncation bool) uint64 {
	if value <= 0 {
		return 0
	}
	if useTruncation {
		return uint64(math.Floor(value + rewardTruncationEpsilon))
	}
	return uint64(math.Round(value))
}

func (m *Manager) resolveProofHashInputs(height uint32) (string, string, error) {
	blockHash := ""
	syncBlock, err := pgdb.GetSyncBlock(m.ctx, height)
	if err != nil {
		return "", "", fmt.Errorf("load sync block failed: %w", err)
	}
	if syncBlock != nil {
		blockHash = strings.TrimSpace(syncBlock.BlockHash)
	}
	if blockHash == "" {
		blockHash, err = node.GetBlockHashRPC(height)
		if err != nil {
			return "", "", fmt.Errorf("get block hash from rpc failed: %w", err)
		}
	}

	state, err := stateapi.GetHeightState(m.ctx, uint64(height))
	if err != nil {
		return "", "", fmt.Errorf("get state hash failed: %w", err)
	}
	stateHash := strings.TrimSpace(state.StateHash)
	if stateHash == "" {
		return "", "", fmt.Errorf("state hash is empty for height %d", height)
	}
	if blockHash == "" {
		blockHash = strings.TrimSpace(state.BlockHash)
	}
	if blockHash == "" {
		return "", "", fmt.Errorf("block hash is empty for height %d", height)
	}
	return blockHash, stateHash, nil
}
