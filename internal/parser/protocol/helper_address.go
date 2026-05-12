package protocolparser

import "stake_indexer/model"

func AddressFromTxoData(data *model.TxoData) string {
	if data == nil {
		return ""
	}
	return AddressFromPkScript(data.PkScript)
}
