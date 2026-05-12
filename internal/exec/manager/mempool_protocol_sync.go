package indexer

import (
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"

	"stake_indexer/constant"
	logger "stake_indexer/internal/component/log"
	"stake_indexer/internal/component/node"
	pgdb "stake_indexer/internal/component/pg"
	rdb "stake_indexer/internal/component/redis"
	mempoolflow "stake_indexer/internal/entry/mempool"
	parser "stake_indexer/internal/parser/node"
	protocolparser "stake_indexer/internal/parser/protocol"
	"stake_indexer/model"
	"stake_indexer/utils"

	redis "github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

type MempoolProtocolSyncStats = mempoolflow.ProtocolSyncStats

func (m *Manager) SyncMempoolProtocolTxs(tipHeight uint32) (MempoolProtocolSyncStats, error) {
	if m == nil {
		return MempoolProtocolSyncStats{}, nil
	}
	m.resetRegisterOwnerSeen()
	return mempoolflow.RunProtocolSync(managerMempoolSyncDeps{m: m, tipHeight: tipHeight}, tipHeight)
}

func normalizeMempoolEntries(entries []string) ([]string, bool) {
	trimmed := make([]string, 0, len(entries))
	for _, item := range entries {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		trimmed = append(trimmed, item)
	}
	if len(trimmed) == 0 {
		return nil, false
	}

	txidMode := true
	sample := len(trimmed)
	if sample > 8 {
		sample = 8
	}
	for i := 0; i < sample; i++ {
		if !isLikelyTxIDHex(trimmed[i]) {
			txidMode = false
			break
		}
	}
	return trimmed, txidMode
}

func isLikelyTxIDHex(raw string) bool {
	raw = strings.TrimSpace(raw)
	if len(raw) != 64 {
		return false
	}
	_, err := hex.DecodeString(raw)
	return err == nil
}

func decodeRawMempoolTx(rawHex string) (*model.Tx, error) {
	rawHex = strings.TrimSpace(rawHex)
	if rawHex == "" {
		return nil, nil
	}

	raw, err := hex.DecodeString(rawHex)
	if err != nil {
		return nil, fmt.Errorf("decode raw mempool tx hex failed: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}

	tx, txOffset, err := parseRawTxSafe(raw)
	if err != nil {
		return nil, err
	}
	if int(txOffset) < len(raw) {
		if int(txOffset) == len(raw)-1 && raw[txOffset] == 0x01 {
			raw = raw[:txOffset]
		} else {
			return nil, fmt.Errorf("raw mempool tx length mismatch offset=%d len=%d", txOffset, len(raw))
		}
	}
	if len(raw) == 0 {
		return nil, nil
	}

	tx.Raw = raw
	if tx.WitOffset > 0 {
		tx.TxId = utils.GetWitnessHash256(tx.Raw, tx.WitOffset)
	} else {
		tx.TxId = utils.GetHash256(tx.Raw)
	}
	tx.TxIdHex = utils.HashString(tx.TxId)
	return tx, nil
}

func (m *Manager) parseMempoolProtocolTx(rawHex string, txIdx uint32) (*protocolparser.ParsedProtocolTx, error) {
	tx, err := decodeRawMempoolTx(rawHex)
	if err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, nil
	}
	return protocolparser.ParseProtocolTxFromModelTx(tx, txIdx, int64(constant.MEMPOOL_HEIGHT))
}

func (m *Manager) accumulateMempoolStakeBalanceDelta(rawHex string, deltas map[string]int64) error {
	if m == nil || len(m.stakeAddrToIndexer) == 0 {
		return nil
	}
	return m.accumulateMempoolStakeBalanceDeltaByAddresses(rawHex, deltas, m.stakeAddrToIndexer)
}

func (m *Manager) accumulateMempoolStakeBalanceDeltaByAddresses(rawHex string, deltas map[string]int64, tracked map[string]StakeAddressInfo) error {
	if m == nil || len(tracked) == 0 {
		return nil
	}

	tx, err := decodeRawMempoolTx(rawHex)
	if err != nil {
		return err
	}
	if tx == nil {
		return nil
	}

	for _, in := range tx.TxIns {
		address, satoshi, ok := m.loadInputAddressAndSatoshi(in)
		if !ok {
			continue
		}
		if _, ok := tracked[address]; !ok {
			continue
		}
		addPendingStakeDelta(deltas, address, -int64(satoshi))
	}

	for _, out := range tx.TxOuts {
		if out == nil {
			continue
		}
		address := strings.TrimSpace(protocolparser.AddressFromPkScript(out.PkScript))
		if address == "" {
			continue
		}
		if _, ok := tracked[address]; !ok {
			continue
		}
		addPendingStakeDelta(deltas, address, int64(out.Satoshi))
	}

	return nil
}

func addPendingStakeDelta(deltas map[string]int64, address string, delta int64) {
	if deltas == nil || address == "" || delta == 0 {
		return
	}
	deltas[address] += delta
	if deltas[address] == 0 {
		delete(deltas, address)
	}
}

func (m *Manager) buildMempoolIndexerDeltas(addressDeltas map[string]int64) (map[string]int64, map[string]map[string]int64) {
	indexerDeltas := make(map[string]int64, 32)
	stakerDeltas := make(map[string]map[string]int64, 32)
	if m == nil || len(addressDeltas) == 0 {
		return indexerDeltas, stakerDeltas
	}

	for stakeAddress, delta := range addressDeltas {
		stakeAddress = strings.TrimSpace(stakeAddress)
		if stakeAddress == "" || delta == 0 {
			continue
		}

		info, ok := m.stakeAddrToIndexer[stakeAddress]
		if !ok {
			continue
		}

		indexerID := strings.TrimSpace(info.IndexerID)
		userAddress := strings.TrimSpace(info.Address)
		if indexerID == "" || userAddress == "" {
			continue
		}

		indexerDeltas[indexerID] += delta
		if indexerDeltas[indexerID] == 0 {
			delete(indexerDeltas, indexerID)
		}

		userMap := stakerDeltas[indexerID]
		if userMap == nil {
			userMap = make(map[string]int64, 8)
			stakerDeltas[indexerID] = userMap
		}
		userMap[userAddress] += delta
		if userMap[userAddress] == 0 {
			delete(userMap, userAddress)
		}
		if len(userMap) == 0 {
			delete(stakerDeltas, indexerID)
		}
	}

	return indexerDeltas, stakerDeltas
}

func (m *Manager) saveMempoolStakeBalanceDeltas(deltas map[string]int64, events []pgdb.StakeMempoolEvent) error {
	if m == nil {
		return nil
	}

	balanceDeltaKey := constant.GetStakeMempoolBalanceDeltaKey()
	indexerDeltaKey := constant.GetStakeMempoolIndexerDeltaKey()
	indexerStakerIndexersKey := constant.GetStakeMempoolIndexerStakerDeltaIndexersKey()
	statusKey := constant.GetStakeIndexerStatusKey()

	existingIndexerIDs, err := rdb.RdbBalanceClient.SMembers(m.ctx, indexerStakerIndexersKey).Result()
	if err != nil {
		return err
	}

	indexerDeltas, stakerDeltas := m.buildMempoolIndexerDeltas(deltas)
	mergeMempoolFirstBindIndexerDeltas(m.stakeAddrToIndexer, deltas, indexerDeltas, events)

	pipe := rdb.RdbBalanceClient.Pipeline()
	defer pipe.Close()

	delKeys := make([]string, 0, len(existingIndexerIDs)+3)
	delKeys = append(delKeys, balanceDeltaKey, indexerDeltaKey, indexerStakerIndexersKey)
	for _, indexerID := range existingIndexerIDs {
		indexerID = strings.TrimSpace(indexerID)
		if indexerID == "" {
			continue
		}
		delKeys = append(delKeys, constant.GetStakeMempoolIndexerStakerDeltaKey(indexerID))
	}
	pipe.Del(m.ctx, delKeys...)

	if len(deltas) > 0 {
		fields := make(map[string]interface{}, len(deltas))
		for address, delta := range deltas {
			address = strings.TrimSpace(address)
			if address == "" || delta == 0 {
				continue
			}
			fields[address] = strconv.FormatInt(delta, 10)
		}
		if len(fields) > 0 {
			pipe.HSet(m.ctx, balanceDeltaKey, fields)
		}
	}

	mempoolTotalDelta := int64(0)
	if len(indexerDeltas) > 0 {
		fields := make(map[string]interface{}, len(indexerDeltas))
		for indexerID, delta := range indexerDeltas {
			indexerID = strings.TrimSpace(indexerID)
			if indexerID == "" || delta == 0 {
				continue
			}
			fields[indexerID] = strconv.FormatInt(delta, 10)
			mempoolTotalDelta += delta
		}
		if len(fields) > 0 {
			pipe.HSet(m.ctx, indexerDeltaKey, fields)
		}
	}
	pipe.HSet(m.ctx, statusKey, map[string]interface{}{
		"mempool_total_staked_delta": strconv.FormatInt(mempoolTotalDelta, 10),
	})

	if len(stakerDeltas) > 0 {
		indexerIDs := make([]interface{}, 0, len(stakerDeltas))
		for indexerID, userDeltas := range stakerDeltas {
			indexerID = strings.TrimSpace(indexerID)
			if indexerID == "" || len(userDeltas) == 0 {
				continue
			}
			fields := make(map[string]interface{}, len(userDeltas))
			for userAddress, delta := range userDeltas {
				userAddress = strings.TrimSpace(userAddress)
				if userAddress == "" || delta == 0 {
					continue
				}
				fields[userAddress] = strconv.FormatInt(delta, 10)
			}
			if len(fields) == 0 {
				continue
			}
			pipe.HSet(m.ctx, constant.GetStakeMempoolIndexerStakerDeltaKey(indexerID), fields)
			indexerIDs = append(indexerIDs, indexerID)
		}
		if len(indexerIDs) > 0 {
			pipe.SAdd(m.ctx, indexerStakerIndexersKey, indexerIDs...)
		}
	}

	if _, err := pipe.Exec(m.ctx); err != nil {
		return err
	}
	return nil
}

func mergeMempoolFirstBindIndexerDeltas(
	confirmedStakeAddrToIndexer map[string]StakeAddressInfo,
	addressDeltas map[string]int64,
	indexerDeltas map[string]int64,
	events []pgdb.StakeMempoolEvent,
) {
	if len(events) == 0 || indexerDeltas == nil {
		return
	}

	type firstBindStake struct {
		indexerID string
	}
	byStakeAddress := make(map[string]firstBindStake, 16)
	for _, event := range events {
		if event.Op != protocolparser.TagStake || event.BizInvalidFlags != 0 {
			continue
		}
		indexerID := strings.TrimSpace(event.IndexerID)
		stakeAddress := strings.TrimSpace(event.StakeAddress)
		if indexerID == "" || stakeAddress == "" {
			continue
		}
		if _, exists := confirmedStakeAddrToIndexer[stakeAddress]; exists {
			continue
		}
		item := byStakeAddress[stakeAddress]
		if item.indexerID == "" {
			item.indexerID = indexerID
		}
		if item.indexerID != indexerID {
			continue
		}
		byStakeAddress[stakeAddress] = item
	}

	for stakeAddress, item := range byStakeAddress {
		delta := addressDeltas[stakeAddress]
		if delta == 0 {
			continue
		}
		indexerDeltas[item.indexerID] += delta
		if indexerDeltas[item.indexerID] == 0 {
			delete(indexerDeltas, item.indexerID)
		}
	}
}

func (m *Manager) loadInputAddressAndSatoshi(in *model.TxIn) (string, uint64, bool) {
	if m == nil || in == nil || in.InputOutpointKey == "" {
		return "", 0, false
	}

	if cached, ok := m.mempoolOutpointCache[in.InputOutpointKey]; ok {
		address := strings.TrimSpace(cached.Address)
		if address != "" {
			return address, cached.Satoshi, true
		}
	}

	raw, err := rdb.RdbUtxoClient.Get(m.ctx, constant.GetUtxoKey(in.InputOutpointKey)).Bytes()
	if err == nil && len(raw) > 0 {
		txo := &model.TxoData{}
		if txo.Unmarshal(raw) {
			address := strings.TrimSpace(protocolparser.AddressFromTxoData(txo))
			if address != "" {
				m.mempoolOutpointCache[in.InputOutpointKey] = mempoolOutpointInfo{Address: address, Satoshi: txo.Satoshi}
				return address, txo.Satoshi, true
			}
		}
	}

	return m.loadInputAddressAndSatoshiFromMempool(in)
}

func (m *Manager) loadInputAddressAndSatoshiFromMempool(in *model.TxIn) (string, uint64, bool) {
	if m == nil || in == nil || in.InputOutpointKey == "" {
		return "", 0, false
	}

	parentTxID := strings.ToLower(strings.TrimSpace(in.InputHashHex))
	if parentTxID == "" {
		return "", 0, false
	}

	rawHex, err := node.GetRawTxHexRPC(parentTxID)
	if err != nil {
		logger.Log.Debug("load parent mempool tx raw by txid failed", zap.String("txid", parentTxID), zap.Error(err))
		return "", 0, false
	}
	parentTx, err := decodeRawMempoolTx(rawHex)
	if err != nil || parentTx == nil {
		return "", 0, false
	}

	outIdx := int(in.InputVout)
	if outIdx < 0 || outIdx >= len(parentTx.TxOuts) {
		return "", 0, false
	}
	out := parentTx.TxOuts[outIdx]
	if out == nil {
		return "", 0, false
	}

	address := strings.TrimSpace(protocolparser.AddressFromPkScript(out.PkScript))
	if address == "" {
		return "", 0, false
	}

	m.mempoolOutpointCache[in.InputOutpointKey] = mempoolOutpointInfo{Address: address, Satoshi: out.Satoshi}
	return address, out.Satoshi, true
}
func parseRawTxSafe(raw []byte) (tx *model.Tx, offset uint, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("decode raw tx panic: %v", r)
		}
	}()

	tx, offset = parser.NewTx(raw)
	if tx == nil || offset == 0 {
		return nil, 0, fmt.Errorf("decode raw tx failed")
	}
	return tx, offset, nil
}

func applyStakeDeltaUint(base uint64, delta int64) uint64 {
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

func buildMempoolFirstBindUserAmountsByIndexer(
	confirmedStakeAddrToIndexer map[string]StakeAddressInfo,
	addressDeltas map[string]int64,
	events []pgdb.StakeMempoolEvent,
) map[string]map[string]uint64 {
	result := make(map[string]map[string]uint64, 16)
	if len(events) == 0 {
		return result
	}

	type firstBindStake struct {
		indexerID   string
		userAddress string
		amount      uint64
	}
	byStakeAddress := make(map[string]firstBindStake, 16)
	for _, event := range events {
		if event.Op != protocolparser.TagStake || event.BizInvalidFlags != 0 {
			continue
		}
		indexerID := strings.TrimSpace(event.IndexerID)
		userAddress := strings.TrimSpace(event.UserAddress)
		stakeAddress := strings.TrimSpace(event.StakeAddress)
		if indexerID == "" || userAddress == "" || stakeAddress == "" {
			continue
		}
		if _, exists := confirmedStakeAddrToIndexer[stakeAddress]; exists {
			continue
		}

		item := byStakeAddress[stakeAddress]
		if item.indexerID == "" {
			item.indexerID = indexerID
			item.userAddress = userAddress
		}
		if item.indexerID != indexerID || item.userAddress != userAddress {
			continue
		}
		byStakeAddress[stakeAddress] = item
	}

	for stakeAddress, item := range byStakeAddress {
		if item.indexerID == "" || item.userAddress == "" {
			continue
		}
		delta := addressDeltas[stakeAddress]
		if delta <= 0 {
			continue
		}
		amount := uint64(delta)
		userMap := result[item.indexerID]
		if userMap == nil {
			userMap = make(map[string]uint64, 4)
			result[item.indexerID] = userMap
		}
		next := userMap[item.userAddress] + amount
		if next < userMap[item.userAddress] {
			userMap[item.userAddress] = ^uint64(0)
		} else {
			userMap[item.userAddress] = next
		}
	}
	return result
}

func (m *Manager) saveMempoolIndexerStakerSnapshots(addressDeltas map[string]int64, stakerDeltas map[string]map[string]int64, events []pgdb.StakeMempoolEvent) error {
	if m == nil {
		return nil
	}

	indexersKey := constant.GetStakeMempoolIndexerStakersIndexersKey()
	existingIndexerIDs, err := rdb.RdbBalanceClient.SMembers(m.ctx, indexersKey).Result()
	if err != nil {
		return err
	}

	pendingStakeByIndexer := buildMempoolFirstBindUserAmountsByIndexer(m.stakeAddrToIndexer, addressDeltas, events)

	activeIndexers := make(map[string]struct{}, len(stakerDeltas)+len(pendingStakeByIndexer))
	for indexerID := range stakerDeltas {
		indexerID = strings.TrimSpace(indexerID)
		if indexerID == "" {
			continue
		}
		activeIndexers[indexerID] = struct{}{}
	}
	for indexerID := range pendingStakeByIndexer {
		indexerID = strings.TrimSpace(indexerID)
		if indexerID == "" {
			continue
		}
		activeIndexers[indexerID] = struct{}{}
	}

	pipe := rdb.RdbBalanceClient.Pipeline()
	defer pipe.Close()

	delKeys := make([]string, 0, len(existingIndexerIDs)*2+1)
	delKeys = append(delKeys, indexersKey)
	for _, indexerID := range existingIndexerIDs {
		indexerID = strings.TrimSpace(indexerID)
		if indexerID == "" {
			continue
		}
		delKeys = append(delKeys,
			constant.GetStakeMempoolIndexerStakersKey(indexerID),
			constant.GetStakeMempoolIndexerStakersPendingKey(indexerID),
		)
	}
	pipe.Del(m.ctx, delKeys...)

	indexerMembers := make([]interface{}, 0, len(activeIndexers))
	for indexerID := range activeIndexers {
		confirmedItems, err := rdb.RdbBalanceClient.ZRevRangeWithScores(m.ctx, constant.GetIndexerStakeZsetKey(indexerID), 0, -1).Result()
		if err != nil && err != redis.Nil {
			return err
		}

		merged := make(map[string]uint64, len(confirmedItems)+8)
		for _, item := range confirmedItems {
			userAddress, ok := item.Member.(string)
			userAddress = strings.TrimSpace(userAddress)
			if !ok || userAddress == "" {
				continue
			}
			amount := uint64(math.Round(item.Score))
			if amount == 0 {
				continue
			}
			merged[userAddress] = amount
		}

		pendingUsers := make(map[string]struct{}, 8)
		for userAddress, delta := range stakerDeltas[indexerID] {
			userAddress = strings.TrimSpace(userAddress)
			if userAddress == "" || delta == 0 {
				continue
			}
			finalAmount := applyStakeDeltaUint(merged[userAddress], delta)
			if finalAmount == 0 {
				delete(merged, userAddress)
				continue
			}
			merged[userAddress] = finalAmount
			pendingUsers[userAddress] = struct{}{}
		}

		for userAddress, amount := range pendingStakeByIndexer[indexerID] {
			userAddress = strings.TrimSpace(userAddress)
			if userAddress == "" || amount == 0 {
				continue
			}
			if _, exists := merged[userAddress]; !exists {
				merged[userAddress] = amount
			}
			pendingUsers[userAddress] = struct{}{}
		}

		if len(merged) > 0 {
			zItems := make([]*redis.Z, 0, len(merged))
			for userAddress, amount := range merged {
				if userAddress == "" || amount == 0 {
					continue
				}
				zItems = append(zItems, &redis.Z{Score: float64(amount), Member: userAddress})
			}
			if len(zItems) > 0 {
				pipe.ZAdd(m.ctx, constant.GetStakeMempoolIndexerStakersKey(indexerID), zItems...)
			}
		}

		if len(pendingUsers) > 0 {
			members := make([]interface{}, 0, len(pendingUsers))
			for userAddress := range pendingUsers {
				members = append(members, userAddress)
			}
			pipe.SAdd(m.ctx, constant.GetStakeMempoolIndexerStakersPendingKey(indexerID), members...)
		}

		indexerMembers = append(indexerMembers, indexerID)
	}

	if len(indexerMembers) > 0 {
		pipe.SAdd(m.ctx, indexersKey, indexerMembers...)
	}

	if _, err := pipe.Exec(m.ctx); err != nil {
		return err
	}
	return nil
}
