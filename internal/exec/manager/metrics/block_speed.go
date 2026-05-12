package metrics

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"stake_indexer/internal/component/log"
	"time"

	"go.uber.org/zap"
)

var (
	start            time.Time = time.Now()
	lastLogTime      time.Time
	lastBlockHeight  uint32
	lastBlockTxCount int
)

func byteCountBinary(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func ParseBlockSpeed(nTx, lenGlobalNewUtxoDataMap, lenGlobalSpentUtxoDataMap int, nextBlockHeight, maxBlockHeight uint32) {
	msg := "parsing block"
	if nTx == 0 {
		// parsing header
		msg = "parsing header"
	}
	lastBlockTxCount += nTx

	if nextBlockHeight != maxBlockHeight-1 && time.Since(lastLogTime) < time.Second {
		return
	}

	if nextBlockHeight < lastBlockHeight {
		lastBlockHeight = 0
	}

	lastLogTime = time.Now()

	timeLeft := uint32(0)
	if maxBlockHeight > 0 && (nextBlockHeight-lastBlockHeight) != 0 {
		timeLeft = (maxBlockHeight - nextBlockHeight) / (nextBlockHeight - lastBlockHeight)
	}

	var rtm runtime.MemStats
	runtime.ReadMemStats(&rtm)
	// free memory when large idle
	if rtm.HeapIdle-rtm.HeapReleased > 2*1024*1024*1024 {
		debug.FreeOSMemory()
	}

	logger.Log.Info(msg,
		zap.Uint32("height", nextBlockHeight),
		zap.Uint32("nblk", nextBlockHeight-lastBlockHeight),
		zap.Int("ntx", lastBlockTxCount),
		zap.Int("txo", lenGlobalSpentUtxoDataMap),
		zap.Int("utxo", lenGlobalNewUtxoDataMap),

		zap.String("mAlloc", byteCountBinary(rtm.HeapAlloc)),
		zap.String("mIdle", byteCountBinary(rtm.HeapIdle-rtm.HeapReleased)),

		zap.Uint32("time", timeLeft),
		zap.Int("elapse", int(time.Since(start).Seconds())),
	)

	lastBlockHeight = nextBlockHeight
	lastBlockTxCount = 0
}

