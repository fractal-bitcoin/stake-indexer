package indexer

import (
	"context"
	"stake_indexer/constant"
	"stake_indexer/internal/exec/manager/parallel"
	rollbackpkg "stake_indexer/internal/entry/rollback"
	"stake_indexer/internal/component/log"
	"stake_indexer/internal/component/redis"
	"stake_indexer/model"
	"sync"
	"sync/atomic"

	redis "github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

var ctx = context.Background()

// ParseBlockParallel parses tx and prepares UTXO maps.
func ParseBlockParallel(block *model.Block) {
	for txIdx, tx := range block.Txs {
		isCoinbase := txIdx == 0
		parallel.ParseTxFirst(tx, isCoinbase, block.ParseData)

		parallel.ParseUpdateTxoSpendByTxParallel(tx, isCoinbase, block.ParseData)
		parallel.ParseUpdateNewUtxoInTxParallel(uint32(txIdx), tx, block.ParseData)
	}
}

// ParseBlockSerialStart runs serial steps that depend on UTXO data.
func ParseBlockSerialStart(m *Manager, block *model.Block) {
	valid := parseGetSpentStakeUtxoData(block.ParseData, m.shouldTrackStakeAddress)
	if !valid {
		model.NeedStop = true
		return
	}
}

func ParseBlockSerialFinalize(m *Manager, block *model.Block) {
	updateStakeUtxoInMap(block.ParseData, m.shouldTrackStakeAddress)
	if err := rollbackpkg.PersistIndexerBlockArtifacts(block); err != nil {
		logger.Log.Error("persist indexer block artifacts failed", zap.Error(err), zap.Uint32("height", block.Height))
		panic(err)
	}
}

// ParseBlockParallelEnd runs final block/tx related writes.
func ParseBlockParallelEnd(block *model.Block) {
	block.Txs = nil
	block.ParseData = nil
}

func SubmitBlocks(m *Manager, startHeight, stageBlockHeight uint32) error {
	defer rollbackpkg.ResetIndexerArtifactStage()
	return runSubmitBlocksFlow(m, startHeight, stageBlockHeight)
}

func updateUtxoInPikaDel(utxoToRemove map[string]struct{}) bool {
	logger.Log.Info("UpdateUtxoInPikaDel",
		zap.Int("del", len(utxoToRemove)))

	if len(utxoToRemove) == 0 {
		return true
	}

	outpointKeys := make([]string, len(utxoToRemove))
	idx := 0
	for outpointKey := range utxoToRemove {
		outpointKeys[idx] = outpointKey
		idx++
	}

	sliceLen := 2500000
	for idx := 0; idx < (len(outpointKeys)-1)/sliceLen+1; idx++ {
		pipe := rdb.RdbUtxoClient.Pipeline()
		n := 0
		for _, outpointKey := range outpointKeys[idx*sliceLen:] {
			if n == sliceLen {
				break
			}
			pipe.Del(ctx, constant.GetUtxoKey(outpointKey))
			n++
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			logger.Log.Error("pika delete exec failed", zap.Error(err))
			pipe.Close()
			return false
		}
		pipe.Close()
	}

	logger.Log.Info("UpdateUtxoInPikaDel finished")
	return true
}

func updateUtxoInPikaAdd(utxoToRestore map[string]*model.TxoData) bool {
	logger.Log.Info("UpdateUtxoInPikaAdd",
		zap.Int("add", len(utxoToRestore)))

	type pair struct {
		Outpoint string
		Utxo     []byte
	}

	utxoBufToRestore := make([]*pair, len(utxoToRestore))
	idx := 0
	for outpointKey, data := range utxoToRestore {
		buf := data.MakeMarshalBuf()
		zbuf, _ := data.Marshal(buf)
		utxoBufToRestore[idx] = &pair{
			Outpoint: outpointKey,
			Utxo:     zbuf,
		}
		idx++
	}

	if len(utxoBufToRestore) == 0 {
		logger.Log.Info("UpdateUtxoInPikaAdd finished")
		return true
	}

	const numWorkers = 8
	txChan := make(chan *pair, 128)
	var wg sync.WaitGroup
	var hasExecErr atomic.Bool
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()

			maxSize := 10000000 // 10MB
			done := false
			for !done {
				done = true

				size := 0
				pikaPipe := rdb.RdbUtxoClient.Pipeline()
				for utxoPair := range txChan {
					pikaPipe.SetNX(ctx, constant.GetUtxoKey(utxoPair.Outpoint), utxoPair.Utxo, 0)
					size += 36 + len(utxoPair.Utxo)
					if size >= maxSize {
						done = false
						break
					}
				}
				if _, err := pikaPipe.Exec(ctx); err != nil && err != redis.Nil {
					logger.Log.Error("pika utxo exec failed", zap.Error(err))
					model.NeedStop = true
					hasExecErr.Store(true)
					pikaPipe.Close()
				}
				pikaPipe.Close()
			}
		}()
	}

	for _, tx := range utxoBufToRestore {
		txChan <- tx
	}
	close(txChan)
	wg.Wait()
	if hasExecErr.Load() {
		return false
	}

	logger.Log.Info("UpdateUtxoInPikaAdd finished")
	return true
}


