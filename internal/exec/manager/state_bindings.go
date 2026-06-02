package indexer

import (
	"strconv"

	"stake_indexer/constant"
	"stake_indexer/internal/component/log"
	"stake_indexer/internal/component/pg"
	"stake_indexer/internal/component/redis"

	redis "github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// LoadStakeBindingsToHeight loads stake binding relations from PG up to the given height
// and caches stakeAddress -> indexerID mapping in memory for fast delta application.
// syncBalance calls this once per consumed block (incremental range load).
func (m *Manager) LoadStakeBindingsToHeight(height uint32, withBalance bool) error {
	if m == nil {
		return nil
	}
	if pgdb.StakeDB == nil {
		return nil
	}

	if height < m.stakeBindingsLoadedHeight {
		m.stakeBindingsLoadedHeight = 0
		m.stakeAddrToIndexer = make(map[string]StakeAddressInfo)
		m.indexerToAddrStakeAmount = make(map[string]map[string]uint64)
		m.indexerToUserStakeAddress = make(map[string]map[string]string)
	}
	if height == m.stakeBindingsLoadedHeight {
		return nil
	}

	items, err := pgdb.ListStakeBindingsRange(m.ctx, m.stakeBindingsLoadedHeight, height)
	if err != nil {
		return err
	}

	m.stakeBindingsLoadedHeight = height

	for _, item := range items {
		m.cacheStakeBinding(item.UserAddress, item.IndexerID, item.AddressType, item.StakeAddress)
	}

	if withBalance {
		pipe := rdb.RdbBalanceClient.Pipeline()
		defer pipe.Close()
		balanceCmds := make(map[string]*redis.StringCmd, len(items))
		for _, item := range items {
			if m.pendingRewardMode {
				// Pending reward flow must not bootstrap from shared snapshot balances.
				// It should rebuild its own pending snapshot balances from configured start height.
				balanceCmds[item.StakeAddress] = pipe.Get(m.ctx, constant.GetPendingSnapshotBalanceKey(item.StakeAddress))
				continue
			}
			balanceCmds[item.StakeAddress] = pipe.Get(m.ctx, constant.GetSnapshotBalanceKey(item.StakeAddress))
		}
		if _, err := pipe.Exec(m.ctx); err != nil && err != redis.Nil {
			logger.Log.Error("load snapshot balance failed", zap.Error(err), zap.Int("height", int(height)))
			return err
		}
		for stakeAddress, cmd := range balanceCmds {
			balance, err := cmd.Result()
			if err != nil && err != redis.Nil {
				logger.Log.Error("load snapshot balance failed", zap.Error(err), zap.String("address", stakeAddress))
				return err
			}
			stakeAddrInfo, ok := m.stakeAddrToIndexer[stakeAddress]
			if !ok {
				continue
			}
			iBalance, _ := strconv.ParseUint(balance, 10, 64)
			if _, ok := m.indexerToAddrStakeAmount[stakeAddrInfo.IndexerID]; !ok {
				m.indexerToAddrStakeAmount[stakeAddrInfo.IndexerID] = make(map[string]uint64)
			}
			m.indexerToAddrStakeAmount[stakeAddrInfo.IndexerID][stakeAddrInfo.Address] = iBalance
		}
	}

	return nil
}

// StageStakeBalanceDeltas applies per-block address balance deltas to stake zset scores.
// The stake amount is purely the stake address balance snapshot, and is updated by balance deltas,
// not by parsing stake tx semantics.
func (m *Manager) StageStakeBalanceDeltas(addressDeltas map[string]int64) {
	if m == nil || m.slowState == nil || len(addressDeltas) == 0 {
		return
	}
	if len(m.stakeAddrToIndexer) == 0 {
		return
	}

	for address, delta := range addressDeltas {
		if address == "" || delta == 0 {
			continue
		}

		stakeAddrInfo, ok := m.stakeAddrToIndexer[address]
		if !ok {
			continue
		}
		stakeAddrs := m.indexerToAddrStakeAmount[stakeAddrInfo.IndexerID]
		if stakeAddrs == nil {
			stakeAddrs = make(map[string]uint64)
		}
		current := stakeAddrs[stakeAddrInfo.Address]
		if delta > 0 {
			current += uint64(delta)
		} else {
			dec := uint64(-delta)
			if dec >= current {
				current = 0
			} else {
				current -= dec
			}
		}
		if current == 0 {
			delete(stakeAddrs, stakeAddrInfo.Address)
		} else {
			stakeAddrs[stakeAddrInfo.Address] = current
		}
		m.indexerToAddrStakeAmount[stakeAddrInfo.IndexerID] = stakeAddrs
	}
}
