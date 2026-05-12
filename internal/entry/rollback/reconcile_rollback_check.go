package rollback

import (
	"context"
	"fmt"
	"stake_indexer/constant"
	rdb "stake_indexer/internal/component/redis"
	"stake_indexer/model"
	"strconv"

	"stake_indexer/internal/component/node"

	redis "github.com/go-redis/redis/v8"
)

var ctx = context.Background()

type StagedIndexerBlock struct {
	Height         uint32
	HashHex        string
	ParentHash     string
	Version        uint32
	CoinbaseReward uint64
	AddressDeltas  map[string]int64
	UndoNew        map[string][]byte
	UndoSpent      map[string][]byte
}

var stagedIndexerBlocks []*StagedIndexerBlock

func ResetIndexerArtifactStage() {
	stagedIndexerBlocks = stagedIndexerBlocks[:0]
}

func PersistIndexerBlockArtifacts(block *model.Block) error {
	if block == nil {
		return nil
	}

	var coinbaseReward uint64
	for _, output := range block.Txs[0].TxOuts {
		coinbaseReward += output.Satoshi
	}

	staged := &StagedIndexerBlock{
		Height:         block.Height,
		HashHex:        block.HashHex,
		ParentHash:     block.ParentHex,
		Version:        block.Version,
		CoinbaseReward: coinbaseReward,
		AddressDeltas:  make(map[string]int64),
		UndoNew:        make(map[string][]byte),
		UndoSpent:      make(map[string][]byte),
	}
	if block.ParseData != nil {
		for raw, delta := range block.ParseData.AddressBalanceDeltaMap {
			if delta == 0 {
				continue
			}
			staged.AddressDeltas[raw] = delta
		}
		staged.UndoNew = encodeUndoMap(block.ParseData.NewUtxoDataMap)
		staged.UndoSpent = encodeUndoMap(block.ParseData.SpentUtxoDataMap)
	}

	stagedIndexerBlocks = append(stagedIndexerBlocks, staged)
	return nil
}

func BuildStagedBlockMapForRange(startHeight, endHeight uint32) (map[uint32]*StagedIndexerBlock, error) {
	rangeLen := int(endHeight-startHeight) + 1
	stagedByHeight := make(map[uint32]*StagedIndexerBlock, rangeLen)
	for _, item := range stagedIndexerBlocks {
		if item == nil {
			continue
		}
		if item.Height < startHeight || item.Height > endHeight {
			continue
		}
		stagedByHeight[item.Height] = item
	}

	for h := startHeight; ; h++ {
		if _, ok := stagedByHeight[h]; !ok {
			return nil, fmt.Errorf("missing staged indexer artifact at height %d", h)
		}
		if h == endHeight {
			break
		}
	}
	return stagedByHeight, nil
}

func GetIndexerUtxoHeight() (uint32, bool, error) {
	res, err := rdb.RdbUtxoClient.HGet(ctx, constant.TASK_INFO_KEYNAME, constant.TASK_UTXO).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	v, err := strconv.ParseUint(res, 10, 32)
	if err != nil {
		return 0, false, err
	}
	return uint32(v), true, nil
}

func GetInfoFieldUInt32(field string) (uint32, bool, error) {
	res, err := rdb.RdbBalanceClient.HGet(ctx, constant.TASK_INFO_KEYNAME, field).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	v, err := strconv.ParseUint(res, 10, 32)
	if err != nil {
		return 0, false, err
	}
	return uint32(v), true, nil
}

func encodeUndoMap(m map[string]*model.TxoData) map[string][]byte {
	result := make(map[string][]byte, len(m))
	for outpoint, d := range m {
		if d == nil {
			continue
		}
		buf := d.MakeMarshalBuf()
		zbuf, _ := d.Marshal(buf)
		copied := make([]byte, len(zbuf))
		copy(copied, zbuf)
		result[outpoint] = copied
	}
	return result
}

func rpcGetBlockRange(startHeight, endHeight uint32) ([]*model.BlockIndexInfo, bool) {
	infos, ok := node.GetBlockIndexRangeRPC(startHeight, endHeight)
	if !ok {
		return nil, false
	}
	result := make([]*model.BlockIndexInfo, 0, len(infos))
	for _, info := range infos {
		if info == nil {
			continue
		}
		result = append(result, &model.BlockIndexInfo{
			Height:     info.Height,
			HashHex:    info.HashHex,
			TxCnt:      info.TxCnt,
			FileIdx:    info.FileIdx,
			FileOffset: info.FileOffset,
		})
	}
	return result, true
}
