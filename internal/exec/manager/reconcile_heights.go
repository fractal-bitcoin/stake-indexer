package indexer

import (
	"fmt"
	"stake_indexer/constant"
	rollbackpkg "stake_indexer/internal/entry/rollback"
	"stake_indexer/internal/component/pg"
)

type heightState struct {
	Name   string
	Exists bool
	Height uint32
}

func EnsureIndexerDataConsistency() (height uint32, err error) {
	committedHeight, committedExists, err := pgdb.GetLatestCommittedSyncBlockHeight(ctx)
	if err != nil {
		return 0, err
	}
	utxoHeight, utxoExists, err := rollbackpkg.GetIndexerUtxoHeight()
	if err != nil {
		return 0, err
	}
	realtimeHeight, realtimeExists, err := rollbackpkg.GetInfoFieldUInt32(constant.TASK_BLOCK_HEIGHT)
	if err != nil {
		return 0, err
	}

	states := []heightState{
		{Name: "pg_committed", Exists: committedExists, Height: committedHeight},
		{Name: "redis_utxo", Exists: utxoExists, Height: utxoHeight},
		{Name: "redis_realtime", Exists: realtimeExists, Height: realtimeHeight},
	}
	if err := validateHeightStates("indexer", states); err != nil {
		return 0, err
	}
	if !committedExists {
		return 0, nil
	}
	return committedHeight + 1, nil
}

func validateHeightStates(domain string, states []heightState) error {
	if len(states) == 0 {
		return nil
	}

	allMissing := true
	for _, item := range states {
		if item.Exists {
			allMissing = false
			break
		}
	}
	if allMissing {
		return nil
	}

	allExists := true
	for _, item := range states {
		if !item.Exists {
			allExists = false
			break
		}
	}
	if !allExists {
		return fmt.Errorf("%s height consistency broken: partial existence: %s", domain, formatHeightStates(states))
	}

	base := states[0].Height
	for i := 1; i < len(states); i++ {
		if states[i].Height != base {
			return fmt.Errorf("%s height consistency broken: mismatch: %s", domain, formatHeightStates(states))
		}
	}
	return nil
}

func formatHeightStates(states []heightState) string {
	result := ""
	for i, item := range states {
		if i > 0 {
			result += ", "
		}
		if item.Exists {
			result += fmt.Sprintf("%s=%d", item.Name, item.Height)
		} else {
			result += fmt.Sprintf("%s=nil", item.Name)
		}
	}
	return result
}


