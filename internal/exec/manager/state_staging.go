package indexer

import (
	"fmt"
	"stake_indexer/constant"
	"stake_indexer/internal/component/redis"
	protocolparser "stake_indexer/internal/parser/protocol"
	"strings"

	redis "github.com/go-redis/redis/v8"
)

type slowWriteState struct {
	hset        map[string]map[string]interface{}
	hincrByF64  map[string]map[string]float64
	zincrBy     map[string]map[string]float64
	incrBy      map[string]int64
	latestRatio map[string]float64
	snapRatio   map[string]float64
}

func newSlowWriteState() *slowWriteState {
	return &slowWriteState{
		hset:        make(map[string]map[string]interface{}),
		hincrByF64:  make(map[string]map[string]float64),
		zincrBy:     make(map[string]map[string]float64),
		incrBy:      make(map[string]int64),
		latestRatio: make(map[string]float64),
		snapRatio:   make(map[string]float64),
	}
}

func (s *slowWriteState) isEmpty() bool {
	return len(s.hset) == 0 &&
		len(s.hincrByF64) == 0 &&
		len(s.zincrBy) == 0 &&
		len(s.incrBy) == 0
}

func (m *Manager) FlushBalanceChangesWithFinalizer(finalizer func(redis.Pipeliner)) error {
	return m.flushBalanceChanges(finalizer)
}

func (m *Manager) flushBalanceChanges(finalizer func(redis.Pipeliner)) error {
	if m == nil || m.slowState == nil {
		return nil
	}
	if m.slowState.isEmpty() && finalizer == nil {
		return nil
	}

	state := m.slowState
	pipe := rdb.RdbBalanceClient.Pipeline()
	defer pipe.Close()

	for key, fields := range state.hset {
		if len(fields) == 0 {
			continue
		}
		pipe.HSet(m.ctx, key, fields)
	}

	for key, fields := range state.hincrByF64 {
		for field, delta := range fields {
			if delta == 0 {
				continue
			}
			pipe.HIncrByFloat(m.ctx, key, field, delta)
		}
	}

	for key, members := range state.zincrBy {
		for member, delta := range members {
			if delta == 0 {
				continue
			}
			pipe.ZIncrBy(m.ctx, key, delta, member)
		}
	}

	for key, delta := range state.incrBy {
		if delta == 0 {
			continue
		}
		pipe.IncrBy(m.ctx, key, delta)
	}

	if finalizer != nil {
		finalizer(pipe)
	}

	if _, err := pipe.Exec(m.ctx); err != nil {
		// Drop staged in-memory mutations on flush failure to avoid duplicate
		// accumulation after retry; caller will re-run block consumption.
		m.slowState = newSlowWriteState()
		return fmt.Errorf("flush stake pending writes failed: %w", err)
	}

	m.slowState = newSlowWriteState()
	return nil
}

func (m *Manager) stageHSet(key string, values map[string]interface{}) {
	if m == nil || m.slowState == nil || len(values) == 0 {
		return
	}

	fields, ok := m.slowState.hset[key]
	if !ok {
		fields = make(map[string]interface{}, len(values))
		m.slowState.hset[key] = fields
	}
	for k, v := range values {
		fields[k] = v
	}

	if strings.HasPrefix(key, constant.REDIS_STAKE_INDEXER_INFO_PREFIX) {
		indexerID := strings.TrimPrefix(key, constant.REDIS_STAKE_INDEXER_INFO_PREFIX)
		if raw, ok := values["index_ratio"]; ok {
			if ratio, parsed := protocolparser.ParseRatio(fmt.Sprint(raw)); parsed {
				m.slowState.latestRatio[indexerID] = ratio
			}
		}
		return
	}

	if key == constant.REDIS_STAKE_INDEXER_RATIO_SNAPSHOT_KEY {
		for indexerID, raw := range values {
			if ratio, parsed := protocolparser.ParseRatio(fmt.Sprint(raw)); parsed {
				m.slowState.snapRatio[indexerID] = ratio
			}
		}
	}
}

func (m *Manager) stageHIncrByFloat(key, field string, delta float64) {
	if m == nil || m.slowState == nil || delta == 0 {
		return
	}
	fields, ok := m.slowState.hincrByF64[key]
	if !ok {
		fields = make(map[string]float64, 2)
		m.slowState.hincrByF64[key] = fields
	}
	fields[field] += delta
}

func (m *Manager) stageZIncrBy(key string, delta float64, member string) {
	if m == nil || m.slowState == nil || member == "" || delta == 0 {
		return
	}
	members, ok := m.slowState.zincrBy[key]
	if !ok {
		members = make(map[string]float64, 4)
		m.slowState.zincrBy[key] = members
	}
	members[member] += delta
	if members[member] == 0 {
		delete(members, member)
	}
}

func (m *Manager) stageIncrBy(key string, delta int64) {
	if m == nil || m.slowState == nil || delta == 0 {
		return
	}
	m.slowState.incrBy[key] += delta
}

func (m *Manager) getCachedIndexerRatio(indexerID string) (float64, bool) {
	if m == nil || m.slowState == nil || indexerID == "" {
		return 0, false
	}
	ratio, ok := m.slowState.latestRatio[indexerID]
	return ratio, ok
}

func (m *Manager) getCachedIndexerSnapshotRatio(indexerID string) (float64, bool) {
	if m == nil || m.slowState == nil || indexerID == "" {
		return 0, false
	}
	ratio, ok := m.slowState.snapRatio[indexerID]
	return ratio, ok
}

func (m *Manager) getIndexerStakeAmount(indexerID string) (map[string]float64, error) {
	addresses := m.indexerToAddrStakeAmount[indexerID]
	amounts := make(map[string]float64, len(addresses))
	for address, amount := range addresses {
		amounts[address] = float64(amount)
	}
	return amounts, nil
}
