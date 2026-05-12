package indexer

import (
	"strings"

	pgdb "stake_indexer/internal/component/pg"
	entryfast "stake_indexer/internal/entry/fast"
	protocolparser "stake_indexer/internal/parser/protocol"
)

const (
	BizInvalidNone            uint64 = entryfast.BizInvalidNone
	BizInvalidIndexerNotFound uint64 = entryfast.BizInvalidIndexerNotFound
	BizInvalidActorNotOwner   uint64 = entryfast.BizInvalidActorNotOwner
	BizInvalidRegisterRule    uint64 = entryfast.BizInvalidRegisterRule
	BizInvalidProofRule       uint64 = entryfast.BizInvalidProofRule
	BizInvalidStakeRule       uint64 = entryfast.BizInvalidStakeRule
	BizInvalidClaimRule       uint64 = entryfast.BizInvalidClaimRule
	BizInvalidUnknown         uint64 = entryfast.BizInvalidUnknown
)

type managerBusinessInvalidDeps struct {
	m *Manager
}

func (d managerBusinessInvalidDeps) ResolveRegisterInvalidFlags(payload *protocolparser.OpReturnPayload) uint64 {
	if d.m == nil || payload == nil {
		return BizInvalidUnknown
	}
	return d.m.resolveRegisterInvalidFlags(payload)
}

func (d managerBusinessInvalidDeps) ResolveOwnerAuthInvalidFlags(currentHeight uint32, payload *protocolparser.OpReturnPayload) uint64 {
	if d.m == nil || payload == nil {
		return BizInvalidUnknown
	}
	indexerID, err := d.m.normalizeIndexerIDAtHeight(payload.Get(protocolparser.OpFieldIndexerID), currentHeight)
	if err != nil || strings.TrimSpace(indexerID) == "" {
		return BizInvalidIndexerNotFound
	}
	userAddress, err := d.m.getIndexerUserAddressForAuth(indexerID)
	if err != nil || strings.TrimSpace(userAddress) == "" {
		return BizInvalidIndexerNotFound
	}
	actorAddress := strings.TrimSpace(payload.Get(protocolparser.OpFieldActorAddr))
	if actorAddress == "" || actorAddress != userAddress {
		return BizInvalidActorNotOwner
	}
	return BizInvalidNone
}

func (d managerBusinessInvalidDeps) BuildStakeProof(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (bool, error) {
	item, err := d.m.buildStakeProofAtHeight(currentHeight, currentHeight, tx, payload)
	return item != nil, err
}

func (d managerBusinessInvalidDeps) ValidateStakeTx(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (bool, error) {
	binding, ok, err := d.m.validateStakeTxWithPayload(currentHeight, tx, payload)
	if err != nil {
		return false, err
	}
	if !ok || binding == nil {
		return false, nil
	}
	return true, nil
}

func (d managerBusinessInvalidDeps) BuildStakeClaimedReward(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (bool, error) {
	item, err := d.m.buildStakeClaimedReward(currentHeight, tx, payload)
	return item != nil, err
}

func (m *Manager) resolveBusinessInvalidFlags(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) uint64 {
	if m == nil {
		return BizInvalidUnknown
	}
	return entryfast.ResolveBusinessInvalidFlags(managerBusinessInvalidDeps{m: m}, currentHeight, tx, payload)
}

func (m *Manager) resolveRegisterInvalidFlags(payload *protocolparser.OpReturnPayload) uint64 {
	if m == nil || payload == nil {
		return BizInvalidUnknown
	}

	userAddress := normalizeUserAddress(payload.Get(protocolparser.OpFieldActorAddr))
	if userAddress == "" {
		return BizInvalidRegisterRule
	}
	if m.registerOwnerSeen == nil {
		m.registerOwnerSeen = make(map[string]struct{})
	}
	if _, exists := m.registerOwnerSeen[userAddress]; exists {
		return BizInvalidRegisterRule
	}
	if m.hasStagedRegisterOwner(userAddress) {
		return BizInvalidRegisterRule
	}

	exists, err := pgdb.ExistsStakeIndexerRegisterByUserAddress(m.ctx, userAddress)
	if err != nil {
		return BizInvalidUnknown
	}
	if exists {
		return BizInvalidRegisterRule
	}

	m.registerOwnerSeen[userAddress] = struct{}{}
	return BizInvalidNone
}

func (m *Manager) hasStagedRegisterOwner(userAddress string) bool {
	if m == nil {
		return false
	}
	userAddress = normalizeUserAddress(userAddress)
	if userAddress == "" {
		return false
	}
	for i := len(m.WaitForUpsert.StakeIndexerRegisterList) - 1; i >= 0; i-- {
		if normalizeUserAddress(m.WaitForUpsert.StakeIndexerRegisterList[i].UserAddress) == userAddress {
			return true
		}
	}
	return false
}

func (m *Manager) resetRegisterOwnerSeen() {
	if m == nil {
		return
	}
	m.registerOwnerSeen = make(map[string]struct{})
}

func normalizeUserAddress(address string) string {
	return strings.TrimSpace(address)
}
