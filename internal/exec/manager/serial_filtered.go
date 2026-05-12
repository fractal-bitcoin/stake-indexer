package indexer

import (
	"context"

	"stake_indexer/constant"
	"stake_indexer/internal/component/redis"
	"stake_indexer/model"
	"stake_indexer/utils"

	"github.com/btcsuite/btcd/chaincfg"
	redis "github.com/go-redis/redis/v8"
)

type trackStakeAddressFn func(address string) bool

func parseGetSpentStakeUtxoData(block *model.ProcessBlock, shouldTrack trackStakeAddressFn) (valid bool) {
	valid = true
	if block == nil {
		return valid
	}

	pipe := rdb.RdbUtxoClient.Pipeline()
	defer pipe.Close()

	m := map[string]*redis.StringCmd{}
	needExec := false
	ctx := context.Background()
	for outpointKey := range block.SpentUtxoKeysMap {
		if data, ok := block.NewUtxoDataMap[outpointKey]; ok {
			delete(block.NewUtxoDataMap, outpointKey)
			if !shouldTrackTxoData(data, shouldTrack) {
				continue
			}
			block.SpentUtxoDataMap[outpointKey] = data
			continue
		}

		if data, ok := model.GlobalNewUtxoDataMap[outpointKey]; ok {
			delete(model.GlobalNewUtxoDataMap, outpointKey)
			if !shouldTrackTxoData(data, shouldTrack) {
				continue
			}
			block.SpentUtxoDataMap[outpointKey] = data
			addTrackedAddressDelta(block, data, -int64(data.Satoshi), shouldTrack)
			continue
		}

		needExec = true
		m[outpointKey] = pipe.Get(ctx, constant.GetUtxoKey(outpointKey))
	}

	if !needExec {
		return valid
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		panic(err)
	}
	for outpointKey, v := range m {
		res, err := v.Result()
		if err == redis.Nil {
			// Stake indexer now stores stake-address UTXO only.
			continue
		} else if err != nil {
			panic(err)
		}

		d := &model.TxoData{}
		d.Unmarshal([]byte(res))
		if !shouldTrackTxoData(d, shouldTrack) {
			continue
		}

		block.SpentUtxoDataMap[outpointKey] = d
		model.GlobalSpentUtxoDataMap[outpointKey] = d
		model.GlobalDeleteUtxoKeysMap[outpointKey] = struct{}{}
		addTrackedAddressDelta(block, d, -int64(d.Satoshi), shouldTrack)
	}

	return valid
}

func updateStakeUtxoInMap(block *model.ProcessBlock, shouldTrack trackStakeAddressFn) {
	if block == nil {
		return
	}

	filteredNew := make(map[string]*model.TxoData, len(block.NewUtxoDataMap))
	for outpointKey, data := range block.NewUtxoDataMap {
		if !shouldTrackTxoData(data, shouldTrack) {
			continue
		}
		filteredNew[outpointKey] = data
		model.GlobalNewUtxoDataMap[outpointKey] = data
		addTrackedAddressDelta(block, data, int64(data.Satoshi), shouldTrack)
	}
	block.NewUtxoDataMap = filteredNew

	filteredSpent := make(map[string]*model.TxoData, len(block.SpentUtxoDataMap))
	for outpointKey, data := range block.SpentUtxoDataMap {
		if !shouldTrackTxoData(data, shouldTrack) {
			continue
		}
		filteredSpent[outpointKey] = data
	}
	block.SpentUtxoDataMap = filteredSpent
}

func shouldTrackTxoData(data *model.TxoData, shouldTrack trackStakeAddressFn) bool {
	if data == nil {
		return false
	}
	if shouldTrack == nil {
		return true
	}
	address, err := utils.GetAddressFromScript(data.PkScript, &chaincfg.MainNetParams)
	if err != nil || address == "" {
		return false
	}
	return shouldTrack(address)
}

func addTrackedAddressDelta(block *model.ProcessBlock, data *model.TxoData, delta int64, shouldTrack trackStakeAddressFn) {
	if data == nil || delta == 0 {
		return
	}

	address, err := utils.GetAddressFromScript(data.PkScript, &chaincfg.MainNetParams)
	if err != nil || address == "" {
		return
	}
	if shouldTrack != nil && !shouldTrack(address) {
		return
	}

	model.GlobalAddressBalanceDeltaMap[address] += delta
	if model.GlobalAddressBalanceDeltaMap[address] == 0 {
		delete(model.GlobalAddressBalanceDeltaMap, address)
	}
	if block != nil {
		block.AddressBalanceDeltaMap[address] += delta
		if block.AddressBalanceDeltaMap[address] == 0 {
			delete(block.AddressBalanceDeltaMap, address)
		}
	}
}
