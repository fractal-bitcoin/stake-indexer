package indexer

import (
	"strings"

	"stake_indexer/conf"
	"stake_indexer/internal/component/log"
	pgdb "stake_indexer/internal/component/pg"
	entryfast "stake_indexer/internal/entry/fast"
	protocolparser "stake_indexer/internal/parser/protocol"

	"go.uber.org/zap"
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
	m                           *Manager
	enforceRegisterIndexerAllow bool
}

func (d managerBusinessInvalidDeps) ResolveRegisterInvalidFlags(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) uint64 {
	if d.m == nil || payload == nil {
		return BizInvalidUnknown
	}
	return d.m.resolveRegisterInvalidFlags(currentHeight, tx, payload, d.enforceRegisterIndexerAllow)
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
	return m.resolveBusinessInvalidFlagsWithOptions(currentHeight, tx, payload, false)
}

func (m *Manager) resolveBusinessInvalidFlagsWithRegisterAllowlist(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) uint64 {
	return m.resolveBusinessInvalidFlagsWithOptions(currentHeight, tx, payload, true)
}

func (m *Manager) resolveBusinessInvalidFlagsWithOptions(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload, enforceRegisterIndexerAllow bool) uint64 {
	if m == nil {
		return BizInvalidUnknown
	}
	flags := entryfast.ResolveBusinessInvalidFlags(managerBusinessInvalidDeps{
		m:                           m,
		enforceRegisterIndexerAllow: enforceRegisterIndexerAllow,
	}, currentHeight, tx, payload)
	m.logRegisterAnalysisResult(currentHeight, tx, payload, flags)
	return flags
}

func (m *Manager) logRegisterAnalysisResult(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload, flags uint64) {
	if payload == nil || payload.Tag != protocolparser.TagRegister {
		return
	}

	fields := []zap.Field{
		zap.Bool("valid", flags == BizInvalidNone),
		zap.Uint64("biz_invalid_flags", flags),
		zap.Strings("reasons", bizInvalidReasonNames(flags)),
		zap.Uint32("height", currentHeight),
		zap.String("user_address", normalizeUserAddress(payload.Get(protocolparser.OpFieldActorAddr))),
		zap.String("reward_address", strings.TrimSpace(payload.Get(protocolparser.OpFieldRewardAddr))),
		zap.String("index_ratio", strings.TrimSpace(payload.Get(protocolparser.OpFieldIndexRatio))),
		zap.String("indexer_name", strings.TrimSpace(payload.Get(protocolparser.OpFieldIndexerName))),
	}
	if tx != nil {
		indexerID := protocolparser.BuildIndexerID(currentHeight, tx.TxIdx)
		fields = append(fields,
			zap.String("indexer_id", indexerID),
			zap.Bool("indexer_allowed", conf.StakeRewardCfg.IsIndexerAllowedAtHeight(indexerID, currentHeight)),
			zap.String("txid", tx.TxID),
			zap.Uint32("txidx", tx.TxIdx),
		)
	}

	logger.Log.Info("register_indexer analysis result", fields...)
}

func bizInvalidReasonNames(flags uint64) []string {
	if flags == BizInvalidNone {
		return []string{"none"}
	}

	reasons := make([]string, 0, 6)
	if flags&BizInvalidIndexerNotFound != 0 {
		reasons = append(reasons, "indexer_not_found")
	}
	if flags&BizInvalidActorNotOwner != 0 {
		reasons = append(reasons, "actor_not_owner")
	}
	if flags&BizInvalidRegisterRule != 0 {
		reasons = append(reasons, "register_rule")
	}
	if flags&BizInvalidProofRule != 0 {
		reasons = append(reasons, "proof_rule")
	}
	if flags&BizInvalidStakeRule != 0 {
		reasons = append(reasons, "stake_rule")
	}
	if flags&BizInvalidClaimRule != 0 {
		reasons = append(reasons, "claim_rule")
	}
	if flags&BizInvalidUnknown != 0 {
		reasons = append(reasons, "unknown")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "unmapped")
	}
	return reasons
}

func (m *Manager) resolveRegisterInvalidFlags(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload, enforceIndexerAllow bool) uint64 {
	if m == nil || payload == nil {
		return BizInvalidUnknown
	}

	if enforceIndexerAllow {
		if tx == nil {
			return BizInvalidRegisterRule
		}
		indexerID := protocolparser.BuildIndexerID(currentHeight, tx.TxIdx)
		if !conf.StakeRewardCfg.IsIndexerAllowedAtHeight(indexerID, currentHeight) {
			return BizInvalidRegisterRule
		}
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
