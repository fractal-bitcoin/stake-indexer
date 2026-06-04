package indexer

import (
	"fmt"
	"strconv"
	"strings"

	pgdb "stake_indexer/internal/component/pg"
	entryfast "stake_indexer/internal/entry/fast"
	stake "stake_indexer/internal/parser/protocol"
	"stake_indexer/model"
)

type managerFastBlockDeps struct {
	m *Manager
}

func (d managerFastBlockDeps) ResolveBusinessInvalidFlags(currentHeight uint32, tx *stake.TxSnapshot, payload *stake.OpReturnPayload) uint64 {
	return d.m.resolveFastBlockBusinessInvalidFlags(currentHeight, tx, payload)
}

func (d managerFastBlockDeps) HandleRegisterTx(height uint32, tx *stake.TxSnapshot, payload *stake.OpReturnPayload) error {
	return d.m.handleRegisterTx(height, tx, payload)
}

func (d managerFastBlockDeps) HandleProofTx(height uint32, tx *stake.TxSnapshot, payload *stake.OpReturnPayload) error {
	return d.m.handleProofTx(height, tx, payload)
}

func (d managerFastBlockDeps) HandleClaimedRewardTx(height uint32, tx *stake.TxSnapshot, payload *stake.OpReturnPayload) error {
	return d.m.handleClaimedRewardTx(height, tx, payload)
}

func (d managerFastBlockDeps) HandleStakeBindTxWithPayload(height uint32, tx *stake.TxSnapshot, payload *stake.OpReturnPayload) error {
	return d.m.handleStakeBindTxWithPayload(height, tx, payload)
}

func (d managerFastBlockDeps) HandleUpdateRatioTxLatest(height uint32, tx *stake.TxSnapshot, payload *stake.OpReturnPayload) error {
	return d.m.handleUpdateRatioTxLatest(height, tx, payload)
}

func (d managerFastBlockDeps) AppendFIP101InscriptionEvent(event pgdb.FIP101InscriptionEvent) {
	d.m.WaitForUpsert.FIP101InscriptionEvents = append(d.m.WaitForUpsert.FIP101InscriptionEvents, event)
}

func (m *Manager) HandleFastBlock(block *model.Block) error {
	if m == nil {
		return nil
	}
	m.resetRegisterOwnerSeen()
	return entryfast.HandleBlockByDeps(managerFastBlockDeps{m: m}, block)
}

type stakeTxBinding struct {
	IndexerID    string
	UserAddress  string
	AddressType  string
	StakeAddress string
	Amount       uint64
}

func (m *Manager) validateStakeTxWithPayload(height uint32, tx *stake.TxSnapshot, payload *stake.OpReturnPayload) (*stakeTxBinding, bool, error) {
	if tx == nil || payload == nil {
		return nil, false, nil
	}

	indexerID := payload.Get(stake.OpFieldIndexerID)
	var err error
	indexerID, err = m.normalizeIndexerIDAtHeight(indexerID, height)
	if err != nil {
		return nil, false, nil
	}
	if indexerID == "" {
		return nil, false, nil
	}
	registered, err := m.isIndexerRegistered(indexerID)
	if err != nil {
		return nil, false, err
	}
	if !registered {
		return nil, false, nil
	}

	pubKeyHex := strings.TrimSpace(payload.Get(stake.OpFieldActorPubKey, stake.OpFieldPubKey))
	if pubKeyHex == "" {
		return nil, false, nil
	}
	addressTypeRaw := strings.TrimSpace(payload.Get(stake.OpFieldAddressType))
	if addressTypeRaw == "" {
		return nil, false, nil
	}
	addressTypeCode, ok := stake.ParseAddressTypeCode(addressTypeRaw)
	if !ok {
		return nil, false, nil
	}
	addressType := strconv.FormatUint(addressTypeCode, 10)

	userAddress, err := stake.DeriveAddressFromPubKeyAndType(pubKeyHex, addressType, nil)
	if err != nil {
		return nil, false, nil
	}
	if userAddress == "" {
		return nil, false, nil
	}

	stakeOut, ok := firstNonOpReturnOutput(tx)
	if !ok || stakeOut.AddressKey == "" || stakeOut.Satoshi == 0 {
		return nil, false, nil
	}

	derivedStakeAddress, err := stake.DeriveStakeAddress(indexerID, pubKeyHex, addressType, nil)
	if err != nil {
		return nil, false, fmt.Errorf("derive stake address failed: %w", err)
	}
	if !strings.EqualFold(derivedStakeAddress, stakeOut.AddressKey) {
		return nil, false, nil
	}

	return &stakeTxBinding{
		IndexerID:    indexerID,
		UserAddress:  userAddress,
		AddressType:  addressType,
		StakeAddress: stakeOut.AddressKey,
		Amount:       stakeOut.Satoshi,
	}, true, nil
}

func (m *Manager) handleStakeBindTxWithPayload(height uint32, tx *stake.TxSnapshot, payload *stake.OpReturnPayload) error {
	binding, ok, err := m.validateStakeTxWithPayload(height, tx, payload)
	if err != nil || !ok {
		return err
	}

	m.WaitForUpsert.StakeBindingList = append(m.WaitForUpsert.StakeBindingList, pgdb.StakeBinding{
		UserAddress:  binding.UserAddress,
		IndexerID:    binding.IndexerID,
		AddressType:  binding.AddressType,
		StakeAddress: binding.StakeAddress,
		Height:       height,
		StakeTxID:    tx.TxID,
		StakeTxIdx:   tx.TxIdx,
	})
	m.cacheStakeBinding(binding.UserAddress, binding.IndexerID, binding.AddressType, binding.StakeAddress)

	return nil
}

func firstSpendableOutput(tx *stake.TxSnapshot) (*stake.OutputSnapshot, bool) {
	for i := range tx.Outputs {
		if tx.Outputs[i].AddressKey != "" {
			return &tx.Outputs[i], true
		}
	}
	return nil, false
}

func firstNonOpReturnOutput(tx *stake.TxSnapshot) (*stake.OutputSnapshot, bool) {
	if tx == nil {
		return nil, false
	}
	for i := range tx.Outputs {
		out := &tx.Outputs[i]
		if len(out.PkScript) > 0 && out.PkScript[0] == 0x6a {
			continue
		}
		if out.AddressKey == "" {
			continue
		}
		return out, true
	}
	return nil, false
}
