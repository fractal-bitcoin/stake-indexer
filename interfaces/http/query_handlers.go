package api

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"stake_indexer/constant"
	"stake_indexer/internal/component/node"
	pgdb "stake_indexer/internal/component/pg"
	rdb "stake_indexer/internal/component/redis"
	rewardsyncsvc "stake_indexer/internal/entry/slow"
	protocolparser "stake_indexer/internal/parser/protocol"

	"github.com/gin-gonic/gin"
	redis "github.com/go-redis/redis/v8"
)

var ctx = context.Background()

const (
	errorCodeParamsInvalid = 1001
	errorCodeNotFound      = 1002
	errorCodeInternal      = 1003
)

const (
	defaultPageLimit = 20
	maxPageLimit     = 500
)

func validatePageParams(params *PageReqParams) error {
	if params.Limit == 0 {
		params.Limit = defaultPageLimit
	}
	if params.Limit > maxPageLimit {
		return fmt.Errorf("limit must be <= %d", maxPageLimit)
	}
	if params.Start < 0 {
		return fmt.Errorf("start must be >= 0")
	}
	return nil
}

func paginateIndexerItems(items []IndexerListItem, start, limit int) []IndexerListItem {
	if start >= len(items) {
		return []IndexerListItem{}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func paginateStakerItems(items []StakerItem, start, limit int) []StakerItem {
	if start >= len(items) {
		return []StakerItem{}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func paginateUserStakingItems(items []UserStakingItem, start, limit int) []UserStakingItem {
	if start >= len(items) {
		return []UserStakingItem{}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func loadLatestIndexerRatios(indexerIDs []string) (map[string]float64, error) {
	if len(indexerIDs) == 0 {
		return map[string]float64{}, nil
	}

	pipe := rdb.RdbBalanceClient.Pipeline()
	defer pipe.Close()

	cmds := make(map[string]*redis.StringCmd, len(indexerIDs))
	for _, indexerID := range indexerIDs {
		if indexerID == "" {
			continue
		}
		cmds[indexerID] = pipe.HGet(ctx, constant.GetIndexerInfoKey(indexerID), "index_ratio")
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}

	result := make(map[string]float64, len(cmds))
	for indexerID, cmd := range cmds {
		if cmd == nil {
			continue
		}
		raw, err := cmd.Result()
		if err != nil || raw == "" {
			continue
		}
		ratio, parseErr := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if parseErr != nil {
			continue
		}
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
		result[indexerID] = ratio
	}
	return result, nil
}

func applyStakeDelta(base uint64, delta int64) uint64 {
	if delta == 0 {
		return base
	}
	if delta > 0 {
		return base + uint64(delta)
	}
	dec := uint64(-delta)
	if dec >= base {
		return 0
	}
	return base - dec
}

func loadMempoolIndexerDeltas(indexerIDs []string) (map[string]int64, error) {
	if len(indexerIDs) == 0 {
		return map[string]int64{}, nil
	}

	pipe := rdb.RdbBalanceClient.Pipeline()
	defer pipe.Close()

	key := constant.GetStakeMempoolIndexerDeltaKey()
	cmds := make(map[string]*redis.StringCmd, len(indexerIDs))
	for _, indexerID := range indexerIDs {
		indexerID = strings.TrimSpace(indexerID)
		if indexerID == "" {
			continue
		}
		cmds[indexerID] = pipe.HGet(ctx, key, indexerID)
	}

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}

	result := make(map[string]int64, len(cmds))
	for indexerID, cmd := range cmds {
		if cmd == nil {
			continue
		}
		raw, err := cmd.Result()
		if err != nil || strings.TrimSpace(raw) == "" {
			continue
		}
		delta, parseErr := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if parseErr != nil || delta == 0 {
			continue
		}
		result[indexerID] = delta
	}
	return result, nil
}

func loadMempoolIndexerStakerDeltas(indexerID string) (map[string]int64, error) {
	indexerID = strings.TrimSpace(indexerID)
	if indexerID == "" {
		return map[string]int64{}, nil
	}

	rawMap, err := rdb.RdbBalanceClient.HGetAll(ctx, constant.GetStakeMempoolIndexerStakerDeltaKey(indexerID)).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	if len(rawMap) == 0 {
		return map[string]int64{}, nil
	}

	result := make(map[string]int64, len(rawMap))
	for userAddress, raw := range rawMap {
		userAddress = strings.TrimSpace(userAddress)
		if userAddress == "" || strings.TrimSpace(raw) == "" {
			continue
		}
		delta, parseErr := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if parseErr != nil || delta == 0 {
			continue
		}
		result[userAddress] = delta
	}
	return result, nil
}

func listMempoolEventsAll(op, userAddress, rewardAddress, indexerID string) ([]pgdb.StakeMempoolEvent, error) {
	total, err := pgdb.CountStakeMempoolEvents(ctx, op, userAddress, rewardAddress, indexerID)
	if err != nil {
		return nil, err
	}
	if total <= 0 {
		return []pgdb.StakeMempoolEvent{}, nil
	}
	return pgdb.ListStakeMempoolEvents(ctx, op, userAddress, rewardAddress, indexerID, total, 0)
}

func pendingIndexerIDFromEvent(event pgdb.StakeMempoolEvent) string {
	if strings.TrimSpace(event.IndexerID) != "" {
		return strings.TrimSpace(event.IndexerID)
	}
	if strings.TrimSpace(event.TxID) != "" {
		return "pending:" + strings.TrimSpace(event.TxID)
	}
	return "pending"
}

func knownIndexerName(indexerID string, names map[string]string) string {
	if name := strings.TrimSpace(names[indexerID]); name != "" {
		return name
	}
	if indexerID == "" {
		return ""
	}
	return indexerID
}

func buildConfirmedIndexerItems(registers []pgdb.StakeIndexerRegister, withStakeRatio bool) ([]IndexerListItem, uint64, error) {
	if len(registers) == 0 {
		return []IndexerListItem{}, 0, nil
	}

	indexerIDs := make([]string, 0, len(registers))
	for _, reg := range registers {
		if reg.IndexerID != "" {
			indexerIDs = append(indexerIDs, reg.IndexerID)
		}
	}

	latestRatios, err := loadLatestIndexerRatios(indexerIDs)
	if err != nil {
		return nil, 0, err
	}
	mempoolIndexerDeltas, err := loadMempoolIndexerDeltas(indexerIDs)
	if err != nil {
		return nil, 0, err
	}

	pipe := rdb.RdbBalanceClient.Pipeline()
	defer pipe.Close()

	totalCmds := make(map[string]*redis.StringCmd, len(registers))
	rewardCmds := make(map[string]*redis.StringCmd, len(registers))
	for _, reg := range registers {
		totalCmds[reg.IndexerID] = pipe.Get(ctx, constant.GetIndexerStakeTotalKey(reg.IndexerID))
		rewardCmds[reg.IndexerID] = pipe.HGet(ctx, constant.GetIndexerInfoKey(reg.IndexerID), "reward_total")
	}
	if _, execErr := pipe.Exec(ctx); execErr != nil && execErr != redis.Nil {
		return nil, 0, execErr
	}

	indexerTotals := make(map[string]uint64, len(registers))
	globalTotal := uint64(0)
	for _, reg := range registers {
		val, err := totalCmds[reg.IndexerID].Result()
		total := uint64(0)
		if err == nil {
			total, _ = strconv.ParseUint(val, 10, 64)
		}
		finalTotal := applyStakeDelta(total, mempoolIndexerDeltas[reg.IndexerID])
		indexerTotals[reg.IndexerID] = finalTotal
		globalTotal += finalTotal
	}

	result := make([]IndexerListItem, 0, len(registers))
	for _, reg := range registers {
		total := indexerTotals[reg.IndexerID]
		reward := uint64(0)
		if cmd := rewardCmds[reg.IndexerID]; cmd != nil {
			val, err := cmd.Result()
			if err == nil && val != "" {
				f, _ := strconv.ParseFloat(val, 64)
				reward = uint64(math.Round(f))
			}
		}

		stakeRatio := 0.0
		if withStakeRatio && globalTotal > 0 {
			stakeRatio = float64(total) / float64(globalTotal)
		}

		displayName := reg.Name
		if displayName == "" {
			displayName = reg.IndexerID
		}
		indexRatio := reg.IndexRatio
		if latest, ok := latestRatios[reg.IndexerID]; ok {
			indexRatio = latest
		}

		result = append(result, IndexerListItem{
			IndexerID:       reg.IndexerID,
			Name:            displayName,
			RewardAddress:   reg.RewardAddress,
			UserAddress:     reg.UserAddress,
			IndexRatio:      indexRatio,
			TotalStaked:     total,
			StakeRatio:      stakeRatio,
			AllocatedReward: reward,
			Pending:         mempoolIndexerDeltas[reg.IndexerID] != 0,
		})
	}

	return result, globalTotal, nil
}

func appendPendingRegisterItems(items []IndexerListItem, addressFilter string) ([]IndexerListItem, error) {
	events, err := listMempoolEventsAll(protocolparser.TagRegister, strings.TrimSpace(addressFilter), "", "")
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		indexerID := pendingIndexerIDFromEvent(event)
		name := strings.TrimSpace(event.IndexerName)
		if name == "" {
			name = indexerID
		}
		items = append(items, IndexerListItem{
			IndexerID:     indexerID,
			Name:          name,
			RewardAddress: strings.TrimSpace(event.RewardAddress),
			UserAddress:   strings.TrimSpace(event.UserAddress),
			IndexRatio:    event.IndexRatio,
			TotalStaked:   0,
			StakeRatio:    0,
			Pending:       true,
		})
	}
	return items, nil
}

func sortIndexerItems(items []IndexerListItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].TotalStaked != items[j].TotalStaked {
			return items[i].TotalStaked > items[j].TotalStaked
		}
		if items[i].Pending != items[j].Pending {
			return !items[i].Pending
		}
		return items[i].IndexerID < items[j].IndexerID
	})
}

func GetIndexers(c *gin.Context) (rData ResponseData, err error) {
	var params PageReqParams
	if err := c.ShouldBindQuery(&params); err != nil {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "params invalid"
		return rData, err
	}
	if err := validatePageParams(&params); err != nil {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = err.Error()
		return rData, err
	}

	registers, err := pgdb.ListStakeIndexerRegisters(ctx)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	items, totalStaked, err := buildConfirmedIndexerItems(registers, true)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	items, err = appendPendingRegisterItems(items, "")
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	sortIndexerItems(items)

	rData.Data = ListIndexersResp{
		Total:       len(items),
		TotalStaked: totalStaked,
		Start:       params.Start,
		Detail:      paginateIndexerItems(items, params.Start, params.Limit),
	}
	return rData, nil
}

func loadPendingRegisterItemByAddress(address string) (*IndexerListItem, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, nil
	}

	events, err := pgdb.ListStakeMempoolEvents(ctx, protocolparser.TagRegister, address, "", "", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}

	event := events[0]
	indexerID := pendingIndexerIDFromEvent(event)
	name := strings.TrimSpace(event.IndexerName)
	if name == "" {
		name = indexerID
	}

	item := &IndexerListItem{
		IndexerID:     indexerID,
		Name:          name,
		RewardAddress: strings.TrimSpace(event.RewardAddress),
		UserAddress:   strings.TrimSpace(event.UserAddress),
		IndexRatio:    event.IndexRatio,
		TotalStaked:   0,
		StakeRatio:    0,
		Pending:       true,
	}
	return item, nil
}

func GetIndexerByAddress(c *gin.Context) (rData ResponseData, err error) {
	address := strings.TrimSpace(c.Param("address"))
	if address == "" {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "address required"
		return rData, fmt.Errorf("address required")
	}

	reg, err := pgdb.GetStakeIndexerRegisterByUserAddress(ctx, address)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	if reg != nil {
		items, _, buildErr := buildConfirmedIndexerItems([]pgdb.StakeIndexerRegister{*reg}, false)
		if buildErr != nil {
			rData.Code = errorCodeInternal
			rData.Msg = buildErr.Error()
			return rData, buildErr
		}
		if len(items) > 0 {
			rData.Data = items[0]
			return rData, nil
		}
	}

	pendingItem, pendingErr := loadPendingRegisterItemByAddress(address)
	if pendingErr != nil {
		rData.Code = errorCodeInternal
		rData.Msg = pendingErr.Error()
		return rData, pendingErr
	}
	if pendingItem == nil {
		rData.Code = errorCodeNotFound
		rData.Msg = "indexer not found"
		return rData, fmt.Errorf("indexer not found")
	}

	rData.Data = *pendingItem
	return rData, nil
}

func listStakersByZSet(zsetKey, pendingSetKey string, start, limit int) (int, []StakerItem, error) {
	total64, err := rdb.RdbBalanceClient.ZCount(ctx, zsetKey, "(0", "+inf").Result()
	if err != nil && err != redis.Nil {
		return 0, nil, err
	}
	total := int(total64)
	if total <= 0 || start >= total {
		return total, []StakerItem{}, nil
	}

	end := start + limit - 1
	if end >= total {
		end = total - 1
	}
	items, err := rdb.RdbBalanceClient.ZRevRangeWithScores(ctx, zsetKey, int64(start), int64(end)).Result()
	if err != nil && err != redis.Nil {
		return 0, nil, err
	}

	pendingCmds := make(map[string]*redis.BoolCmd, len(items))
	if strings.TrimSpace(pendingSetKey) != "" && len(items) > 0 {
		pipe := rdb.RdbBalanceClient.Pipeline()
		defer pipe.Close()
		for _, item := range items {
			userAddress, ok := item.Member.(string)
			userAddress = strings.TrimSpace(userAddress)
			if !ok || userAddress == "" {
				continue
			}
			pendingCmds[userAddress] = pipe.SIsMember(ctx, pendingSetKey, userAddress)
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return 0, nil, err
		}
	}

	detail := make([]StakerItem, 0, len(items))
	for _, item := range items {
		userAddress, ok := item.Member.(string)
		userAddress = strings.TrimSpace(userAddress)
		if !ok || userAddress == "" {
			continue
		}
		amount := uint64(math.Round(item.Score))
		if amount == 0 {
			continue
		}
		pending := false
		if cmd := pendingCmds[userAddress]; cmd != nil {
			pending, _ = cmd.Result()
		}
		detail = append(detail, StakerItem{
			UserAddress: userAddress,
			Amount:      amount,
			Pending:     pending,
		})
	}
	return total, detail, nil
}

func GetIndexerStakers(c *gin.Context) (rData ResponseData, err error) {
	indexerID := strings.TrimSpace(c.Param("id"))
	if indexerID == "" {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "indexer_id required"
		return rData, fmt.Errorf("indexer_id required")
	}

	reg, err := pgdb.GetStakeIndexerRegisterByID(ctx, indexerID)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}
	if reg == nil {
		rData.Code = errorCodeNotFound
		rData.Msg = "indexer not found"
		return rData, fmt.Errorf("indexer not found")
	}

	var params PageReqParams
	if err := c.ShouldBindQuery(&params); err != nil {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "params invalid"
		return rData, err
	}
	if err := validatePageParams(&params); err != nil {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = err.Error()
		return rData, err
	}

	useMempoolSnapshot, err := rdb.RdbBalanceClient.SIsMember(ctx, constant.GetStakeMempoolIndexerStakersIndexersKey(), indexerID).Result()
	if err != nil && err != redis.Nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	zsetKey := constant.GetIndexerStakeZsetKey(indexerID)
	pendingSetKey := ""
	if useMempoolSnapshot {
		zsetKey = constant.GetStakeMempoolIndexerStakersKey(indexerID)
		pendingSetKey = constant.GetStakeMempoolIndexerStakersPendingKey(indexerID)
	}

	total, detail, err := listStakersByZSet(zsetKey, pendingSetKey, params.Start, params.Limit)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	displayName := reg.Name
	if displayName == "" {
		displayName = reg.IndexerID
	}
	rData.Data = ListIndexerStakersResp{
		IndexerID: indexerID,
		Name:      displayName,
		Total:     total,
		Start:     params.Start,
		Detail:    detail,
	}
	return rData, nil
}

func GetIndexerProofs(c *gin.Context) (rData ResponseData, err error) {
	indexerID := strings.TrimSpace(c.Param("id"))
	if indexerID == "" {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "indexer_id required"
		return rData, fmt.Errorf("indexer_id required")
	}

	reg, err := pgdb.GetStakeIndexerRegisterByID(ctx, indexerID)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}
	if reg == nil {
		rData.Code = errorCodeNotFound
		rData.Msg = "indexer not found"
		return rData, fmt.Errorf("indexer not found")
	}

	var params PageReqParams
	if err := c.ShouldBindQuery(&params); err != nil {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "params invalid"
		return rData, err
	}
	if err := validatePageParams(&params); err != nil {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = err.Error()
		return rData, err
	}

	total, err := pgdb.CountStakeProofsByIndexerID(ctx, indexerID)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}
	pendingTotal, err := pgdb.CountStakeMempoolEvents(ctx, protocolparser.TagProveStake, "", "", indexerID)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}
	total += pendingTotal

	detail := make([]ProofItem, 0, params.Limit)
	pendingOffset := params.Start
	pendingLimit := 0
	if pendingOffset < pendingTotal {
		pendingLimit = params.Limit
		restPending := pendingTotal - pendingOffset
		if pendingLimit > restPending {
			pendingLimit = restPending
		}
	}
	if pendingLimit > 0 {
		pendingProofs, err := pgdb.ListStakeMempoolEvents(ctx, protocolparser.TagProveStake, "", "", indexerID, pendingLimit, pendingOffset)
		if err != nil {
			rData.Code = errorCodeInternal
			rData.Msg = err.Error()
			return rData, err
		}
		for _, p := range pendingProofs {
			detail = append(detail, ProofItem{
				IndexerID:        p.IndexerID,
				ProveBlockHeight: p.ProveBlockHeight,
				ProveDataHash:    p.ProveDataHash,
				TxID:             p.TxID,
				Height:           constant.MEMPOOL_HEIGHT,
				TxIdx:            p.TxIdx,
				VerifyStatus:     0,
				Pending:          true,
			})
		}
	}

	restLimit := params.Limit - len(detail)
	if restLimit > 0 {
		confirmedOffset := 0
		if params.Start >= pendingTotal {
			confirmedOffset = params.Start - pendingTotal
		}
		proofs, err := pgdb.ListStakeProofsByIndexerID(ctx, indexerID, restLimit, confirmedOffset)
		if err != nil {
			rData.Code = errorCodeInternal
			rData.Msg = err.Error()
			return rData, err
		}
		for _, p := range proofs {
			detail = append(detail, ProofItem{
				IndexerID:        p.IndexerID,
				ProveBlockHeight: p.ProveBlockHeight,
				ProveDataHash:    p.ProveDataHash,
				TxID:             p.TxID,
				Height:           p.Height,
				TxIdx:            p.TxIdx,
				VerifyStatus:     p.VerifyStatus,
			})
		}
	}

	rData.Data = ListIndexerProofsResp{
		IndexerID: indexerID,
		Name:      reg.Name,
		Total:     total,
		Start:     params.Start,
		Detail:    detail,
	}
	return rData, nil
}

func GetUserStakings(c *gin.Context) (rData ResponseData, err error) {
	userAddress := strings.TrimSpace(c.Param("address"))
	if userAddress == "" {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "user address required"
		return rData, fmt.Errorf("user address required")
	}

	var params PageReqParams
	if err := c.ShouldBindQuery(&params); err != nil {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "params invalid"
		return rData, err
	}
	if err := validatePageParams(&params); err != nil {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = err.Error()
		return rData, err
	}

	bindings, err := pgdb.ListStakeBindingsByUserAddress(ctx, userAddress)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	registers, err := pgdb.ListStakeIndexerRegisters(ctx)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}
	indexerNameMap := make(map[string]string, len(registers))
	for _, reg := range registers {
		name := reg.Name
		if name == "" {
			name = reg.IndexerID
		}
		indexerNameMap[reg.IndexerID] = name
	}

	pipe := rdb.RdbBalanceClient.Pipeline()
	defer pipe.Close()

	balanceCmds := make(map[string]*redis.StringCmd, len(bindings))
	rewardCmds := make(map[string]*redis.FloatCmd, len(bindings))
	deltaCmds := make(map[string]*redis.StringCmd, len(bindings))
	totalRewardsCmd := pipe.Get(ctx, constant.GetStakeRewardsTotalKey(userAddress))
	for _, b := range bindings {
		balanceCmds[b.StakeAddress] = pipe.Get(ctx, constant.GetRealtimeBalanceKey(b.StakeAddress))
		rewardCmds[b.StakeAddress] = pipe.ZScore(ctx, constant.GetStakeRewardsKey(userAddress), b.IndexerID)
		deltaCmds[b.StakeAddress] = pipe.HGet(ctx, constant.GetStakeMempoolBalanceDeltaKey(), b.StakeAddress)
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	itemMap := make(map[string]UserStakingItem, len(bindings))
	confirmedBindingKeys := make(map[string]struct{}, len(bindings))
	for _, b := range bindings {
		amount := uint64(0)
		if cmd := balanceCmds[b.StakeAddress]; cmd != nil {
			val, err := cmd.Result()
			if err == nil {
				amount, _ = strconv.ParseUint(val, 10, 64)
			}
		}

		pendingDelta := int64(0)
		if cmd := deltaCmds[b.StakeAddress]; cmd != nil {
			val, err := cmd.Result()
			if err == nil && strings.TrimSpace(val) != "" {
				pendingDelta, _ = strconv.ParseInt(strings.TrimSpace(val), 10, 64)
			}
		}

		finalAmount := int64(amount) + pendingDelta
		if finalAmount <= 0 {
			continue
		}

		rewards := uint64(0)
		if cmd := rewardCmds[b.StakeAddress]; cmd != nil {
			score, err := cmd.Result()
			if err == nil && score > 0 {
				rewards = uint64(math.Round(score))
			}
		}

		name := knownIndexerName(b.IndexerID, indexerNameMap)
		key := b.IndexerID + "|" + b.StakeAddress
		confirmedBindingKeys[key] = struct{}{}
		itemMap[key] = UserStakingItem{
			IndexerID:    b.IndexerID,
			Name:         name,
			StakeAddress: b.StakeAddress,
			Amount:       uint64(finalAmount),
			Rewards:      rewards,
			Pending:      pendingDelta != 0,
		}
	}

	pendingStakeEvents, err := listMempoolEventsAll(protocolparser.TagStake, userAddress, "", "")
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	pendingFirstBindStakeAddrByKey := make(map[string]string, len(pendingStakeEvents))
	for _, event := range pendingStakeEvents {
		if event.BizInvalidFlags != 0 {
			continue
		}
		indexerID := strings.TrimSpace(event.IndexerID)
		stakeAddress := strings.TrimSpace(event.StakeAddress)
		if indexerID == "" || stakeAddress == "" {
			continue
		}
		key := indexerID + "|" + stakeAddress
		if _, isConfirmedBinding := confirmedBindingKeys[key]; isConfirmedBinding {
			continue
		}
		pendingFirstBindStakeAddrByKey[key] = stakeAddress
	}

	pendingFirstBindDeltaCmds := make(map[string]*redis.StringCmd, len(pendingFirstBindStakeAddrByKey))
	if len(pendingFirstBindStakeAddrByKey) > 0 {
		pipe := rdb.RdbBalanceClient.Pipeline()
		defer pipe.Close()
		for key, stakeAddress := range pendingFirstBindStakeAddrByKey {
			pendingFirstBindDeltaCmds[key] = pipe.HGet(ctx, constant.GetStakeMempoolBalanceDeltaKey(), stakeAddress)
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			rData.Code = errorCodeInternal
			rData.Msg = err.Error()
			return rData, err
		}
	}

	for _, event := range pendingStakeEvents {
		if event.BizInvalidFlags != 0 {
			continue
		}
		indexerID := strings.TrimSpace(event.IndexerID)
		stakeAddress := strings.TrimSpace(event.StakeAddress)
		if indexerID == "" || stakeAddress == "" {
			continue
		}
		key := indexerID + "|" + stakeAddress
		item := itemMap[key]
		if _, isConfirmedBinding := confirmedBindingKeys[key]; isConfirmedBinding {
			if item.StakeAddress == "" {
				continue
			}
			item.Pending = true
			itemMap[key] = item
			continue
		}

		deltaCmd := pendingFirstBindDeltaCmds[key]
		if deltaCmd == nil {
			continue
		}
		rawDelta, deltaErr := deltaCmd.Result()
		if deltaErr != nil || strings.TrimSpace(rawDelta) == "" {
			continue
		}
		netDelta, parseErr := strconv.ParseInt(strings.TrimSpace(rawDelta), 10, 64)
		if parseErr != nil || netDelta <= 0 {
			continue
		}

		itemMap[key] = UserStakingItem{
			IndexerID:    indexerID,
			Name:         knownIndexerName(indexerID, indexerNameMap),
			StakeAddress: stakeAddress,
			Amount:       uint64(netDelta),
			Rewards:      0,
			Pending:      true,
		}
	}

	totalRewards := uint64(0)
	if val, err := totalRewardsCmd.Result(); err == nil {
		totalRewards, _ = strconv.ParseUint(val, 10, 64)
	}

	allItems := make([]UserStakingItem, 0, len(itemMap))
	for _, item := range itemMap {
		if item.Amount == 0 {
			continue
		}
		allItems = append(allItems, item)
	}

	sort.Slice(allItems, func(i, j int) bool {
		if allItems[i].Amount != allItems[j].Amount {
			return allItems[i].Amount > allItems[j].Amount
		}
		if allItems[i].IndexerID != allItems[j].IndexerID {
			return allItems[i].IndexerID < allItems[j].IndexerID
		}
		return allItems[i].StakeAddress < allItems[j].StakeAddress
	})

	rData.Data = ListUserStakingsResp{
		Total:        len(allItems),
		Start:        params.Start,
		TotalRewards: totalRewards,
		Detail:       paginateUserStakingItems(allItems, params.Start, params.Limit),
	}
	return rData, nil
}

func GetUserRewardRecords(c *gin.Context) (rData ResponseData, err error) {
	userAddress := strings.TrimSpace(c.Param("address"))
	if userAddress == "" {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "user address required"
		return rData, fmt.Errorf("user address required")
	}

	var params PageReqParams
	if err := c.ShouldBindQuery(&params); err != nil {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "params invalid"
		return rData, err
	}
	if err := validatePageParams(&params); err != nil {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = err.Error()
		return rData, err
	}

	total, err := pgdb.CountStakeAllocatedRewardsByUserAddress(ctx, userAddress)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}
	items, err := pgdb.ListStakeAllocatedRewardsByUserAddress(ctx, userAddress, params.Limit, params.Start)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	detail := make([]UserRewardRecordItem, 0, len(items))
	for _, item := range items {
		detail = append(detail, UserRewardRecordItem{
			UserAddress:          item.UserAddress,
			IndexerID:            item.IndexerID,
			StakeAddress:         item.StakeAddress,
			RewardType:           item.RewardType,
			Height:               item.Height,
			StakeAmountSnapshot:  item.StakeAmountSnapshot,
			StakeAmountEffective: item.StakeAmountEffective,
			TotalEffectiveStake:  item.TotalEffectiveStake,
			ReleasePercent:       item.ReleasePercent,
			BlockRewardAmount:    item.BlockRewardAmount,
			IndexerRatio:         item.IndexerRatio,
			AllocateAmount:       item.AllocateAmount,
		})
	}

	rData.Data = ListUserRewardRecordsResp{
		Total:  total,
		Start:  params.Start,
		Detail: detail,
	}
	return rData, nil
}

func GetIndexerStatus(c *gin.Context) (rData ResponseData, err error) {
	if cached, ok, cacheErr := loadIndexerStatusCache(); cacheErr != nil {
		rData.Code = errorCodeInternal
		rData.Msg = cacheErr.Error()
		return rData, cacheErr
	} else if ok {
		rData.Data = cached
		return rData, nil
	}

	registers, err := pgdb.ListStakeIndexerRegisters(ctx)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	totalStaked := uint64(0)
	if len(registers) > 0 {
		pipe := rdb.RdbBalanceClient.Pipeline()
		defer pipe.Close()

		totalCmds := make(map[string]*redis.StringCmd, len(registers))
		indexerIDs := make([]string, 0, len(registers))
		for _, reg := range registers {
			totalCmds[reg.IndexerID] = pipe.Get(ctx, constant.GetIndexerStakeTotalKey(reg.IndexerID))
			if reg.IndexerID != "" {
				indexerIDs = append(indexerIDs, reg.IndexerID)
			}
		}
		mempoolIndexerDeltas, deltaErr := loadMempoolIndexerDeltas(indexerIDs)
		if deltaErr != nil {
			rData.Code = errorCodeInternal
			rData.Msg = deltaErr.Error()
			return rData, deltaErr
		}
		if _, execErr := pipe.Exec(ctx); execErr != nil && execErr != redis.Nil {
			rData.Code = errorCodeInternal
			rData.Msg = execErr.Error()
			return rData, execErr
		}
		for _, reg := range registers {
			cmd := totalCmds[reg.IndexerID]
			if cmd == nil {
				continue
			}
			confirmedTotal := uint64(0)
			val, getErr := cmd.Result()
			if getErr == nil && val != "" {
				confirmedTotal, _ = strconv.ParseUint(val, 10, 64)
			}
			totalStaked += applyStakeDelta(confirmedTotal, mempoolIndexerDeltas[reg.IndexerID])
		}
	}

	latestBlockHeight, err := node.GetBlockCountRPC()
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	stakeRewardSyncHeight := uint32(0)
	if h, exists, syncErr := rewardsyncsvc.GetSnapshotConsumerHeight(); syncErr != nil {
		rData.Code = errorCodeInternal
		rData.Msg = syncErr.Error()
		return rData, syncErr
	} else if exists {
		stakeRewardSyncHeight = h
	}

	latestAllocatedRewardHeight := uint32(0)
	latestAllocatedRewardAmount := uint64(0)
	height, exists, latestErr := pgdb.GetLatestStakeAllocatedRewardHeight(ctx)
	if latestErr != nil {
		rData.Code = errorCodeInternal
		rData.Msg = latestErr.Error()
		return rData, latestErr
	}
	if exists {
		latestAllocatedRewardHeight = height
		syncBlock, blockErr := pgdb.GetSyncBlock(ctx, height)
		if blockErr != nil {
			rData.Code = errorCodeInternal
			rData.Msg = blockErr.Error()
			return rData, blockErr
		}
		if syncBlock != nil {
			latestAllocatedRewardAmount = syncBlock.CoinbaseReward
		}
	}

	rData.Data = IndexerStatusResp{
		TotalIndexers:               len(registers),
		TotalStaked:                 totalStaked,
		LatestBlockHeight:           latestBlockHeight,
		StakeRewardSyncHeight:       stakeRewardSyncHeight,
		LatestAllocatedRewardHeight: latestAllocatedRewardHeight,
		LatestAllocatedRewardAmount: latestAllocatedRewardAmount,
		PendingRewardSyncHeight:     0,
		PendingRewardTotalAmount:    0,
	}
	return rData, nil
}

func GetStakeRewardSyncStatus(c *gin.Context) (rData ResponseData, err error) {
	height, exists, err := rewardsyncsvc.GetSnapshotConsumerHeight()
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}
	if !exists {
		rData.Data = StakeRewardSyncStatusResp{Height: 0, BlockReward: 0, BlockHash: ""}
		return rData, nil
	}

	syncBlock, err := pgdb.GetSyncBlock(ctx, height)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}
	if syncBlock == nil {
		rData.Data = StakeRewardSyncStatusResp{Height: height, BlockReward: 0, BlockHash: ""}
		return rData, nil
	}

	rData.Data = StakeRewardSyncStatusResp{
		Height:      height,
		BlockReward: syncBlock.CoinbaseReward,
		BlockHash:   syncBlock.BlockHash,
	}
	return rData, nil
}

type MempoolProtocolQueryParams struct {
	PageReqParams
	Op            string `form:"op"`
	IndexerID     string `form:"indexer_id"`
	UserAddress   string `form:"user_address"`
	RewardAddress string `form:"reward_address"`
}

func isSupportedProtocolOp(op string) bool {
	switch op {
	case "", protocolparser.TagRegister, protocolparser.TagStake, protocolparser.TagProveStake, protocolparser.TagPledgedReward, protocolparser.TagAllocatRatio:
		return true
	default:
		return false
	}
}

func GetMempoolProtocolTxs(c *gin.Context) (rData ResponseData, err error) {
	var params MempoolProtocolQueryParams
	if err := c.ShouldBindQuery(&params); err != nil {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "params invalid"
		return rData, err
	}
	if err := validatePageParams(&params.PageReqParams); err != nil {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = err.Error()
		return rData, err
	}

	op := strings.TrimSpace(params.Op)
	indexerID := strings.TrimSpace(params.IndexerID)
	userAddress := strings.TrimSpace(params.UserAddress)
	rewardAddress := strings.TrimSpace(params.RewardAddress)
	if !isSupportedProtocolOp(op) {
		rData.Code = errorCodeParamsInvalid
		rData.Msg = "unsupported op"
		return rData, fmt.Errorf("unsupported op")
	}

	total, err := pgdb.CountStakeMempoolEvents(ctx, op, userAddress, rewardAddress, indexerID)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}
	items, err := pgdb.ListStakeMempoolEvents(ctx, op, userAddress, rewardAddress, indexerID, params.Limit, params.Start)
	if err != nil {
		rData.Code = errorCodeInternal
		rData.Msg = err.Error()
		return rData, err
	}

	detail := make([]MempoolProtocolTxItem, 0, len(items))
	for _, item := range items {
		detail = append(detail, MempoolProtocolTxItem{
			TxID:               item.TxID,
			Op:                 item.Op,
			Height:             item.Height,
			InscriptionContent: item.InscriptionContent,
			IndexerID:          item.IndexerID,
			UserAddress:        item.UserAddress,
			RewardAddress:      item.RewardAddress,
			StakeAddress:       item.StakeAddress,
			Amount:             item.Amount,
			IndexRatio:         item.IndexRatio,
			IndexerName:        item.IndexerName,
			ProveBlockHeight:   item.ProveBlockHeight,
			ProveDataHash:      item.ProveDataHash,
			TxIdx:              item.TxIdx,
		})
	}

	rData.Data = ListMempoolProtocolTxsResp{
		Total:  total,
		Start:  params.Start,
		Detail: detail,
	}
	return rData, nil
}

func parseInt64Field(raw interface{}) (int64, bool) {
	if raw == nil {
		return 0, false
	}
	text := strings.TrimSpace(fmt.Sprint(raw))
	if text == "" || strings.EqualFold(text, "<nil>") {
		return 0, false
	}
	val, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, false
	}
	return val, true
}

func loadIndexerStatusCache() (IndexerStatusResp, bool, error) {
	key := constant.GetStakeIndexerStatusKey()
	vals, err := rdb.RdbBalanceClient.HMGet(ctx, key,
		"total_indexers",
		"confirmed_total_staked",
		"mempool_total_staked_delta",
		"latest_block_height",
		"stake_reward_sync_height",
		"latest_allocated_reward_height",
		"latest_allocated_reward_amount",
		"pending_reward_sync_height",
		"pending_reward_total_amount",
	).Result()
	if err != nil && err != redis.Nil {
		return IndexerStatusResp{}, false, err
	}
	if len(vals) != 9 {
		return IndexerStatusResp{}, false, nil
	}

	totalIndexers, ok0 := parseInt64Field(vals[0])
	confirmedTotal, ok1 := parseInt64Field(vals[1])
	mempoolDelta, ok2 := parseInt64Field(vals[2])
	latestBlockHeight, ok3 := parseInt64Field(vals[3])
	stakeRewardSyncHeight, ok4 := parseInt64Field(vals[4])
	latestAllocatedRewardHeight, ok5 := parseInt64Field(vals[5])
	latestAllocatedRewardAmount, ok6 := parseInt64Field(vals[6])
	pendingRewardSyncHeight, ok7 := parseInt64Field(vals[7])
	pendingRewardTotalAmount, ok8 := parseInt64Field(vals[8])
	if !ok0 || !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 || !ok8 {
		return IndexerStatusResp{}, false, nil
	}
	if totalIndexers < 0 {
		totalIndexers = 0
	}
	if confirmedTotal < 0 {
		confirmedTotal = 0
	}
	if latestBlockHeight < 0 {
		latestBlockHeight = 0
	}
	if stakeRewardSyncHeight < 0 {
		stakeRewardSyncHeight = 0
	}
	if latestAllocatedRewardHeight < 0 {
		latestAllocatedRewardHeight = 0
	}
	if latestAllocatedRewardAmount < 0 {
		latestAllocatedRewardAmount = 0
	}
	if pendingRewardSyncHeight < 0 {
		pendingRewardSyncHeight = 0
	}
	if pendingRewardTotalAmount < 0 {
		pendingRewardTotalAmount = 0
	}

	resp := IndexerStatusResp{
		TotalIndexers:               int(totalIndexers),
		TotalStaked:                 applyStakeDelta(uint64(confirmedTotal), mempoolDelta),
		LatestBlockHeight:           uint32(latestBlockHeight),
		StakeRewardSyncHeight:       uint32(stakeRewardSyncHeight),
		LatestAllocatedRewardHeight: uint32(latestAllocatedRewardHeight),
		LatestAllocatedRewardAmount: uint64(latestAllocatedRewardAmount),
		PendingRewardSyncHeight:     uint32(pendingRewardSyncHeight),
		PendingRewardTotalAmount:    uint64(pendingRewardTotalAmount),
	}
	return resp, true, nil
}
