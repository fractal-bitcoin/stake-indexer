package model

import (
	"runtime"
)

var (
	GlobalNewUtxoDataMap    map[string]*TxoData
	GlobalSpentUtxoDataMap  map[string]*TxoData
	GlobalDeleteUtxoKeysMap map[string]struct{}
	// key: address string, value: pending balance delta in current batch
	GlobalAddressBalanceDeltaMap map[string]int64
)

func init() {
	CleanUtxoMap()
}

// CleanUtxoMap clears local map memory.
func CleanUtxoMap() {
	GlobalNewUtxoDataMap = nil
	GlobalSpentUtxoDataMap = nil
	GlobalDeleteUtxoKeysMap = nil
	GlobalAddressBalanceDeltaMap = nil
	runtime.GC()

	GlobalNewUtxoDataMap = make(map[string]*TxoData, 0)
	GlobalSpentUtxoDataMap = make(map[string]*TxoData, 0)
	GlobalDeleteUtxoKeysMap = make(map[string]struct{}, 0)
	GlobalAddressBalanceDeltaMap = make(map[string]int64, 0)

}
