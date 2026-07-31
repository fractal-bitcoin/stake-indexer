package indexer

import (
	"strings"

	stake "stake_indexer/internal/parser/protocol"
	"stake_indexer/model"
)

func (m *Manager) fillTxInputAddressesFromBlock(tx *stake.TxSnapshot, block *model.Block) {
	if tx == nil || block == nil || block.ParseData == nil || len(tx.Inputs) == 0 {
		return
	}
	for i := range tx.Inputs {
		outpointKey := tx.Inputs[i].OutpointKey
		if outpointKey == "" {
			continue
		}
		data := block.ParseData.SpentUtxoDataMap[outpointKey]
		if data == nil {
			continue
		}
		address := strings.TrimSpace(stake.AddressFromTxoData(data))
		if address == "" {
			continue
		}
		tx.Inputs[i].AddressKey = address
		tx.Inputs[i].Satoshi = data.Satoshi
	}
}

func (m *Manager) fillTxInputAddressesFromMempoolTx(tx *stake.TxSnapshot, rawTx *model.Tx) {
	if m == nil || tx == nil || rawTx == nil || len(tx.Inputs) == 0 || len(rawTx.TxIns) == 0 {
		return
	}
	for i := range tx.Inputs {
		idx := int(tx.Inputs[i].InputIdx)
		if idx < 0 || idx >= len(rawTx.TxIns) {
			continue
		}
		address, satoshi, ok := m.loadInputAddressAndSatoshi(rawTx.TxIns[idx])
		if !ok || strings.TrimSpace(address) == "" {
			continue
		}
		tx.Inputs[i].AddressKey = strings.TrimSpace(address)
		tx.Inputs[i].Satoshi = satoshi
	}
}

func (m *Manager) hasRewardClaimSenderInput(tx *stake.TxSnapshot) bool {
	if tx == nil || len(tx.Inputs) == 0 {
		return false
	}
	for i := range tx.Inputs {
		if m.isRewardClaimSenderAddress(tx.Inputs[i].AddressKey) {
			return true
		}
	}
	return false
}

func (m *Manager) isRewardClaimSenderAddress(address string) bool {
	if m == nil {
		return false
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return false
	}
	for _, configured := range m.cfg.RewardClaimSenderAddressKeys {
		if strings.EqualFold(address, strings.TrimSpace(configured)) {
			return true
		}
	}
	return false
}
