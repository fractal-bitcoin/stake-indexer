package indexer

import (
	"fmt"
	"math"
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

const stakeStatusRewardReleasePercentField = "reward_release_percent"
const rewardTruncationEpsilon = 1e-9

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
	validProofs, err := pgdb.ResolveStakeProofValidityByProveHeight(
		m.ctx,
		block.Height,
		resolveRewardProofWindow(block.Height),
		blockHash,
		stateHash,
		conf.StakeRewardCfg.DelaySubmitTriggerBlocks,
	)
	if err != nil {
		return fmt.Errorf("resolve proof validity failed: %w", err)
	}
	if len(validProofs) == 0 {
		return nil
	}

	proofPenaltyByIndexer := make(map[string]bool, len(validProofs))
	for _, item := range validProofs {
		if item.IndexerID == "" {
			continue
		}
		proofPenaltyByIndexer[item.IndexerID] = proofPenaltyByIndexer[item.IndexerID] || item.VerifyStatus == pgdb.StakeProofVerifyValidDelayed
	}
	if len(proofPenaltyByIndexer) == 0 {
		return nil
	}

	type indexerStakeWeight struct {
		raw       uint64
		effective uint64
		penalized bool
	}

	indexerStakeTotal := make(map[string]indexerStakeWeight, len(proofPenaltyByIndexer))
	totalEffectiveStake := uint64(0)
	totalRawStake := uint64(0)

	for indexerID, penalized := range proofPenaltyByIndexer {
		logger.Log.Info("handleBlockReward", zap.String("indexer_id", indexerID), zap.Bool("delayed_valid_status", penalized))
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

		effectiveStake := rawStake
		if penalized {
			effectiveStake = rawStake * 95 / 100
		}
		if effectiveStake == 0 {
			continue
		}

		indexerStakeTotal[indexerID] = indexerStakeWeight{raw: rawStake, effective: effectiveStake, penalized: penalized}
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

		indexerRatio, err := m.getIndexerSnapshotRatio(indexerID)
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

		m.stageHIncrByFloat(constant.GetIndexerInfoKey(indexerID), "reward_total", float64(firstLayerReward))
		m.stageHIncrByFloat(constant.GetIndexerInfoKey(indexerID), "self_reward_total", float64(indexerReward))
		if indexerReward > 0 {
			userAddress, err := m.getIndexerUserAddress(indexerID)
			if err != nil {
				return err
			}
			if userAddress != "" {
				m.stageZIncrBy(constant.GetIndexerRewardsKey(userAddress), float64(indexerReward), indexerID)
				m.stageIncrBy(constant.GetIndexerRewardsTotalKey(userAddress), int64(indexerReward))

				allocatedRewards = append(allocatedRewards, pgdb.StakeAllocatedReward{
					UserAddress:          userAddress,
					IndexerID:            indexerID,
					StakeAddress:         indexerID,
					RewardType:           pgdb.StakeRewardTypeIndexer,
					Height:               block.Height,
					StakeAmountSnapshot:  weights.raw,
					StakeAmountEffective: weights.effective,
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
			m.stageZIncrBy(constant.GetStakeRewardsKey(address), float64(reward), indexerID)
			m.stageIncrBy(constant.GetStakeRewardsTotalKey(address), int64(reward))

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
				StakeAmountEffective: weights.effective,
				TotalEffectiveStake:  totalEffectiveStake,
				ReleasePercent:       releasePercent,
				BlockRewardAmount:    rewardAmount,
				IndexerRatio:         indexerRatio,
				AllocateAmount:       reward,
			})
		}
	}

	if len(allocatedRewards) > 0 {
		m.stageHSet(constant.GetStakeIndexerStatusKey(), map[string]interface{}{
			"latest_allocated_reward_height": block.Height,
			"latest_allocated_reward_amount": unlockedRewardAmount,
		})
		if err := pgdb.UpsertStakeAllocatedRewardBatch(m.ctx, allocatedRewards); err != nil {
			return fmt.Errorf("upsert stake allocated rewards failed: %w", err)
		}
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

func (m *Manager) getIndexerLatestRatio(indexerID string) (float64, error) {
	if ratio, ok := m.getCachedIndexerRatio(indexerID); ok {
		return ratio, nil
	}

	values, err := rdb.RdbBalanceClient.HMGet(m.ctx, constant.GetIndexerInfoKey(indexerID), "index_ratio").Result()
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

func (m *Manager) getIndexerSnapshotRatio(indexerID string) (float64, error) {
	if ratio, ok := m.getCachedIndexerSnapshotRatio(indexerID); ok {
		return ratio, nil
	}

	raw, err := rdb.RdbBalanceClient.HGet(m.ctx, constant.REDIS_STAKE_INDEXER_RATIO_SNAPSHOT_KEY, indexerID).Result()
	if err != nil && err != redis.Nil {
		return 0, fmt.Errorf("load indexer snapshot ratio failed: %w", err)
	}
	if ratio, ok := protocolparser.ParseRatio(raw); ok {
		return ratio, nil
	}

	return m.getIndexerLatestRatio(indexerID)
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
	return height >= constant.REWARD_ALLOCATION_STAGE2_CHECKPOINT_HEIGHT
}

func resolveRewardProofWindow(height uint32) uint32 {
	if shouldUseRewardTruncation(height) {
		return constant.REWARD_ALLOCATION_STAGE2_PROOF_WINDOW
	}
	return conf.StakeRewardCfg.ProofWindow
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
