package indexer

import (
	"context"
	"strings"

	pgdb "stake_indexer/internal/component/pg"
	mempoolentry "stake_indexer/internal/entry/mempool"
	protocolparser "stake_indexer/internal/parser/protocol"
)

type managerMempoolSyncDeps struct {
	m         *Manager
	tipHeight uint32
}

func (d managerMempoolSyncDeps) Context() context.Context { return d.m.ctx }
func (d managerMempoolSyncDeps) LoadStakeBindingsToHeight(height uint32, withBalance bool) error {
	return d.m.LoadStakeBindingsToHeight(height, withBalance)
}
func (d managerMempoolSyncDeps) ResetMempoolOutpointCache() {
	d.m.mempoolOutpointCache = make(map[string]mempoolOutpointInfo, 1024)
}
func (d managerMempoolSyncDeps) IsConfirmedStakeAddress(stakeAddress string) bool {
	stakeAddress = strings.TrimSpace(stakeAddress)
	if stakeAddress == "" {
		return false
	}
	_, ok := d.m.stakeAddrToIndexer[stakeAddress]
	return ok
}
func (d managerMempoolSyncDeps) AccumulateMempoolStakeBalanceDelta(rawHex string, deltas map[string]int64) error {
	return d.m.accumulateMempoolStakeBalanceDelta(rawHex, deltas)
}
func (d managerMempoolSyncDeps) AccumulateMempoolStakeBalanceDeltaByAddresses(rawHex string, deltas map[string]int64, stakeAddresses map[string]struct{}) error {
	if len(stakeAddresses) == 0 {
		return nil
	}
	tracked := make(map[string]StakeAddressInfo, len(stakeAddresses))
	for stakeAddress := range stakeAddresses {
		stakeAddress = strings.TrimSpace(stakeAddress)
		if stakeAddress == "" {
			continue
		}
		tracked[stakeAddress] = StakeAddressInfo{}
	}
	return d.m.accumulateMempoolStakeBalanceDeltaByAddresses(rawHex, deltas, tracked)
}
func (d managerMempoolSyncDeps) ParseMempoolProtocolTx(rawHex string, txIdx uint32) (*protocolparser.ParsedProtocolTx, error) {
	return d.m.parseMempoolProtocolTx(rawHex, txIdx)
}
func (d managerMempoolSyncDeps) ResolveBusinessInvalidFlags(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) uint64 {
	return d.m.resolveBusinessInvalidFlags(currentHeight, tx, payload)
}
func (d managerMempoolSyncDeps) BuildStakeProof(txHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (*pgdb.StakeProof, error) {
	return d.m.buildStakeProofAtHeight(d.tipHeight, txHeight, tx, payload)
}
func (d managerMempoolSyncDeps) ValidateStakeBinding(tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (*mempoolentry.StakeBinding, bool, error) {
	binding, ok, err := d.m.validateStakeTxWithPayload(d.tipHeight, tx, payload)
	if err != nil || !ok || binding == nil {
		return nil, ok, err
	}
	return &mempoolentry.StakeBinding{IndexerID: binding.IndexerID, UserAddress: binding.UserAddress, StakeAddress: binding.StakeAddress, Amount: binding.Amount}, true, nil
}
func (d managerMempoolSyncDeps) BuildStakeClaimedReward(txHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (*pgdb.StakeClaimedReward, error) {
	return d.m.buildStakeClaimedReward(txHeight, tx, payload)
}
func (d managerMempoolSyncDeps) ValidateUpdateRatioTx(tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (string, float64, bool, error) {
	return d.m.validateUpdateRatioTx(d.tipHeight, tx, payload)
}
func (d managerMempoolSyncDeps) SaveMempoolStakeBalanceDeltas(deltas map[string]int64, events []pgdb.StakeMempoolEvent) error {
	return d.m.saveMempoolStakeBalanceDeltas(deltas, events)
}
func (d managerMempoolSyncDeps) BuildMempoolIndexerDeltas(addressDeltas map[string]int64) (map[string]int64, map[string]map[string]int64) {
	return d.m.buildMempoolIndexerDeltas(addressDeltas)
}
func (d managerMempoolSyncDeps) SaveMempoolIndexerStakerSnapshots(addressDeltas map[string]int64, stakerDeltas map[string]map[string]int64, events []pgdb.StakeMempoolEvent) error {
	return d.m.saveMempoolIndexerStakerSnapshots(addressDeltas, stakerDeltas, events)
}
