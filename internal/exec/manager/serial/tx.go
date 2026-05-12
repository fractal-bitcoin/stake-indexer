package serial

import (
	"context"
	"encoding/hex"
	"stake_indexer/constant"
	"stake_indexer/internal/component/log"
	"stake_indexer/internal/component/redis"
	"stake_indexer/model"
	"stake_indexer/utils"

	"github.com/btcsuite/btcd/chaincfg"
	redis "github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// ParseGetSpentUtxoDataFromRedisSerial loads spent UTXOs needed by current block.
func ParseGetSpentUtxoDataFromRedisSerial(block *model.ProcessBlock) (valid bool) {
	valid = true
	pipe := rdb.RdbUtxoClient.Pipeline()
	defer pipe.Close()

	m := map[string]*redis.StringCmd{}
	needExec := false
	ctx := context.Background()
	for outpointKey := range block.SpentUtxoKeysMap {
		if data, ok := block.NewUtxoDataMap[outpointKey]; ok {
			block.SpentUtxoDataMap[outpointKey] = data
			delete(block.NewUtxoDataMap, outpointKey)
			continue
		}

		if data, ok := model.GlobalNewUtxoDataMap[outpointKey]; ok {
			block.SpentUtxoDataMap[outpointKey] = data
			delete(model.GlobalNewUtxoDataMap, outpointKey)
			addAddressBalanceDelta(block, data, -int64(data.Satoshi))
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
			logger.Log.Error("parse block, but missing utxo from redis",
				zap.String("outpoint", hex.EncodeToString([]byte(outpointKey))))
			valid = false
			return valid
		} else if err != nil {
			panic(err)
		}

		d := &model.TxoData{}
		d.Unmarshal([]byte(res))

		block.SpentUtxoDataMap[outpointKey] = d
		model.GlobalSpentUtxoDataMap[outpointKey] = d
		model.GlobalDeleteUtxoKeysMap[outpointKey] = struct{}{}
		addAddressBalanceDelta(block, d, -int64(d.Satoshi))
	}

	return valid
}

// UpdateUtxoInMapSerial merges block new UTXOs into global maps.
func UpdateUtxoInMapSerial(block *model.ProcessBlock) {
	for outpointKey, data := range block.NewUtxoDataMap {
		model.GlobalNewUtxoDataMap[outpointKey] = data
		addAddressBalanceDelta(block, data, int64(data.Satoshi))
	}
}

func addAddressBalanceDelta(block *model.ProcessBlock, data *model.TxoData, delta int64) {
	if data == nil || delta == 0 {
		return
	}

	address, err := utils.GetAddressFromScript(data.PkScript, &chaincfg.MainNetParams)
	if err != nil || address == "" {
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
