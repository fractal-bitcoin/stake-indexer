package mempool

import (
	"context"
	"fmt"
	"strings"

	"stake_indexer/constant"
	logger "stake_indexer/internal/component/log"
	"stake_indexer/internal/component/node"
	pgdb "stake_indexer/internal/component/pg"
	entryfast "stake_indexer/internal/entry/fast"
	protocolparser "stake_indexer/internal/parser/protocol"

	"go.uber.org/zap"
)

type StakeBinding struct {
	IndexerID    string
	UserAddress  string
	StakeAddress string
	Amount       uint64
}

type SyncDeps interface {
	WriteDeps
	Context() context.Context
	LoadStakeBindingsToHeight(height uint32, withBalance bool) error
	ResetMempoolOutpointCache()
	IsConfirmedStakeAddress(stakeAddress string) bool
	AccumulateMempoolStakeBalanceDelta(rawHex string, deltas map[string]int64) error
	AccumulateMempoolStakeBalanceDeltaByAddresses(rawHex string, deltas map[string]int64, stakeAddresses map[string]struct{}) error
	ParseMempoolProtocolTx(rawHex string, txIdx uint32) (*protocolparser.ParsedProtocolTx, error)
	ResolveBusinessInvalidFlags(currentHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) uint64
	BuildStakeProof(txHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (*pgdb.StakeProof, error)
	ValidateStakeBinding(tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (*StakeBinding, bool, error)
	BuildStakeClaimedReward(txHeight uint32, tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (*pgdb.StakeClaimedReward, error)
	ValidateUpdateRatioTx(tx *protocolparser.TxSnapshot, payload *protocolparser.OpReturnPayload) (string, float64, bool, error)
}

func RunProtocolSync(deps SyncDeps, tipHeight uint32) (ProtocolSyncStats, error) {
	stats := ProtocolSyncStats{}
	if deps == nil {
		return stats, nil
	}

	if err := deps.LoadStakeBindingsToHeight(tipHeight, false); err != nil {
		return stats, fmt.Errorf("load stake bindings for mempool failed: %w", err)
	}

	tipHash, err := node.GetBlockHashRPC(tipHeight)
	if err != nil {
		return stats, fmt.Errorf("load tip block hash failed: %w", err)
	}
	rawEntries, err := node.GetRawMemPoolRPC(tipHash)
	if err != nil {
		return stats, fmt.Errorf("load raw mempool failed: %w", err)
	}
	trimmedEntries, txidMode := normalizeMempoolEntries(rawEntries)
	stats.MempoolTxs = len(trimmedEntries)
	incomplete := false
	deps.ResetMempoolOutpointCache()

	ctx := deps.Context()
	existingEventTxIDs, err := pgdb.ListStakeMempoolEventTxIDs(ctx)
	if err != nil {
		return stats, fmt.Errorf("list existing mempool event txids failed: %w", err)
	}
	existingInscriptionTxIDs, err := pgdb.ListMempoolFIP101InscriptionEventTxIDs(ctx)
	if err != nil {
		return stats, fmt.Errorf("list existing mempool fip101 event txids failed: %w", err)
	}
	existingEventSet := make(map[string]struct{}, len(existingEventTxIDs))
	for _, txID := range existingEventTxIDs {
		existingEventSet[txID] = struct{}{}
	}
	existingInscriptionSet := make(map[string]struct{}, len(existingInscriptionTxIDs))
	for _, txID := range existingInscriptionTxIDs {
		existingInscriptionSet[txID] = struct{}{}
	}

	events := make([]pgdb.StakeMempoolEvent, 0, 32)
	inscriptionEvents := make([]pgdb.FIP101InscriptionEvent, 0, 32)
	currentProveTxIDs := make(map[string]struct{}, 16)
	currentEventTxIDs := make(map[string]struct{}, 32)
	currentInscriptionTxIDs := make(map[string]struct{}, 32)
	parseIncomplete := false
	pendingStakeDeltas := make(map[string]int64, 64)

	for i, entry := range trimmedEntries {
		rawHex := entry
		if txidMode {
			txID := strings.ToLower(strings.TrimSpace(entry))
			fetched, err := node.GetRawTxHexRPC(txID)
			if err != nil {
				incomplete = true
				logger.Log.Debug("load mempool tx raw by txid failed", zap.String("txid", txID), zap.Error(err))
				continue
			}
			rawHex = strings.TrimSpace(fetched)
			if rawHex == "" {
				incomplete = true
				continue
			}
		}

		if err := deps.AccumulateMempoolStakeBalanceDelta(rawHex, pendingStakeDeltas); err != nil {
			logger.Log.Debug("accumulate mempool stake balance delta failed", zap.Error(err))
			parseIncomplete = true
			continue
		}

		parsed, err := deps.ParseMempoolProtocolTx(rawHex, uint32(i))
		if err != nil {
			logger.Log.Debug("parse mempool tx failed", zap.Error(err))
			parseIncomplete = true
			continue
		}
		if parsed == nil || parsed.Payload == nil {
			continue
		}
		bizInvalidFlags := deps.ResolveBusinessInvalidFlags(tipHeight, &parsed.Snapshot, parsed.Payload)
		inscriptionContent := ""
		if parsed.Event != nil {
			inscriptionContent = parsed.Event.InscriptionContent
		}
		if parsed.Event != nil {
			parsed.Event.BizInvalidFlags = bizInvalidFlags
			inscriptionEvents = append(inscriptionEvents, *parsed.Event)
			currentInscriptionTxIDs[parsed.Event.TxID] = struct{}{}
		}

		stats.ProtocolTxs++
		switch parsed.Payload.Tag {
		case protocolparser.TagProveStake:
			event := pgdb.StakeMempoolEvent{TxID: parsed.Snapshot.TxID, Op: protocolparser.TagProveStake, Height: int64(constant.MEMPOOL_HEIGHT), InscriptionContent: inscriptionContent, IndexerID: strings.TrimSpace(parsed.Payload.Get(protocolparser.OpFieldIndexerID)), BizInvalidFlags: bizInvalidFlags, TxIdx: parsed.Snapshot.TxIdx}
			if bizInvalidFlags != entryfast.BizInvalidNone {
				events = append(events, event)
				currentEventTxIDs[event.TxID] = struct{}{}
				continue
			}
			proof, err := deps.BuildStakeProof(constant.MEMPOOL_HEIGHT, &parsed.Snapshot, parsed.Payload)
			if err != nil || proof == nil {
				event.BizInvalidFlags = entryfast.BizInvalidProofRule
				events = append(events, event)
				currentEventTxIDs[event.TxID] = struct{}{}
				continue
			}
			if _, exists := currentProveTxIDs[proof.TxID]; !exists {
				currentProveTxIDs[proof.TxID] = struct{}{}
				stats.ProofTxs++
			}
			event.IndexerID, event.ProveBlockHeight, event.ProveDataHash = proof.IndexerID, proof.ProveBlockHeight, proof.ProveDataHash
			events = append(events, event)
			currentEventTxIDs[event.TxID] = struct{}{}
		case protocolparser.TagRegister:
			ratio, _ := protocolparser.ParseRatio(parsed.Payload.Get(protocolparser.OpFieldIndexRatio))
			ownerAddr := strings.TrimSpace(parsed.Payload.Get(protocolparser.OpFieldActorAddr))
			rewardAddr := strings.TrimSpace(parsed.Payload.Get(protocolparser.OpFieldRewardAddr))
			event := pgdb.StakeMempoolEvent{TxID: parsed.Snapshot.TxID, Op: protocolparser.TagRegister, Height: int64(constant.MEMPOOL_HEIGHT), InscriptionContent: inscriptionContent, UserAddress: ownerAddr, RewardAddress: rewardAddr, IndexRatio: ratio, IndexerName: strings.TrimSpace(parsed.Payload.Get(protocolparser.OpFieldIndexerName)), BizInvalidFlags: bizInvalidFlags, TxIdx: parsed.Snapshot.TxIdx}
			events = append(events, event)
			currentEventTxIDs[event.TxID] = struct{}{}
		case protocolparser.TagStake:
			event := pgdb.StakeMempoolEvent{TxID: parsed.Snapshot.TxID, Op: protocolparser.TagStake, Height: int64(constant.MEMPOOL_HEIGHT), InscriptionContent: inscriptionContent, IndexerID: strings.TrimSpace(parsed.Payload.Get(protocolparser.OpFieldIndexerID)), BizInvalidFlags: bizInvalidFlags, TxIdx: parsed.Snapshot.TxIdx}
			if bizInvalidFlags != entryfast.BizInvalidNone {
				events = append(events, event)
				currentEventTxIDs[event.TxID] = struct{}{}
				continue
			}
			binding, ok, err := deps.ValidateStakeBinding(&parsed.Snapshot, parsed.Payload)
			if err != nil || !ok || binding == nil {
				event.BizInvalidFlags = entryfast.BizInvalidStakeRule
				events = append(events, event)
				currentEventTxIDs[event.TxID] = struct{}{}
				continue
			}
			event.IndexerID, event.UserAddress, event.StakeAddress, event.Amount = binding.IndexerID, binding.UserAddress, binding.StakeAddress, binding.Amount
			events = append(events, event)
			currentEventTxIDs[event.TxID] = struct{}{}
		case protocolparser.TagPledgedReward:
			event := pgdb.StakeMempoolEvent{TxID: parsed.Snapshot.TxID, Op: protocolparser.TagPledgedReward, Height: int64(constant.MEMPOOL_HEIGHT), InscriptionContent: inscriptionContent, BizInvalidFlags: bizInvalidFlags, TxIdx: parsed.Snapshot.TxIdx}
			if bizInvalidFlags != entryfast.BizInvalidNone {
				events = append(events, event)
				currentEventTxIDs[event.TxID] = struct{}{}
				continue
			}
			item, err := deps.BuildStakeClaimedReward(constant.MEMPOOL_HEIGHT, &parsed.Snapshot, parsed.Payload)
			if err != nil || item == nil {
				event.BizInvalidFlags = entryfast.BizInvalidClaimRule
				events = append(events, event)
				currentEventTxIDs[event.TxID] = struct{}{}
				continue
			}
			event.UserAddress, event.Amount = item.UserAddress, item.Amount
			events = append(events, event)
			currentEventTxIDs[event.TxID] = struct{}{}
		case protocolparser.TagAllocatRatio:
			event := pgdb.StakeMempoolEvent{TxID: parsed.Snapshot.TxID, Op: protocolparser.TagAllocatRatio, Height: int64(constant.MEMPOOL_HEIGHT), InscriptionContent: inscriptionContent, IndexerID: strings.TrimSpace(parsed.Payload.Get(protocolparser.OpFieldIndexerID)), BizInvalidFlags: bizInvalidFlags, TxIdx: parsed.Snapshot.TxIdx}
			if bizInvalidFlags != entryfast.BizInvalidNone {
				events = append(events, event)
				currentEventTxIDs[event.TxID] = struct{}{}
				continue
			}
			indexerID, ratio, ok, err := deps.ValidateUpdateRatioTx(&parsed.Snapshot, parsed.Payload)
			if err != nil {
				event.BizInvalidFlags = entryfast.BizInvalidUnknown
				events = append(events, event)
				currentEventTxIDs[event.TxID] = struct{}{}
				continue
			}
			if !ok {
				event.BizInvalidFlags = entryfast.BizInvalidActorNotOwner | entryfast.BizInvalidIndexerNotFound
				events = append(events, event)
				currentEventTxIDs[event.TxID] = struct{}{}
				continue
			}
			event.IndexerID, event.IndexRatio = indexerID, ratio
			events = append(events, event)
			currentEventTxIDs[event.TxID] = struct{}{}
		}
	}

	firstBindStakeAddresses := make(map[string]struct{}, 16)
	for _, event := range events {
		if event.Op != protocolparser.TagStake || event.BizInvalidFlags != entryfast.BizInvalidNone {
			continue
		}
		stakeAddress := strings.TrimSpace(event.StakeAddress)
		if stakeAddress == "" || deps.IsConfirmedStakeAddress(stakeAddress) {
			continue
		}
		firstBindStakeAddresses[stakeAddress] = struct{}{}
	}
	if len(firstBindStakeAddresses) > 0 {
		for _, entry := range trimmedEntries {
			rawHex := entry
			if txidMode {
				txID := strings.ToLower(strings.TrimSpace(entry))
				fetched, err := node.GetRawTxHexRPC(txID)
				if err != nil {
					incomplete = true
					logger.Log.Debug("load mempool tx raw by txid failed", zap.String("txid", txID), zap.Error(err))
					continue
				}
				rawHex = strings.TrimSpace(fetched)
				if rawHex == "" {
					incomplete = true
					continue
				}
			}
			if err := deps.AccumulateMempoolStakeBalanceDeltaByAddresses(rawHex, pendingStakeDeltas, firstBindStakeAddresses); err != nil {
				logger.Log.Debug("accumulate mempool first-bind stake balance delta failed", zap.Error(err))
				parseIncomplete = true
				continue
			}
		}
	}

	if err := RunProtocolWriteFlow(ctx, deps, &stats, ProtocolWriteInput{ExistingEventTxIDs: existingEventTxIDs, ExistingEventSet: existingEventSet, CurrentEventTxIDs: currentEventTxIDs, Events: events, ExistingInscriptionTxIDs: existingInscriptionTxIDs, ExistingInscriptionSet: existingInscriptionSet, CurrentInscriptionTxIDs: currentInscriptionTxIDs, InscriptionEvents: inscriptionEvents, PendingStakeDeltas: pendingStakeDeltas, Incomplete: incomplete, ParseIncomplete: parseIncomplete}); err != nil {
		return stats, err
	}

	return stats, nil
}

func normalizeMempoolEntries(entries []string) ([]string, bool) {
	trimmed := make([]string, 0, len(entries))
	for _, item := range entries {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		trimmed = append(trimmed, item)
	}
	if len(trimmed) == 0 {
		return nil, false
	}

	txidMode := true
	sample := len(trimmed)
	if sample > 8 {
		sample = 8
	}
	for i := 0; i < sample; i++ {
		if !isLikelyTxIDHex(trimmed[i]) {
			txidMode = false
			break
		}
	}
	return trimmed, txidMode
}

func isLikelyTxIDHex(raw string) bool {
	raw = strings.TrimSpace(raw)
	if len(raw) != 64 {
		return false
	}
	for _, ch := range raw {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F') {
			return false
		}
	}
	return true
}
