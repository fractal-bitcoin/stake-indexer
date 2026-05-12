package parallel

import (
	"encoding/binary"
	scriptDecoder "stake_indexer/lib/script"
	"stake_indexer/model"
)

// ParseTxFirst initializes tx in/out metadata used by later pipeline stages.
func ParseTxFirst(tx *model.Tx, isCoinbase bool, block *model.ProcessBlock) {
	if tx == nil {
		return
	}

	for idx, input := range tx.TxIns {
		if input == nil {
			continue
		}
		key := make([]byte, 36)
		copy(key, tx.TxId)
		binary.LittleEndian.PutUint32(key[32:], uint32(idx))
		input.InputPoint = key
	}

	for idx, output := range tx.TxOuts {
		if output == nil {
			continue
		}
		key := make([]byte, 36)
		copy(key, tx.TxId)
		binary.LittleEndian.PutUint32(key[32:], uint32(idx))
		output.OutpointKey = string(key)
		output.Outpoint = key

		output.ScriptType = scriptDecoder.GetLockingScriptType(output.PkScript)
		if scriptDecoder.IsOpreturn(output.ScriptType) {
			output.LockingScriptUnspendable = true
		}
		output.AddressData = scriptDecoder.ExtractPkScriptForTxo(output.PkScript, output.ScriptType)
	}
}

// ParseUpdateTxoSpendByTxParallel records tx inputs that consume previous UTXOs.
func ParseUpdateTxoSpendByTxParallel(tx *model.Tx, isCoinbase bool, block *model.ProcessBlock) {
	if tx == nil || block == nil || isCoinbase {
		return
	}
	for _, input := range tx.TxIns {
		if input == nil || input.InputOutpointKey == "" {
			continue
		}
		block.SpentUtxoKeysMap[input.InputOutpointKey] = struct{}{}
	}
}

// ParseUpdateNewUtxoInTxParallel builds new UTXO entries from tx outputs.
func ParseUpdateNewUtxoInTxParallel(txIdx uint32, tx *model.Tx, block *model.ProcessBlock) {
	if tx == nil || block == nil {
		return
	}

	for _, output := range tx.TxOuts {
		if output == nil || output.OutpointKey == "" {
			continue
		}
		// Never index provably unspendable outputs (e.g. OP_RETURN), regardless of value.
		if output.LockingScriptUnspendable {
			continue
		}

		d := &model.TxoData{}
		d.BlockHeight = block.Height
		d.TxIdx = txIdx
		d.Satoshi = output.Satoshi
		d.PkScript = output.PkScript

		block.NewUtxoDataMap[output.OutpointKey] = d
	}
}
