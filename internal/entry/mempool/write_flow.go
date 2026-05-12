package mempool

import (
	"context"
	"fmt"
	"strings"

	"stake_indexer/internal/component/log"
	"stake_indexer/internal/component/pg"

	"go.uber.org/zap"
)

type ProtocolSyncStats struct {
	MempoolTxs  int
	ProtocolTxs int
	ProofTxs    int
	Upserted    int
	Removed     int
}

type ProtocolWriteInput struct {
	ExistingEventTxIDs       []string
	ExistingEventSet         map[string]struct{}
	CurrentEventTxIDs        map[string]struct{}
	Events                   []pgdb.StakeMempoolEvent
	ExistingInscriptionTxIDs []string
	ExistingInscriptionSet   map[string]struct{}
	CurrentInscriptionTxIDs  map[string]struct{}
	InscriptionEvents        []pgdb.FIP101InscriptionEvent
	PendingStakeDeltas       map[string]int64
	Incomplete               bool
	ParseIncomplete          bool
}

type WriteDeps interface {
	SaveMempoolStakeBalanceDeltas(deltas map[string]int64, events []pgdb.StakeMempoolEvent) error
	BuildMempoolIndexerDeltas(addressDeltas map[string]int64) (map[string]int64, map[string]map[string]int64)
	SaveMempoolIndexerStakerSnapshots(addressDeltas map[string]int64, stakerDeltas map[string]map[string]int64, events []pgdb.StakeMempoolEvent) error
}

func RunProtocolWriteFlow(ctx context.Context, deps WriteDeps, stats *ProtocolSyncStats, input ProtocolWriteInput) error {
	if deps == nil || stats == nil {
		return nil
	}

	for _, event := range input.Events {
		if event.TxID == "" {
			continue
		}
		if err := pgdb.UpsertStakeMempoolEvent(ctx, event); err != nil {
			return fmt.Errorf("upsert mempool protocol event failed: %w", err)
		}
		if _, exists := input.ExistingEventSet[event.TxID]; !exists {
			stats.Upserted++
		}
	}
	if len(input.InscriptionEvents) > 0 {
		items := make([]*pgdb.FIP101InscriptionEvent, 0, len(input.InscriptionEvents))
		for i := range input.InscriptionEvents {
			event := input.InscriptionEvents[i]
			if strings.TrimSpace(event.TxID) == "" {
				continue
			}
			items = append(items, &input.InscriptionEvents[i])
			if _, exists := input.ExistingInscriptionSet[event.TxID]; !exists {
				stats.Upserted++
			}
		}
		if err := pgdb.UpsertFIP101InscriptionEventsBatch(ctx, items); err != nil {
			return fmt.Errorf("upsert mempool fip101 inscription events failed: %w", err)
		}
	}

	if input.Incomplete || input.ParseIncomplete {
		logger.Log.Warn("skip mempool cleanup due incomplete snapshot",
			zap.Bool("normalize_incomplete", input.Incomplete),
			zap.Bool("parse_incomplete", input.ParseIncomplete),
		)
		return nil
	}

	if err := deps.SaveMempoolStakeBalanceDeltas(input.PendingStakeDeltas, input.Events); err != nil {
		return fmt.Errorf("sync mempool stake balance deltas failed: %w", err)
	}

	_, stakerDeltas := deps.BuildMempoolIndexerDeltas(input.PendingStakeDeltas)
	if err := deps.SaveMempoolIndexerStakerSnapshots(input.PendingStakeDeltas, stakerDeltas, input.Events); err != nil {
		return fmt.Errorf("sync mempool indexer stakers snapshot failed: %w", err)
	}

	eventsToDelete := make([]string, 0)
	for _, txID := range input.ExistingEventTxIDs {
		if _, ok := input.CurrentEventTxIDs[txID]; ok {
			continue
		}
		eventsToDelete = append(eventsToDelete, txID)
	}
	if len(eventsToDelete) > 0 {
		if err := pgdb.DeleteStakeMempoolEventsByTxIDs(ctx, eventsToDelete); err != nil {
			return fmt.Errorf("cleanup stale mempool protocol events failed: %w", err)
		}
		stats.Removed += len(eventsToDelete)
	}
	inscriptionEventsToDelete := make([]string, 0)
	for _, txID := range input.ExistingInscriptionTxIDs {
		if _, ok := input.CurrentInscriptionTxIDs[txID]; ok {
			continue
		}
		inscriptionEventsToDelete = append(inscriptionEventsToDelete, txID)
	}
	if len(inscriptionEventsToDelete) > 0 {
		if err := pgdb.DeleteMempoolFIP101InscriptionEventsByTxIDs(ctx, inscriptionEventsToDelete); err != nil {
			return fmt.Errorf("cleanup stale mempool fip101 inscription events failed: %w", err)
		}
		stats.Removed += len(inscriptionEventsToDelete)
	}

	return nil
}
