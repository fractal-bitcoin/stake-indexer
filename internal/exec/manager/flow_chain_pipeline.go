package indexer

import (
	"stake_indexer/internal/component/log"
	"stake_indexer/internal/component/node"
	progressTask "stake_indexer/internal/exec/manager/metrics"
	"stake_indexer/internal/parser/node"
	"stake_indexer/model"
	"sync"

	"go.uber.org/zap"
)

func (m *Manager) ParseLongestChain(startBlockHeight, endBlockHeight uint32) (lastBlockId []byte, lastHeight uint32, txCount int) {
	blocksReady := make(chan *model.Block, 64)
	blocksDone := make(chan struct{}, 64)
	blocksStage := make(chan *model.Block, 64)

	go m.InitLongestChainBlockByHeader(blocksDone, blocksReady, startBlockHeight, endBlockHeight)
	go m.ParseLongestChainBlockStart(blocksDone, blocksReady, blocksStage, startBlockHeight, endBlockHeight)

	return m.ParseLongestChainBlockEnd(blocksStage)
}

func (m *Manager) InitLongestChainBlockByHeader(blocksDone chan struct{}, blocksReady chan *model.Block, startBlockHeight, endBlockHeight uint32) {
	var wg sync.WaitGroup

	blocksTotal := m.MainChainHeight + 1
	if endBlockHeight == 0 {
		endBlockHeight = blocksTotal
	}

	var txCount int
	for nextBlockHeight := startBlockHeight; nextBlockHeight < endBlockHeight; nextBlockHeight++ {
		if model.NeedStop {
			break
		}

		blockInfo, ok := m.BlocksOfChainByHeight[nextBlockHeight]
		if !ok {
			break
		}

		txCount += int(blockInfo.TxCnt)

		rawblock, err := node.GetRawBlock(blockInfo.HashHex)
		if err != nil {
			logger.Log.Error("get block error", zap.Error(err), zap.Uint32("height", nextBlockHeight), zap.String("hash", blockInfo.HashHex))
			break
		}
		if len(rawblock) < 80+9 {
			continue
		}

		block := &model.Block{Height: blockInfo.Height}
		parser.InitBlock(block, rawblock)
		if blockInfo.HashHex != block.HashHex {
			logger.Log.Info("block hash mismatch",
				zap.Uint32("height", nextBlockHeight),
				zap.String("expected", blockInfo.HashHex),
				zap.String("actual", block.HashHex))
			break
		}

		blocksDone <- struct{}{}
		wg.Add(1)
		go func(block *model.Block) {
			defer wg.Done()

			if txs, ok := parser.NewTxs(block.Raw[80:], block.Height); ok {
				block.Txs = txs
			} else {
				logger.Log.Fatal("block tx decode invalid", zap.Uint32("height", block.Height), zap.String("blkHash", block.HashHex))
				return
			}

			processBlock := &model.ProcessBlock{
				Height:                 block.Height,
				NewUtxoDataMap:         make(map[string]*model.TxoData, block.TxCnt),
				SpentUtxoDataMap:       make(map[string]*model.TxoData, block.TxCnt),
				SpentUtxoKeysMap:       make(map[string]struct{}, block.TxCnt),
				AddressBalanceDeltaMap: make(map[string]int64, block.TxCnt),
			}
			block.ParseData = processBlock
			ParseBlockParallel(block)

			block.Raw = nil
			blocksReady <- block
		}(block)
	}
	wg.Wait()

	close(blocksReady)
	logger.Log.Info("produce ok")
}

func (m *Manager) ParseLongestChainBlockStart(blocksDone chan struct{}, blocksReady, blocksStage chan *model.Block, startBlockHeight, maxBlockHeight uint32) {
	defer close(blocksStage)

	if maxBlockHeight == 0 || maxBlockHeight > m.MainChainHeight+1 {
		maxBlockHeight = m.MainChainHeight + 1
	}

	nextBlockHeight := startBlockHeight
	blockParseBufferBlock := make([]*model.Block, maxBlockHeight-startBlockHeight)
	for block := range blocksReady {
		if block.Height < maxBlockHeight {
			blockParseBufferBlock[block.Height-startBlockHeight] = block
		}

		if block.Height != nextBlockHeight {
			continue
		}
		for nextBlockHeight < maxBlockHeight {
			block = blockParseBufferBlock[nextBlockHeight-startBlockHeight]
			if block == nil {
				break
			}

			<-blocksDone
			ParseBlockSerialStart(m, block)
			if model.NeedStop {
				return
			}

			if err := m.HandleFastBlock(block); err != nil {
				logger.Log.Error("handle fast block failed", zap.Error(err), zap.Uint32("height", block.Height))
				model.NeedStop = true
				return
			}
			if model.NeedStop {
				return
			}
			ParseBlockSerialFinalize(m, block)

			progressTask.ParseBlockSpeed(len(block.Txs), len(model.GlobalNewUtxoDataMap), len(model.GlobalSpentUtxoDataMap), block.Height, maxBlockHeight)
			blocksStage <- block
			nextBlockHeight++
		}
		if nextBlockHeight >= maxBlockHeight {
			break
		}
	}
}

func (m *Manager) ParseLongestChainBlockEnd(blocksStage chan *model.Block) (lastBlockId []byte, lastHeight uint32, txCount int) {
	var wg sync.WaitGroup
	blocksLimit := make(chan struct{}, 64)
	for block := range blocksStage {
		lastBlockId = block.Hash
		lastHeight = block.Height
		txCount += int(block.TxCnt)
		blocksLimit <- struct{}{}
		wg.Add(1)
		go func(block *model.Block) {
			defer wg.Done()
			ParseBlockParallelEnd(block)
			<-blocksLimit
		}(block)
	}
	wg.Wait()
	logger.Log.Info("consume ok")
	return lastBlockId, lastHeight, txCount
}

func (m *Manager) InitLatestBlockFromRPC(startHeight, batch uint32) bool {
	m.BlocksOfChainByHeight = make(map[uint32]*model.BlockIndexInfo, 0)
	endHeight := startHeight + 1 + batch
	blockInfos, ok := node.GetBlockIndexRangeRPC(startHeight, endHeight)
	if !ok {
		return false
	}

	if len(blockInfos) == 0 {
		if startHeight > 0 {
			m.MainChainHeight = startHeight - 1
		} else {
			m.MainChainHeight = 0
		}
		return true
	}

	for _, blk := range blockInfos {
		block := &model.BlockIndexInfo{
			Height:     blk.Height,
			HashHex:    blk.HashHex,
			TxCnt:      blk.TxCnt,
			FileIdx:    blk.FileIdx,
			FileOffset: blk.FileOffset,
		}
		m.BlocksOfChainByHeight[blk.Height] = block
		m.MainChainHeight = blk.Height
	}
	return true
}
