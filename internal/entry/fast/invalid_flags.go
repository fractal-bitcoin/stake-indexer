package fast

import (
	"strings"

	protocolparser "stake_indexer/internal/parser/protocol"
)

const (
	BizInvalidNone            uint64 = 0
	BizInvalidIndexerNotFound uint64 = 1 << 0
	BizInvalidActorNotOwner   uint64 = 1 << 1
	BizInvalidRegisterRule    uint64 = 1 << 2
	BizInvalidProofRule       uint64 = 1 << 3
	BizInvalidStakeRule       uint64 = 1 << 4
	BizInvalidClaimRule       uint64 = 1 << 5
	BizInvalidUnknown         uint64 = 1 << 31
)

type BusinessInvalidDeps interface {
	ResolveRegisterInvalidFlags(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) uint64
	ResolveOwnerAuthInvalidFlags(currentHeight uint32, payload *protocolparser.OpReturnPayload) uint64
	BuildStakeProof(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (bool, error)
	ValidateStakeTx(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (bool, error)
	BuildStakeClaimedReward(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (bool, error)
}

func ResolveBusinessInvalidFlags(deps BusinessInvalidDeps, currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) uint64 {
	if deps == nil || tx == nil || payload == nil {
		return BizInvalidUnknown
	}

	switch payload.Tag {
	case protocolparser.TagRegister:
		if strings.TrimSpace(payload.Get(protocolparser.OpFieldRewardAddr)) == "" {
			return BizInvalidRegisterRule
		}
		if _, ok := protocolparser.ParseRatio(payload.Get(protocolparser.OpFieldIndexRatio)); !ok {
			return BizInvalidRegisterRule
		}
		return deps.ResolveRegisterInvalidFlags(currentHeight, tx, payload)
	case protocolparser.TagAllocatRatio:
		return deps.ResolveOwnerAuthInvalidFlags(currentHeight, payload)
	case protocolparser.TagProveStake:
		ok, err := deps.BuildStakeProof(currentHeight, tx, payload)
		if err != nil || !ok {
			return BizInvalidProofRule
		}
		return BizInvalidNone
	case protocolparser.TagStake:
		ok, err := deps.ValidateStakeTx(currentHeight, tx, payload)
		if err != nil || !ok {
			return BizInvalidStakeRule
		}
		return BizInvalidNone
	case protocolparser.TagPledgedReward:
		ok, err := deps.BuildStakeClaimedReward(currentHeight, tx, payload)
		if err != nil || !ok {
			return BizInvalidClaimRule
		}
		return BizInvalidNone
	default:
		return BizInvalidUnknown
	}
}
