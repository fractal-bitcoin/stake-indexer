package rollback

import (
	"fmt"
	pgdb "stake_indexer/internal/component/pg"
)

func RollbackCheckWithFloor(lookback uint32, floorHeight uint32) (bool, uint32, error) {
	if lookback == 0 {
		lookback = 100
	}
	latestHeight, exists, err := pgdb.GetLatestCommittedSyncBlockHeight(ctx)
	if err != nil {
		return false, 0, err
	}
	if !exists {
		return false, 0, nil
	}
	start := uint32(0)
	if latestHeight+1 > lookback {
		start = latestHeight + 1 - lookback
	}
	if start < floorHeight {
		start = floorHeight
	}
	if start > latestHeight {
		return false, 0, nil
	}
	nodeInfos, ok := rpcGetBlockRange(start, latestHeight+1)
	if !ok {
		return false, 0, fmt.Errorf("load block index range from rpc failed")
	}
	dbBlocks, err := pgdb.ListSyncBlocksRange(ctx, start, latestHeight+1)
	if err != nil {
		return false, 0, err
	}
	nodeByHeight := make(map[uint32]string, len(nodeInfos))
	for _, info := range nodeInfos {
		if info == nil {
			continue
		}
		nodeByHeight[info.Height] = info.HashHex
	}
	dbByHeight := make(map[uint32]string, len(dbBlocks))
	for _, b := range dbBlocks {
		if b.State != "committed" {
			continue
		}
		dbByHeight[b.Height] = b.BlockHash
	}
	for h := start; h <= latestHeight; h++ {
		nodeHash, okNode := nodeByHeight[h]
		dbHash, okDB := dbByHeight[h]
		if !okNode || !okDB || nodeHash != dbHash {
			if err := RollbackIndexerFromHeight(h); err != nil {
				return false, 0, err
			}
			return true, h, nil
		}
	}
	return false, 0, nil
}
